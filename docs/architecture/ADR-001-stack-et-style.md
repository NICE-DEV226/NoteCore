# ADR-001 - Stack et style d'architecture

## Statut

Accepté pour la V1, sous réserve d'un prototype de build.

## Contexte

NoteCore doit être une application de bureau offline-first, légère, multi-plateforme et adaptée à une petite équipe. Les fonctions essentielles ne doivent dépendre d'aucun service distant.

## Décision

Utiliser Wails v2 avec un backend Go et une interface Svelte + TypeScript. Organiser le backend en monolithe modulaire, avec des services applicatifs et des repositories séparés.

## Options considérées

- Application web avec serveur local : plus de composants à installer et à opérer.
- Electron : écosystème riche, mais empreinte mémoire plus élevée pour une application de notes locale.
- Tauri : option pertinente, mais elle introduit Rust alors que le cahier des charges propose Go.
- Wails v2 : binaire desktop léger, accès natif en Go et frontend web moderne.

## Raisons

Cette option limite l'infrastructure, conserve une séparation nette entre interface et logique métier, et reste cohérente avec les technologies proposées. Le monolithe modulaire évite le coût opérationnel des microservices pour une application mono-utilisateur.

## Conséquences

- Le contrat entre Svelte et Go doit être stable et testé.
- La compatibilité des versions Wails, Go et Node doit être documentée.
- Une montée en charge serveur future ne sera pas automatique ; les services métier devront rester découplés des bindings Wails.
