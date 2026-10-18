# Procédure de build et de développement (livrable CDC §11, critère 12)

## Prérequis

- **Go 1.27+** (`go version`, aligné sur `backend/go.mod`)
- **Node 20+ et npm** (frontend)
- **CLI Wails v2.16.0** — version épinglée, identique à celle de la CI :
  ```
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
  ```
  > La CLI et la bibliothèque `go.mod` sont deux choses distinctes. Une CLI
  > plus récente que la lib affiche un avertissement et aligne `go.mod` au
  > premier lancement de `wails build` ou `wails dev` : le dépôt se
  > modifie alors sans que rien ne le signale (`git status`). Vérifiez
  > après chaque lancement, et rebasculez si besoin :
  > `git checkout -- backend/go.mod backend/go.sum`.
- Dépendances Linux (noms Arch, l'exemple doctor Debian demande `libwebkit2gtk-4.0`) :
  ```
  pacman -S gcc pkgconf gtk3 webkit2gtk-4.1 npm
  ```

## Frontend seul (référent visuel, mock API)

```
cd frontend
npm install
npm run dev      # http://localhost:5173
npm run check    # svelte-check
npm run build    # dist/
```

## Backend (tests)

```
cd backend
go test ./...
```

## Application de bureau complète (une seule commande)

Depuis `backend/`, `wails build` compile le frontend (script npm
`build:wails`), copie le bundle dans `frontenddist/` et compile l'app Go :

```
cd backend

# Arch : Wails v2 appelle pkg-config « webkit2gtk-4.0 » alors que le
# système fournit 4.1. Un shim local fait le lien — le fichier est copié
# tel quel, sans réécrire son champ Libs (qui doit rester -lwebkit2gtk-4.1).
pkgdir=$(mktemp -d)
cp /usr/lib/pkgconfig/webkit2gtk-4.1.pc "$pkgdir/webkit2gtk-4.0.pc"

PKG_CONFIG_PATH=$pkgdir $(go env GOPATH)/bin/wails build
```

Le binaire est `backend/build/bin/notecore` ; le lancer :

```
./build/bin/notecore
```

> Le bundle frontend est intégré par `go:embed` : si l'on compile le
> frontend à la main (`npm run build`), il faut le recopier
> (`npm run build:wails` fait build + copie). Ne jamais lancer
> `wails build -s` en croyant que le frontend est à jour — `-s` saute
> justement la compilation du frontend.

La base de données vit dans le répertoire utilisateur :

- Linux : `~/.config/NoteCore/notecore.db`
- Windows : `%APPDATA%\NoteCore\notecore.db`
- macOS : `~/Library/Application Support/NoteCore/notecore.db`

## Version Windows 10 / 11

La cible Windows n'utilise pas de cgo (le WebView2Loader est en Go pur) :
elle se compile depuis Linux, sans chaîne de compilation croisée.

```
cd backend
CGO_ENABLED=0 $(go env GOPATH)/bin/wails build -platform windows/amd64
```

Le résultat est `backend/build/bin/notecore.exe` : un exécutable autonome
d'environ 15 Mo, interface embarquée par `go:embed`. Rien à installer à
côté, il suffit de le double-cliquer.

Deux points à connaître :

- L'exécutable requiert le runtime **Microsoft Edge WebView2**. Il est
  fourni d'origine avec Windows 11 et présent sur les Windows 10 à jour ;
  sinon l'application propose son installation.
- L'icône de l'exécutable vient de `build/windows/icon.ico` (générée
  depuis `assets/notecore.png`) — fichier suivi par git, contrairement
  aux sorties de `build/bin/`.

L'intégration continue (`release.yml`) produit cet exécutable à chaque
push sur `main` et l'attache à un release GitHub sur étiquette `v*`.

## Paquet Arch

Le projet n'est pas publié sur l'AUR (pas de compte de mainteneur) : le
`PKGBUILD` se construit depuis le dépôt. Il est versionné dans `packaging/aur/PKGBUILD`. Il construit
le paquet sans CLI Wails :

- `prepare()` récupère les dépendances Go (`go mod download`) et npm
  (`npm ci`), verrouillées par `go.sum` et `package-lock.json`. Le
  `build()` tourne donc sans réseau.
- `build()` compile le frontend (`npm run build:wails`, qui copie `dist/`
  dans `backend/frontenddist`), puis `go build -tags "desktop production"`.

Deux points non évidents, tous deux essentiels :

1. **Le shim `webkit2gtk-4.0`.** Le composant cgo de Wails réclame
   `webkit2gtk-4.0` par pkg-config, alors que le système ne fournit que la
   série 4.1 (voir plus haut). `build()` copie donc le `.pc` de 4.1 sous
   le nom 4.0 dans un répertoire éphémère et l'exporte via
   `PKG_CONFIG_PATH`. Le champ `Libs` du fichier copié reste inchangé :
   c'est bien `libwebkit2gtk-4.1` qui est liée. Sans ce shim, la
   compilation s'arrête sur `Package webkit2gtk-4.0 was not found`.
   Attention : un cache Go antérieur (alimenté par un shim local) peut
   masquer le problème lors d'un test manuel — vérifier avec
   `GOCACHE=$(mktemp -d) go build ...`.
2. **Les tags.** `go build` sans `-tags "desktop production"` ne refuse
   pas de compiler : il produit un exécutable qui s'arrête aussitôt avec
   « Wails applications will not build without the correct build tags ».

Le nom du répertoire extrait est `NoteCore-<version>` (nom amont), d'où la
variable `_pkgname` — le nom du paquet, lui, reste en minuscules.

Installation et test locaux :

```bash
cd packaging/aur
makepkg -si        # -i installe, nécessite sudo pour les dépendances
```

L'icône installée est `backend/assets/notecore.png` (256×256) dans
`hicolor/256x256`, et le lanceur est `packaging/notecore.desktop`.

## Développement à chaud

- Backend Go : `go test ./...` couvre migrations, persistance, recherche FTS
  et règles de suppression — il faut tous les tests verts avant de commit.
- Interface en mode développement avec backend natif : exécuter
  `wails dev` une fois les dépendances système installées (le chemin
  `frontend/dist` est reconstruit automatiquement).
  `backend/wails.json` doit définir `frontend:dev:watcher` (`npm run dev`) :
  sans lui, `wails dev` échoue à démarrer le serveur de développement.
  Le serveur Vite s'affiche alors sur `http://localhost:5173` et l'appel
  des méthodes Go depuis le navigateur sur `http://localhost:34115`.


## Installateurs et paquets (livrables d'installation)

Tout est produit par `.github/workflows/release.yml` (étiquette `v*`, ou
« Run workflow » pour essayer sans publier). Les fichiers sont décrits ici pour
pouvoir les reconstruire à la main.

### Windows : installateur (NSIS) et exécutable

```
sudo apt install nsis                # Debian/Ubuntu ; makensis est requis
cd backend
CGO_ENABLED=0 $(go env GOPATH)/bin/wails build -platform windows/amd64 -clean -nsis -webview2 embed
```

- `build/bin/notecore.exe` : exécutable autonome (installe WebView2 au premier
  lancement s'il manque, grâce à `-webview2 embed`).
- `build/bin/NoteCore-amd64-installer.exe` : installateur. Sa source est
  `build/windows/installer/project.nsi` (installation par utilisateur, sans
  UAC, interface en français, case « Lancer NoteCore »). `wails_tools.nsh` et
  `tmp/` sont régénérés à chaque build et ne sont pas suivis par git.
- La désinstallation retire le programme et le cache WebView2, **jamais**
  `%APPDATA%\NoteCore` (les notes).

### Linux : archive, .deb, paquet Arch

- **Archive** : binaire + `install.sh` + `notecore.desktop` + icône + metainfo
  (`packaging/linux/install.sh`). Installation utilisateur sans `sudo`.
- **.deb** : `VERSION=0.2.0 nfpm package -p deb -f packaging/nfpm.yaml -t dist/`
  (nfpm : `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.46.3`).
  Dépend de `libgtk-3-0` et `libwebkit2gtk-4.1-0`.
- **Arch** : `packaging/aur/PKGBUILD` (le projet n'est pas publié sur l'AUR ; le
  fichier se construit depuis le dépôt avec `makepkg -si`). Après toute modification :
  `cd packaging/aur && makepkg --printsrcinfo > .SRCINFO`. À chaque nouvelle
  version, mettre à jour `pkgver` et `sha256sums` (`updpkgsums`).
