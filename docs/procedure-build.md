# Procédure de build et de développement (livrable CDC §11, critère 12)

## Prérequis

- **Go 1.27+** (`go version`, aligné sur `backend/go.mod`)
- **Node 20+ et npm** (frontend)
- **CLI Wails v2** :
  ```
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```
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
# système fournit 4.1. Un shim local fait le lien :
pkgdir=$(mktemp -d)
cp /usr/lib/pkgconfig/webkit2gtk-4.1.pc "$pkgdir/webkit2gtk-4.0.pc"
sed -i 's/Name: webkit2gtk-4.1/Name: webkit2gtk-4.0/' "$pkgdir/webkit2gtk-4.0.pc"

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

- Linux  : `~/.config/NoteCore/notecore.db`
- Windows : `%APPDATA%\NoteCore\notecore.db`
- macOS  : `~/Library/Application Support/NoteCore/notecore.db`

## Développement auquel du chaud

- Backend Go : `go test ./...` couvre migrations, persistance, recherche FTS
  et règles de suppression — il faut tous les tests verts avant de commit.
- Interface en mode développement avec backend natif : exécuter
  `wails dev` une fois les dépendances système installées (le chemin
  `frontend/dist` est reconstruit automatiquement).
