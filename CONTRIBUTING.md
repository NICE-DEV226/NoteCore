# Guide de contribution à NoteCore

Merci de l'intérêt que vous portez à NoteCore ! Les contributions de la communauté sont les bienvenues pour améliorer l'application, corriger des bugs ou enrichir la documentation.

Ce document rassemble les informations et consignes nécessaires pour contribuer efficacement au projet.

---

## Sommaire

- [Code de conduite](#code-de-conduite)
- [Comment contribuer ?](#comment-contribuer)
  - [Rapporter un problème (Bug)](#rapporter-un-problème-bug)
  - [Proposer une fonctionnalité](#proposer-une-fonctionnalité)
  - [Soumettre une Pull Request](#soumettre-une-pull-request)
- [Environnement de développement](#environnement-de-développement)
  - [Prérequis](#prérequis)
  - [Frontend Svelte](#frontend-svelte)
  - [Backend Go](#backend-go)
  - [Application complète (Wails)](#application-complète-wails)
- [Contrôles et tests](#contrôles-et-tests)
- [Conventions de code](#conventions-de-code)
- [Documentation et architecture](#documentation-et-architecture)

---

## Code de conduite

Nous visons un environnement accueillant, respectueux et constructif. Veuillez faire preuve de courtoisie et de bienveillance lors de vos échanges (issues, Pull Requests, discussions).

---

## Comment contribuer ?

### Rapporter un problème (Bug)

Avant d'ouvrir une issue pour un bug :
1. Vérifiez dans les issues ouvertes si le problème n'a pas déjà été signalé.
2. Assurez-vous d'utiliser la dernière version publiée ou la branche `main`.

Lors de la rédaction de l'issue, veillez à préciser :
- La version de NoteCore et votre système d'exploitation (Windows, Linux, etc.).
- Les étapes précises pour reproduire le comportement observé.
- Le comportement attendu vs le comportement obtenu.
- Les captures d'écran ou journaux d'erreurs utiles, le cas échéant.

### Proposer une fonctionnalité

Les propositions d'évolution sont encouragées ! Pour toute modification majeure de l'interface ou des fonctionnalités :
1. Ouvrez une issue de discussion pour présenter votre idée avant de vous lancer dans le développement.
2. Expliquez le besoin utilisateur et le cas d'usage visé.

### Soumettre une Pull Request

1. **Forkez** le dépôt et créez une branche dédiée à votre fonctionnalité ou correctif (`git checkout -b feature/ma-fonctionnalite` ou `fix/mon-correctif`).
2. **Développez** vos modifications en suivant les conventions du projet.
3. **Exécutez les tests** et contrôles automatiques (formatage, types, tests unitaires).
4. **Commitez** vos changements avec des messages de commit clairs et explicites.
5. **Ouvrez une Pull Request** vers la branche `main` du dépôt principal en décrivant vos changements.

---

## Environnement de développement

### Prérequis

- **Go** : v1.27 ou plus récent (aligné sur `backend/go.mod`).
- **Node.js et npm** : Node v20 ou plus récent.
- **Wails CLI** : v2 (ex. `v2.16.0`).
- **Linux (dépendances système)** : `gcc`, `pkg-config`, GTK 3, WebKitGTK 4.1 (`webkit2gtk-4.1`).

Exemple d'installation des dépendances système sous Linux :

```bash
# Arch Linux / Manjaro
sudo pacman -S gcc pkgconf gtk3 webkit2gtk-4.1 npm

# Debian / Ubuntu (22.04+)
sudo apt install gcc pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev npm
```

Pour l'outil Wails CLI :

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
```

### Frontend Svelte

Le frontend (`frontend/`) utilise **Svelte 5**, **TypeScript** et **Vite**. Il peut être exécuté indépendamment avec des données simulées (mock API) pour travailler sur l'interface :

```bash
cd frontend
npm install
npm run dev      # Serveur de dev sur http://localhost:5173
```

### Backend Go

Le backend (`backend/`) est écrit en Go et gère la base de données SQLite (FTS5), les sauvegardes/exports et les communications réseau locales.

Pour exécuter les tests du backend :

```bash
cd backend
go test ./...
```

### Application complète (Wails)

Pour exécuter l'application complète en mode développement avec rechargement à chaud :

```bash
cd backend
wails dev
```

Pour compiler l'application de bureau en mode release :

```bash
cd backend
wails build
```

Le binaire produit se trouve dans `backend/build/bin/notecore` (ou `notecore.exe` sous Windows).

> **Note sous Linux :** Si Wails recherche `webkit2gtk-4.0`, créez un shim temporaire pointant vers la version `4.1` installée sur votre système (voir `docs/procedure-build.md` pour le détail).

---

## Contrôles et tests

Avant de soumettre votre PR, assurez-vous que l'ensemble de la suite de vérification passe avec succès :

| Domaine | Commande | Description |
| --- | --- | --- |
| **Go** | `cd backend && gofmt -l .` | Vérification du formatage Go (ne doit rien retourner) |
| **Go** | `cd backend && go vet ./...` | Analyse statique Go |
| **Go** | `cd backend && go test ./...` | Tests unitaires et d'intégration Go |
| **Frontend** | `cd frontend && npm run check` | Vérification des types et composants Svelte (`svelte-check`) |
| **Frontend** | `cd frontend && npm test` | Tests unitaires du frontend |

---

## Conventions de code

- **Go** : Respectez le formatage standard (`gofmt`). Codez en suivant les règles idomatiques de Go. Les commentaires et la documentation dans le code peuvent être rédigés en français.
- **Frontend** : Respectez la vérification des types TypeScript (`svelte-check`). Gardez les composants Svelte lisibles et modulaires.
- **Accessibilité** : L'interface doit rester entièrement accessible au clavier, respecter des ratios de contraste supérieurs à 4,5:1 et inclure les attributs ARIA nécessaires.
- **Architecture** : Ne contournez pas l'isolation entre frontend et backend. Le frontend communique exclusivement via les bindings exposés par Wails, et SQLite reste l'unique source de vérité.

---

## Documentation et architecture

Avant d'effectuer des modifications architecturales, veuillez consulter la documentation dans `docs/` :

- [`docs/architecture/ARCHITECTURE.md`](docs/architecture/ARCHITECTURE.md) : Modèle de données, persistance et sécurité.
- [`docs/architecture/`](docs/architecture/) : Décisions d'architecture (ADR-001 à ADR-009).
- [`docs/procedure-build.md`](docs/procedure-build.md) : Détails sur la procédure de compilation et d'empaquetage.
