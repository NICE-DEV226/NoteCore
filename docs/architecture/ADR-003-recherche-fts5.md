# ADR-003 - Recherche locale avec SQLite FTS5

## Statut

Accepté pour la V1.

## Contexte

La recherche doit fonctionner hors ligne sur le titre, le contenu et les tags, avec plusieurs milliers de notes et sans moteur sémantique.

## Décision

Utiliser une table SQLite FTS5 synchronisée avec les notes dans la même transaction. Le backend construit les extraits et renvoie au frontend des résultats déjà structurés.

## Options considérées

- Recherche SQL avec `LIKE` : simple, mais moins robuste et moins performante sur le contenu.
- Index FTS5 : inclus dans SQLite, local, rapide et adapté à la V1.
- Moteur externe ou recherche sémantique : complexité et dépendances inutiles au périmètre actuel.

## Conséquences

- Les migrations doivent reconstruire l'index si son schéma change.
- Les tests doivent couvrir accents, ponctuation, code et contenu vide.
- La recherche reste lexicale ; les relations sémantiques sont une évolution future.
