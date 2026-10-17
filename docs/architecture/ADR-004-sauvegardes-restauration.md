# ADR-004 - Sauvegardes versionnées et restauration contrôlée

## Statut

Accepté pour la V1.

## Contexte

Une sauvegarde valide doit pouvoir être restaurée sans perte silencieuse, y compris après une interruption ou une base plus ancienne.

## Décision

Utiliser un format de backup versionné avec manifeste, validation préalable et restauration en deux étapes. Toute restauration destructive exige une confirmation explicite et une sauvegarde de l'état courant lorsque c'est possible.

## Options considérées

- Copier seulement le fichier SQLite : rapide, mais peu explicite et difficile à valider avant remplacement.
- Exporter uniquement les fichiers Markdown : portable, mais perd les préférences et certaines métadonnées.
- Archive versionnée avec manifeste : vérifiable, extensible et adaptée aux migrations.

## Raisons

La fiabilité et la compréhension par l'utilisateur priment sur la simplicité apparente d'une copie brute. Le manifeste permet de contrôler le format, le nombre d'éléments et l'intégrité avant modification des données courantes.

## Conséquences

- Le format de backup devient un contrat à versionner.
- Les restaurations doivent être testées avec des fichiers invalides, anciens et partiellement corrompus.
- Le remplacement atomique dépend des garanties du système de fichiers ; un fallback doit être prévu.
