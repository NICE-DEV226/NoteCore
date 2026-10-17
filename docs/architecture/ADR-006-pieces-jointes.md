# ADR-006 - Pièces jointes locales (images et fichiers)

## Statut

Accepté pour la V1, en extension du périmètre initial (cahier des charges §10 les classait en évolutions futures ; décision explicite de l'utilisateur de les intégrer).

## Contexte

Une note doit pouvoir référencer une image ou un fichier présent sur la machine, l'afficher dans l'aperçu et rester exportable en Markdown lisible hors de NoteCore (cahier des charges §3.7, §6.5). Le CDC interdit de maintenir deux copies indépendantes d'une même donnée sans mécanisme de cohérence (§5.3) et impose que les sauvegardes restaurées ne perdent rien (§6.2) — y compris les pièces jointes.

## Décision

1. **Stockage** : les octets sont conservés dans SQLite, table `attachments` (`id`, `note_id`, `filename`, `mime`, `size`, `data BLOB`, `created_at`). Une pièce jointe appartient à **une seule** note ; `ON DELETE CASCADE` la supprime avec elle. Pas de déduplication par empreinte : la propriété doit rester simple et prévisible.
2. **Référence Markdown** : `![légende](attachment:<id>)` pour une image, `[nom](attachment:<id>)` pour un fichier. Le schéma `attachment:` n'est reconnu que par NoteCore ; il est **réécrit à l'export** vers un chemin relatif.
3. **Export** : chaque note exportée écrit ses pièces jointes dans un dossier frère `<nom>.assets/` et remplace `attachment:<id>` par un chemin relatif — le Markdown reste lisible ailleurs.
4. **Sauvegarde** : la table `attachments` fait partie du schéma (migration 2) ; `VACUUM INTO` la copie comme le reste, aucun format supplémentaire.
5. **Sécurité (§6.4)** : taille bornée par pièce jointe ; nom de fichier assaini ; type MIME normalisé ; à l'aperçu, une image n'est rendue que si son type est `image/*`, et jamais via `data:`/`javascript:`.

## Options considérées

- **Fichiers sur disque + table de métadonnées** : léger pour de gros volumes, mais crée deux sources (fichiers + base) que sauvegardes, renommages et suppressions doivent synchroniser — précisément ce que §5.3 déconseille.
- **Octets dans SQLite** : une seule copie, sauvegarde et restauration automatiques (ADR-004 inchangé), cohérence transactionnelle avec la note. Coût : la base grossit avec les pièces jointes, d'où la borne de taille.
- **Mettre les images en `data:` dans le Markdown** : autonome, mais alourdit la note et casse la lisibilité de la source.

## Raisons

La fiabilité et la simplicité de maintenance l'emportent : une seule copie sauvegardée comme le reste, une suppression de note qui emporte ses pièces jointes, et un Markdown qui reste la représentation portable du contenu.

## Conséquences

- `tablesBackup` et la restauration gagnent `attachments` ; une sauvegarde de schéma 1 (sans la table) reste restaurable — la table absente est simplement ignorée.
- L'aperçu doit résoudre les références de façon asynchrone (les octets viennent du backend).
- Une borne de taille par pièce jointe est nécessaire, et un dépassement est refusé avec un message lisible.
- Le nettoyage des pièces jointes orphelines est garanti par `ON DELETE CASCADE` (clés étrangères activées à l'ouverture, cf. ADR-005).
