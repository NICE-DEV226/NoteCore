package bindings

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Mise à jour dans l'application : là où CheckForUpdate se contente de
// prévenir, ApplyUpdate enchaîne tout — release GitHub, choix de
// l'artefact adapté à la plateforme et au mode d'installation,
// téléchargement, vérification SHA256 (SHA256SUMS de la release),
// installation puis relance. Chaque étape échoue proprement : une
// connexion coupée ou un paquet corrompu n'installe jamais rien.
//
// Méthodes d'installation détectées :
//   - local   : ~/.local (install.sh utilisateur) → extraction directe
//   - system  : /usr/local (install.sh --system)  → pkexec + install.sh
//   - pacman  : binaire appartenant à un paquet pacman → pkexec pacman -U
//   - deb     : binaire appartenant à un paquet dpkg  → pkexec apt-get
//   - windows : installeur NSIS /S puis fermeture de l'app
//   - macos   : remplacement du .app (admin si besoin)
//   - manual  : mode inconnu → le paquet est déposé dans ~/Downloads

// updateProgressEvent est le nom de l'événement Wails émis pendant le
// téléchargement (0-100), écouté par le store frontend.
const updateProgressEvent = "notecore:update-progress"

// UpdateResult décrit ce que l'interface annonce une fois l'opération
// terminée. Restart indique que l'application se ferme (ou s'est déjà
// fermée) pour laisser place à la nouvelle version.
type UpdateResult struct {
	Method  string `json:"method"`
	Message string `json:"message"`
	Restart bool   `json:"restart"`
}

