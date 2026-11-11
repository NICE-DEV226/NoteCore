package bindings

import (
	"strings"
	"testing"
)

func releaseWithAssets(names ...string) ghRelease {
	var rel ghRelease
	for _, n := range names {
		rel.Assets = append(rel.Assets, struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{Name: n, BrowserDownloadURL: "https://example.test/" + n})
	}
	return rel
}

func TestAssetSuffix(t *testing.T) {
	cases := map[string]string{
		"local": "-linux-x64.tar.gz", "system": "-linux-x64.tar.gz",
		"manual": "-linux-x64.tar.gz", "pacman": ".pkg.tar.zst",
		"deb": "-linux-amd64.deb", "windows": "-windows-x64-setup.exe",
		"macos": "-macos-universal.zip",
	}
	for method, want := range cases {
		if got := assetSuffix(method); got != want {
			t.Errorf("assetSuffix(%q) = %q, want %q", method, got, want)
		}
	}
	if assetSuffix("inconnu") != "" {
		t.Error("méthode inconnue : suffixe attendu vide")
	}
}

func TestDetectInstallMethod(t *testing.T) {
	home := "/home/test"
	if got := detectInstallMethod("linux", home+"/.local/bin/notecore", home); got != "local" {
		t.Errorf("install ~/.local : %q", got)
	}
	if got := detectInstallMethod("windows", `C:\Users\x\notecore.exe`, home); got != "windows" {
		t.Errorf("windows : %q", got)
	}
	if got := detectInstallMethod("darwin", "/Applications/NoteCore.app/Contents/MacOS/notecore", home); got != "macos" {
		t.Errorf("macos : %q", got)
	}
	// Hors ~/.local, /usr/local et sans paquet connu → manuel.
	if got := detectInstallMethod("linux", "/opt/notecore/notecore", home); got != "manual" {
		t.Errorf("inconnu : %q", got)
	}
}

func TestPlanUpdate(t *testing.T) {
	rel := releaseWithAssets(
		"notecore-v0.6.0-linux-x64.tar.gz",
		"notecore-v0.6.0-linux-amd64.deb",
		"notecore-0.6.0-1-x86_64.pkg.tar.zst",
		"notecore-v0.6.0-windows-x64-setup.exe",
		"notecore-v0.6.0-macos-universal.zip",
		"SHA256SUMS",
	)
	home := "/home/test"

	plan, err := planUpdate("linux", home+"/.local/bin/notecore", home, rel)
	if err != nil || plan.method != "local" || !strings.HasSuffix(plan.assetName, ".tar.gz") {
		t.Fatalf("plan local : %+v err=%v", plan, err)
	}

	// Le suffixe le plus spécifique gagne : un .deb ne doit pas être
	// confondu avec le tar.gz qui finit aussi par « .gz ».
	if _, err := planUpdate("linux", "/opt/notecore/notecore", home, rel); err != nil {
		t.Fatalf("plan manual : %v", err)
	}

	plan, err = planUpdate("windows", `C:\nc\notecore.exe`, home, rel)
	if err != nil || plan.method != "windows" || !strings.HasSuffix(plan.assetName, "-setup.exe") {
		t.Fatalf("plan windows : %+v err=%v", plan, err)
	}

	// Release sans l'artefact attendu → erreur explicite.
	if _, err := planUpdate("darwin", "/a/b", home, releaseWithAssets("notecore-v0.6.0-linux-x64.tar.gz")); err == nil {
		t.Fatal("artefact macOS absent : une erreur était attendue")
	}
}

func TestSHA256ForAsset(t *testing.T) {
	sums := "aaa111  notecore-v0.6.0-linux-x64.tar.gz\n" +
		"bbb222 *notecore-0.6.0-1-x86_64.pkg.tar.zst\n" +
		"ccc333  SHA256SUMS\n"
	if h, ok := sha256ForAsset(sums, "notecore-v0.6.0-linux-x64.tar.gz"); !ok || h != "aaa111" {
		t.Errorf("tar.gz : %q %v", h, ok)
	}
	if h, ok := sha256ForAsset(sums, "notecore-0.6.0-1-x86_64.pkg.tar.zst"); !ok || h != "bbb222" {
		t.Errorf("pkg.tar.zst (format binaire *) : %q %v", h, ok)
	}
	if _, ok := sha256ForAsset(sums, "absent.zip"); ok {
		t.Error("asset absent ne doit pas avoir d'empreinte")
	}
}
