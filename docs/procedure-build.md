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

L'intégration continue (`windows.yml`) produit cet exécutable à chaque
push sur `main` et l'attache à un release GitHub sur étiquette `v*`.

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