// ApplyUpdate télécharge et installe la dernière release. Un appel
// synchrone long (le téléchargement peut prendre du temps sur une
// connexion lente) : l'interface affiche la progression émise par
// EventsEmit.
func (n *NoteCore) ApplyUpdate() (UpdateResult, error) {
	if appVersion == "" {
		return UpdateResult{}, fmt.Errorf("version de l'application inconnue")
	}

	ctx, cancel := context.WithTimeout(n.callContext(), 20*time.Minute)
	defer cancel()

	rel, err := fetchRelease(ctx)
	if err != nil {
		return UpdateResult{}, err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	if compareVersions(latest, appVersion) <= 0 {
		return UpdateResult{}, fmt.Errorf("vous avez déjà la dernière version (%s)", appVersion)
	}

	exe, err := os.Executable()
	if err != nil {
		return UpdateResult{}, fmt.Errorf("exécutable introuvable : %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return UpdateResult{}, fmt.Errorf("dossier personnel introuvable : %w", err)
	}

	plan, err := planUpdate(runtime.GOOS, exe, home, rel)
	if err != nil {
		return UpdateResult{}, err
	}

	// Téléchargement dans un dossier temporaire : pour pacman/dpkg le
	// paquet doit survivre à l'invite d'authentification pkexec, /tmp
	// convient (nettoyé au prochain démarrage du système).
	tmp, err := os.MkdirTemp("", "notecore-update-")
	if err != nil {
		return UpdateResult{}, fmt.Errorf("dossier temporaire : %w", err)
	}
	defer os.RemoveAll(tmp)

	pkgPath := filepath.Join(tmp, plan.assetName)
	if err := n.downloadFile(ctx, plan.assetURL, pkgPath); err != nil {
		return UpdateResult{}, fmt.Errorf("téléchargement : %w", err)
	}

	// Empreinte croisée avec le SHA256SUMS publié : un binaire altéré
	// ou tronqué ne passe jamais à l'installation.
	if err := n.verifyChecksum(ctx, rel, plan.assetName, pkgPath); err != nil {
		return UpdateResult{}, err
	}

	switch plan.method {
	case "local":
		if err := applyLocalUpdate(pkgPath, home); err != nil {
			return UpdateResult{}, err
		}
		n.relaunchAndQuit()
		return UpdateResult{Method: plan.method, Message: fmt.Sprintf("NoteCore %s installée — redémarrage.", latest), Restart: true}, nil
	case "system":
		if err := applySystemUpdate(pkgPath); err != nil {
			return UpdateResult{}, err
		}
		n.relaunchAndQuit()
		return UpdateResult{Method: plan.method, Message: fmt.Sprintf("NoteCore %s installée — redémarrage.", latest), Restart: true}, nil
	case "pacman":
		if err := runPrivileged("pacman", "-U", "--noconfirm", pkgPath); err != nil {
			return UpdateResult{}, fmt.Errorf("installation pacman : %w", err)
		}
		n.relaunchAndQuit()
		return UpdateResult{Method: plan.method, Message: fmt.Sprintf("NoteCore %s installée — redémarrage.", latest), Restart: true}, nil
	case "deb":
		if err := runPrivileged("apt-get", "install", "-y", pkgPath); err != nil {
			return UpdateResult{}, fmt.Errorf("installation du paquet : %w", err)
		}
		n.relaunchAndQuit()
		return UpdateResult{Method: plan.method, Message: fmt.Sprintf("NoteCore %s installée — redémarrage.", latest), Restart: true}, nil
	case "windows":
		// L'installateur NSIS se lance avec /S (silencieux si supporté,
		// assistant sinon) ; l'application se ferme pour libérer les
		// fichiers. Le parcours de l'installeur se charge du reste.
		if err := exec.Command(pkgPath, "/S").Start(); err != nil {
			return UpdateResult{}, fmt.Errorf("lancement de l'installateur : %w", err)
		}
		time.Sleep(700 * time.Millisecond)
		wailsruntime.Quit(n.callContext())
		return UpdateResult{Method: plan.method, Message: "Installateur lancé — l'application redémarre ensuite.", Restart: true}, nil
	case "macos":
		appPath, err := applyMacUpdate(pkgPath, exe)
		if err != nil {
			return UpdateResult{}, err
		}
		_ = exec.Command("open", "-n", appPath).Start()
		wailsruntime.Quit(n.callContext())
		return UpdateResult{Method: plan.method, Message: fmt.Sprintf("NoteCore %s installée — redémarrage.", latest), Restart: true}, nil
	default: // manual
		dest := filepath.Join(home, "Downloads", plan.assetName)
		if err := copyFile(pkgPath, dest); err != nil {
			return UpdateResult{}, err
		}
		return UpdateResult{Method: plan.method, Message: fmt.Sprintf("Mode d'installation non reconnu : le paquet est dans %s — installez-le manuellement.", dest), Restart: false}, nil
	}
}

// updatePlan associe l'artefact à télécharger à la méthode d'installation.
type updatePlan struct {
	assetName string
	assetURL  string
	method    string
}

// planUpdate choisit l'artefact de la release et la stratégie
// d'installation. Séparé du reste pour rester testable.
func planUpdate(goos, exe, home string, rel ghRelease) (updatePlan, error) {
	method := detectInstallMethod(goos, exe, home)
	suffix := assetSuffix(method)
	if suffix == "" {
		return updatePlan{}, fmt.Errorf("aucun paquet disponible pour ce système")
	}
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, suffix) {
			return updatePlan{assetName: a.Name, assetURL: a.BrowserDownloadURL, method: method}, nil
		}
	}
	return updatePlan{}, fmt.Errorf("la release ne contient pas l'artefact attendu (%s)", suffix)
}

// detectInstallMethod déduit comment l'application a été installée à
// partir de l'emplacement du binaire et de son propriétaire de paquet.
func detectInstallMethod(goos, exe, home string) string {
	switch goos {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	case "linux":
		if strings.HasPrefix(exe, filepath.Join(home, ".local")) {
			return "local"
		}
		if packageOwns(exe, "pacman", "-Qo") {
			return "pacman"
		}
		if packageOwns(exe, "dpkg-query", "-S") {
			return "deb"
		}
		if strings.HasPrefix(exe, "/usr/local") {
			return "system"
		}
	}
	return "manual"
}

