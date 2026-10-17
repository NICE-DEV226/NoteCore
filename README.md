# NoteCore

[![CI](https://github.com/NICE-DEV226/NoteCore/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/NICE-DEV226/NoteCore/actions/workflows/ci.yml)

Prise de notes Markdown **locale et hors ligne**, pour étudiants et développeurs.

Notes, dossiers, tags, recherche plein texte, mise en forme complète avec aperçu, pièces jointes locales, exports Markdown et sauvegardes vérifiables. Pas de compte, pas de serveur, pas de synchronisation : la base SQLite locale est l'unique source de vérité.

## Sommaire

- [Fonctionnalités](#fonctionnalités)
- [Architecture](#architecture)
- [Prérequis](#prérequis)
- [Construire l'application](#construire-lapplication)
- [Développement](#développement)
- [Tests et intégration continue](#tests-et-intégration-continue)
- [Vos données](#vos-données)
- [Documentation](#documentation)
- [Licence](#licence)

## Fonctionnalités

**Organisation**

- Notes en Markdown, dossiers, tags colorés, épinglage.
- Recherche plein texte (SQLite FTS5) avec extraits où les termes trouvés sont surlignés.
- Enregistrement automatique après une courte pause, indicateur d'état (`enregistré`, `modifié`, `en cours`, `erreur`), plus `Ctrl/Cmd+S`.

**Écriture**

- Éditeur Markdown et aperçu côte à côte.
- Barre de mise en forme : titres, gras, italique, mise en évidence, code en ligne et en bloc, listes — à puces, ordonnée, cases à cocher —, citations, liens, images, séparateur, effacement de la mise en forme.
- Raccourcis `Ctrl/Cmd+B` (gras) et `Ctrl/Cmd+I` (italique) dans l'éditeur.
- Pièces jointes locales : une image ou un fichier se joint à une note, 10 Mio au maximum par pièce jointe.

**Confort**

- Thème clair ou sombre, aligné sur le système par défaut.
- Préférences d'affichage : police (serif, sans, monospace), taille du texte et largeur de colonne. Ces réglages sont **locaux et sans effet sur la note** — le Markdown reste portable.
- Accessibilité : navigation clavier, focus piégé dans les boîtes de confirmation, annonces `aria-live`, contrastes texte/fond tenus au-dessus de 4,5:1.

## Architecture

Application de bureau mono-utilisateur, conçue comme un **monolithe modulaire** : un seul processus, avec une frontière nette entre l'interface, les cas d'usage et la persistance. Aucun serveur distant, aucun compte, aucune synchronisation réseau.

```text
frontend/          Svelte 5 + TypeScript + Vite
                   Affichage, état de navigation et interactions.
                   Ne connaît ni SQL ni chemins système.

backend/           Go + Wails v2
  main.go          Fenêtre, embarquement du bundle, ouverture de la base
  internal/notes       Notes, dossiers, tags, pièces jointes
  internal/storage     Schéma et migrations SQLite
  internal/search      Recherche FTS5 et construction des extraits
  internal/transfer    Export, sauvegarde, validation, restauration
  internal/preferences Préférences locales
  internal/bindings    Méthodes exposées à Wails

packaging/         Éléments d'installation du bureau
docs/              Architecture et décisions techniques (ADR)
```

SQLite est l'unique source de vérité ; le frontend n'appelle que les méthodes Wails exposées par le backend. Le contenu des notes n'est jamais transformé en format propriétaire : c'est du Markdown, ce qui garantit qu'un export reste lisible sans NoteCore.

## Prérequis

| Outil | Version |
|---|---|
| Go | 1.27 ou plus récent (aligné sur `backend/go.mod`) |
| Node.js et npm | Node 20 ou plus récent |
| CLI Wails | v2 — `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Linux | `gcc`, `pkg-config`, GTK 3, WebKitGTK 4.1 |

Installation des dépendances système :

```bash
# Arch
pacman -S gcc pkgconf gtk3 webkit2gtk-4.1 npm

# Debian / Ubuntu
apt install gcc pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev npm
```

## Construire l'application

Une seule commande compile le frontend, l'incorpore au binaire et compile l'application :

```bash
cd backend
$(go env GOPATH)/bin/wails build
```

Le binaire est produit dans `backend/build/bin/notecore` :

```bash
./build/bin/notecore
```

> **Linux — WebKitGTK 4.0 contre 4.1.** Wails v2 interroge `pkg-config` sous le nom `webkit2gtk-4.0`, que les distributions récentes ne fournissent plus. Créez un alias local le temps de la compilation :
>
> ```bash
> pkgdir=$(mktemp -d)
> cp /usr/lib/pkgconfig/webkit2gtk-4.1.pc "$pkgdir/webkit2gtk-4.0.pc"
> PKG_CONFIG_PATH=$pkgdir wails build
> ```
>
> Le fichier est **copié, jamais réécrit** : son champ `Libs` doit continuer de pointer vers `-lwebkit2gtk-4.1`, seule bibliothèque réellement installée. Substituer le nom casserait l'édition de liens.

> **Ne lancez pas `wails build -s`.** `-s` saute la compilation du frontend, alors que le bundle est intégré au binaire Go par `go:embed` : un bundle périmé serait embarqué sans avertissement.

### Windows 10 / 11

La cible Windows n'utilise pas de cgo : elle se compile **depuis Linux**, sans chaîne de compilation croisée.

```bash
cd backend
CGO_ENABLED=0 $(go env GOPATH)/bin/wails build -platform windows/amd64
```

Le résultat, `backend/build/bin/notecore.exe`, est **autonome** : environ 15 Mo, interface embarquée, rien à installer à côté. Sur Windows, il suffit de double-cliquer.

> Windows a besoin du runtime **Microsoft Edge WebView2**, fourni d'origine avec Windows 11 et présent sur les Windows 10 à jour. S'il manque, l'application propose son installation au premier lancement.

L'intégration continue produit cet exécutable à chaque push sur `main` : onglet **Actions**, workflow **Windows**, artefact `notecore-windows-x64`. Une étiquette `v*` crée en plus un release GitHub avec le fichier attaché.

## Développement

Le frontend tourne seul, sur une API simulée — pratique pour travailler l'interface sans backend natif :

```bash
cd frontend
npm install
npm run dev        # http://localhost:5173
```

Le backend se teste indépendamment :

```bash
cd backend
go test ./...
```

Pour l'application complète avec rechargement, voir `docs/procedure-build.md`.

## Tests et intégration continue

| Commande | Portée |
| --- | --- |
| `cd backend && gofmt -l .` | formatage Go |
| `cd backend && go vet ./...` | analyse statique Go |
| `cd backend && go test ./...` | migrations, persistance, recherche, pièces jointes, export et sauvegarde |
| `cd frontend && npm run check` | types et composants Svelte (`svelte-check`) |

L'intégration continue rejoue ces contrôles, puis compile l'application complète avec Wails, à chaque push et pull request sur `main`. Voir `.github/workflows/ci.yml`.

## Vos données

Base locale SQLite, en mode WAL :

| Système | Emplacement |
| --- | --- |
| Linux | `~/.config/NoteCore/notecore.db` |
| macOS | `~/Library/Application Support/NoteCore/notecore.db` |
| Windows | `%APPDATA%\NoteCore\notecore.db` |

**Export.** Une note part en fichier Markdown UTF-8 ; ses pièces jointes sont écrites dans un dossier frère `<titre>-assets/` et les références sont réécrites en chemins relatifs, de sorte que le document reste lisible sans NoteCore. Plusieurs notes produisent une archive versionnée accompagnée d'un manifeste JSON.

**Sauvegarde et restauration.** Une sauvegarde est une copie cohérente de la base (checkpoint SQLite préalable), accompagnée de métadonnées de format et de la version de schéma. Une restauration est relue dans une zone temporaire, validée, présentée en aperçu des conflits, puis appliquée par remplacement atomique ou import explicite — jamais en écrasant silencieusement des données.

**Réseau.** L'application n'ouvre aucune connexion : ni compte, ni télémétrie, ni synchronisation. Seule exception, volontaire et image par image : une image insérée par URL est chargée depuis son hôte. Seuls les schémas `http` et `https` sont acceptés — `javascript:`, `data:` et `file:` sont refusés.

**Pièces jointes.** Les octets vivent dans la base (table `attachments`), donc sauvegardés et restaurés avec le reste, et supprimés avec leur note. Aucune copie externe, aucun service tiers.

## Documentation

- [`docs/architecture/ARCHITECTURE.md`](docs/architecture/ARCHITECTURE.md) — architecture, modèle de données, sauvegardes, sécurité
- [`docs/architecture/ADR-001`](docs/architecture/ADR-001-stack-et-style.md) à [`ADR-006`](docs/architecture/ADR-006-pieces-jointes.md) — décisions techniques et raisons
- [`docs/procedure-build.md`](docs/procedure-build.md) — build et développement en détail

## Licence

Apache 2.0 — voir [`LICENSE`](LICENSE).
