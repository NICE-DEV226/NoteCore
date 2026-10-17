# ADR-002 - SQLite comme source de vérité

## Statut

Accepté pour la V1.

## Contexte

Le contenu Markdown peut être placé dans des fichiers ou directement dans SQLite. L'autosauvegarde, la restauration et l'export doivent rester fiables sans maintenir deux copies actives.

## Décision

Conserver le contenu Markdown et les métadonnées dans SQLite. Produire les fichiers Markdown uniquement lors d'un export explicite.

## Options considérées

- Fichiers Markdown + index SQLite : lisibilité native, mais gestion complexe des renommages, suppressions, modifications externes et cohérence.
- SQLite uniquement : transactions simples et sauvegardes cohérentes.
- Modèle hybride avec synchronisation bidirectionnelle : souplesse maximale, mais conflits et risques de perte plus élevés.

## Raisons

La V1 privilégie la conservation fiable des données et la simplicité de maintenance. L'export conserve la portabilité sans faire de fichiers un état concurrent.

## Conséquences

- Les notes ne sont pas directement éditables par un autre outil sans export/import.
- Le backup doit protéger le fichier SQLite et vérifier sa cohérence.
- Les migrations doivent préserver le contenu texte sans transformation implicite.