// assetSuffix associe chaque méthode à la fin du nom d'artefact
// correspondant dans la release.
func assetSuffix(method string) string {
	switch method {
	case "pacman":
		return ".pkg.tar.zst"
	case "deb":
		return "-linux-amd64.deb"
	case "windows":
		return "-windows-x64-setup.exe"
	case "macos":
		return "-macos-universal.zip"
	case "local", "system", "manual":
		return "-linux-x64.tar.gz"
	}
	return ""
}

// packageOwns demande au gestionnaire de paquets s'il possède le
// binaire : pacman -Qo et dpkg-query -S sortent en erreur sinon.
func packageOwns(exe, tool string, arg string) bool {
	if _, err := exec.LookPath(tool); err != nil {
		return false
	}
	return exec.Command(tool, arg, exe).Run() == nil
}

// downloadFile télécharge url vers dest en émettant la progression
// (pourcent entier) via onProgress.
func (n *NoteCore) downloadFile(ctx context.Context, url, dest string) error {
	return downloadFile(ctx, url, dest, func(pct int) {
		wailsruntime.EventsEmit(n.callContext(), updateProgressEvent, pct)
	})
}

func downloadFile(ctx context.Context, url, dest string, onProgress func(int)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "NoteCore")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	total := resp.ContentLength
	buf := make([]byte, 128*1024)
	var done int64
	last := -1
	for {
		rn, rerr := resp.Body.Read(buf)
		if rn > 0 {
			if _, err := out.Write(buf[:rn]); err != nil {
				return err
			}
			done += int64(rn)
			if total > 0 {
				pct := int(done * 100 / total)
				if pct != last {
					last = pct
					onProgress(pct)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if total > 0 && done != total {
		return fmt.Errorf("téléchargement incomplet (%d/%d octets)", done, total)
	}
	return out.Close()
}

// verifyChecksum contrôle le fichier téléchargé contre le SHA256SUMS
// de la release. L'asset doit y figurer : une release sans empreintes
// ne peut pas être vérifiée et n'est donc pas installée.
func (n *NoteCore) verifyChecksum(ctx context.Context, rel ghRelease, assetName, path string) error {
	var sumsURL string
	for _, a := range rel.Assets {
		if a.Name == "SHA256SUMS" {
			sumsURL = a.BrowserDownloadURL
			break
		}
	}
	if sumsURL == "" {
		return fmt.Errorf("la release ne publie pas de SHA256SUMS : installation refusée")
	}

	tmp, err := os.CreateTemp("", "notecore-sums-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := downloadFile(ctx, sumsURL, tmp.Name(), func(int) {}); err != nil {
		return fmt.Errorf("empreintes : %w", err)
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	expected, ok := sha256ForAsset(string(data), assetName)
	if !ok {
		return fmt.Errorf("aucune empreinte pour %s dans SHA256SUMS", assetName)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, expected) {
		return fmt.Errorf("empreinte SHA256 invalide pour %s", assetName)
	}
	return nil
}

// sha256ForAsset extrait l'empreinte d'un fichier depuis un fichier
// SHA256SUMS (« hash  nom » ou « hash *nom »).
func sha256ForAsset(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.TrimPrefix(fields[len(fields)-1], "*") == name ||
			(fields[1] == name) {
			return fields[0], true
		}
	}
	return "", false
}

// applyLocalUpdate installe le tar.gz dans ~/.local, sans droits
// particuliers : même disposition que packaging/linux/install.sh.
func applyLocalUpdate(pkgPath, home string) error {
	dir, err := extractTarGz(pkgPath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	src, err := tarballRoot(dir)
	if err != nil {
		return err
	}

	local := filepath.Join(home, ".local")
	binDir := filepath.Join(local, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if err := installFile(filepath.Join(src, "notecore"), filepath.Join(binDir, "notecore"), 0o755); err != nil {
		return fmt.Errorf("binaire : %w", err)
	}

	appsDir := filepath.Join(local, "share", "applications")
	iconsDir := filepath.Join(local, "share", "icons", "hicolor", "256x256", "apps")
	metaDir := filepath.Join(local, "share", "metainfo")
	for _, d := range []string{appsDir, iconsDir, metaDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := installFile(filepath.Join(src, "notecore.png"), filepath.Join(iconsDir, "notecore.png"), 0o644); err != nil {
		return err
	}
	if err := installFile(filepath.Join(src, "notecore.metainfo.xml"), filepath.Join(metaDir, "notecore.metainfo.xml"), 0o644); err != nil {
		return err
	}

	// Le lanceur pointe vers le binaire installé (chemin absolu), comme
	// le fait install.sh — indifférent à la présence de ~/.local/bin
	// dans le PATH.
	desktop, err := os.ReadFile(filepath.Join(src, "notecore.desktop"))
	if err == nil {
		lines := strings.Split(string(desktop), "\n")
		for i, l := range lines {
			if strings.HasPrefix(l, "Exec=") {
				lines[i] = "Exec=" + filepath.Join(binDir, "notecore")
			}
		}
		if err := os.WriteFile(filepath.Join(appsDir, "notecore.desktop"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return err
		}
	}

	runBestEffort("gtk-update-icon-cache", "-q", "-t", filepath.Join(local, "share", "icons", "hicolor"))
	runBestEffort("update-desktop-database", appsDir)
	return nil
}

// applySystemUpdate réutilise install.sh --system sous pkexec : le
// paquet tar.gz extrait contient le script et tous les fichiers.
func applySystemUpdate(pkgPath string) error {
	dir, err := extractTarGz(pkgPath)
	if err != nil {
		return err
	}
	// Le dossier extrait doit survivre jusqu'à la fin du script : on le
	// laisse dans /tmp (nettoyé au prochain démarrage).
	keep := filepath.Join(os.TempDir(), "notecore-update-"+filepath.Base(dir))
	if err := os.Rename(dir, keep); err != nil {
		return err
	}
	root, err := tarballRoot(keep)
	if err != nil {
		return err
	}
	return runPrivileged("bash", filepath.Join(root, "install.sh"), "--system")
}

// applyMacUpdate remplace le .app courant par celui du zip téléchargé.
// Retourne le chemin du .app à rouvrir.
func applyMacUpdate(zipPath, exe string) (string, error) {
	dir, err := os.MkdirTemp("", "notecore-app-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	if out, err := exec.Command("ditto", "-x", "-k", zipPath, dir).CombinedOutput(); err != nil {
		return "", fmt.Errorf("extraction : %w (%s)", err, strings.TrimSpace(string(out)))
	}
	newApp, err := findAppBundle(dir)
	if err != nil {
		return "", err
	}

	// exe : .../NoteCore.app/Contents/MacOS/notecore → .app = 3 niveaux au-dessus.
	appPath := filepath.Clean(filepath.Join(exe, "..", "..", ".."))
	if !strings.HasSuffix(filepath.Base(appPath), ".app") {
		return "", fmt.Errorf("application .app introuvable autour de l'exécutable")
	}

	if err := replaceAppBundle(newApp, appPath); err != nil {
		return "", fmt.Errorf("remplacement de l'application : %w", err)
	}
	return appPath, nil
}

// findAppBundle cherche le premier *.app extrait.
func findAppBundle(root string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if found == "" && info.IsDir() && strings.HasSuffix(info.Name(), ".app") {
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("aucun .app dans l'archive")
	}
	return found, nil
}

// replaceAppBundle échange l'ancien .app contre le nouveau. Sans droits
// suffisants (ex. /Applications administré), on passe par osascript
// avec privilèges administrateur — une invite graphique, assumée.
func replaceAppBundle(newApp, appPath string) error {
	if err := os.RemoveAll(appPath); err != nil {
		// Droits insuffisants : script administrateur.
		script := filepath.Join(os.TempDir(), "notecore-replace-"+strings.TrimSuffix(filepath.Base(newApp), ".app")+".sh")
		content := fmt.Sprintf("#!/bin/bash\nset -e\nrm -rf %q\nditto %q %q\n", appPath, newApp, appPath)
		if werr := os.WriteFile(script, []byte(content), 0o755); werr != nil {
			return err
		}
		defer os.Remove(script)
		out, oerr := exec.Command("osascript", "-e",
			fmt.Sprintf("do shell script \"/bin/bash %s\" with administrator privileges", script),
		).CombinedOutput()
		if oerr != nil {
			return fmt.Errorf("%w (%s)", oerr, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return os.Rename(newApp, appPath)
}

// extractTarGz décompresse une archive .tar.gz dans un dossier
// temporaire et renvoie sa racine.
func extractTarGz(pkgPath string) (string, error) {
	f, err := os.Open(pkgPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("archive illisible : %w", err)
	}
	defer gz.Close()

	dest, err := os.MkdirTemp("", "notecore-pkg-")
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(dest)
			return "", err
		}
		// Garde-fou : jamais de chemin absolu ni de « .. » dans l'archive.
		clean := filepath.Clean(hdr.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			os.RemoveAll(dest)
			return "", fmt.Errorf("archive suspecte : %s", hdr.Name)
		}
		target := filepath.Join(dest, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				os.RemoveAll(dest)
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				os.RemoveAll(dest)
				return "", err
			}
			mode := os.FileMode(hdr.Mode & 0o777)
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				os.RemoveAll(dest)
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				os.RemoveAll(dest)
				return "", err
			}
			out.Close()
		}
	}
	return dest, nil
}

// tarballRoot trouve le dossier qui contient le binaire « notecore »
// dans une extraction (l'archive a une unique racine
// « notecore-vX.Y.Z-linux-x64/ »).
func tarballRoot(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(dir, entries[0].Name()), nil
	}
	if _, err := os.Stat(filepath.Join(dir, "notecore")); err == nil {
		return dir, nil
	}
	return "", fmt.Errorf("structure d'archive inattendue")
}

// installFile copie src vers dst de façon atomique (écriture d'un
// fichier voisin puis renommage) avec les droits demandés.
func installFile(src, dst string, mode os.FileMode) error {
	tmp := dst + ".new"
	if err := copyFile(src, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// runPrivileged exécute une commande via pkexec (invite graphique
// d'authentification) quand il existe, sinon sudo en mode non
// interactif — l'échec remonte avec la sortie de la commande.
func runPrivileged(name string, args ...string) error {
	tool := "pkexec"
	full := append([]string{name}, args...)
	if _, err := exec.LookPath("pkexec"); err != nil {
		tool = "sudo"
		full = append([]string{"-n"}, full...)
	}
	cmd := exec.Command(tool, full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return fmt.Errorf("%s%s", err, map[bool]string{true: " — " + msg, false: ""}[msg != ""])
	}
	return nil
}

func runBestEffort(name string, args ...string) {
	if _, err := exec.LookPath(name); err == nil {
		_ = exec.Command(name, args...).Run()
	}
}

// relaunchAndQuit démarre la nouvelle version puis quitte
// l'instance courante. Le nouveau processus ouvre la même base :
// SQLite/WAL tolère le bref chevauchement le temps de la fermeture.
func (n *NoteCore) relaunchAndQuit() {
	if exe, err := os.Executable(); err == nil {
		if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
			exe = resolved
		}
		_ = exec.Command(exe).Start()
	}
	wailsruntime.Quit(n.callContext())
}
