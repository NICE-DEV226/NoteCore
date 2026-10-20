# ADR-009 - Dossiers imbriqués et rangement intelligent

## Statut

Accepté (V2.2). Complète le CDC V1 §3.2 (organisation) sans en changer les garanties : **ranger ne détruit jamais rien**.

## Contexte

Les dossiers étaient plats : impossible de refléter *Semestre 3 → Réseaux → TD*. Ranger une note se faisait par un petit menu dans l'éditeur, une note à la fois, et rien n'aidait à classer les notes laissées « sans dossier ». Ranger est une tâche répétitive que l'application peut simplifier sans rien décider à la place de l'utilisateur.

## Décision

### Dossiers imbriqués

1. **Schéma 4** : `folders.parent_id` (NULL = racine). Migration avec copie de sécurité préalable (ADR-004/V1 critère 12). L'unicité du nom devient **par parent** : deux matières ont chacune leur dossier « TD ».
2. **Profondeur bornée à 8 niveaux** ; déplacer un dossier **dans lui-même ou l'un de ses descendants est refusé** (le contrôle est dans le backend, pas seulement dans l'interface).
3. **Supprimer un dossier ne détruit rien** : ses notes **et** ses sous-dossiers montent d'un niveau, chez son parent (ou à la racine). Si un sous-dossier porte le même nom qu'un dossier du niveau supérieur, l'opération est refusée avec un message, plutôt que de renommer en silence.
4. **Voir un dossier montre aussi ses sous-dossiers** (« Réseaux » affiche les notes de « Réseaux › TD »). Le compteur est celui du sous-arbre.
5. **Ranger n'est pas modifier** : déplacer une note dans un dossier ne change pas sa date de modification (comme l'épinglage).
6. **Déplacement groupé tout-ou-rien** (`MoveNotes`, une transaction) : une note inconnue annule tout le lot.
7. **Données corrompues ou piégées** : les parcours de l'arborescence sont bornés (profondeur) et ne peuvent pas tourner à vide ; une sauvegarde dont les dossiers forment une boucle, ou sont trop profonds, est **refusée en bloc** à la restauration (la base courante reste intacte) plutôt que « réparée » en silence.
8. **Sauvegarde, export, partage** : la restauration rattache les parents après l'insertion de tous les dossiers (un enfant peut précéder son parent dans la table) ; l'export et le paquet `.notecore` portent le **chemin complet** (`folderPath`, un tableau : un nom peut contenir « / »), et l'import retrouve ou crée la chaîne de dossiers *par parent* — « Semestre 3 / TD » reçu est fusionné avec l'existant, jamais dupliqué.

### Interface

9. Arborescence repliable (état mémorisé localement), **glisser-déposer** de notes (une ou toute la sélection) et de dossiers, dépliage automatique d'un dossier survolé, zone « racine » visible pendant le glissement. **Sélection multiple** (Ctrl/Maj+clic).
10. **Alternative sans souris** : sélecteur de destination (« Ranger dans… », « Déplacer ce dossier… ») au clavier et au lecteur d'écran ; les destinations interdites y sont grisées avec la raison.

### Rangement intelligent (sans IA générative)

11. **Boîte « À classer »** : les notes sans dossier, avec pour chacune la meilleure proposition. Un clic range ; « Ranger les N sûres » range d'un coup celles dont la proposition est de confiance haute ; les autres restent à décider une à une. La proposition apparaît aussi dans l'éditeur d'une note sans dossier (acceptable ou ignorable).
12. **Méthode** : comparaison du vocabulaire de la note à celui des notes déjà rangées — TF-IDF (titre ×3, étiquettes ×4, début du texte), similarité cosinus avec le centre de chaque dossier (les sous-dossiers comptent à 40 %) — combinée à l'**affinité des étiquettes** (part des notes portant l'étiquette qui sont dans ce dossier, pondérée par le nombre d'exemples). Mots vides français et anglais retirés, accents ignorés. Tout est calculé localement, sur la base : aucun modèle, aucun réseau.
13. **Honnêteté** : une note trop courte ne reçoit rien ; une note hors sujet ne reçoit pas de proposition « fiable » ; une note déjà rangée est comparée **sans sa propre contribution** (elle ne se confirme pas elle-même) ; à égalité, le dossier le plus précis passe avant son parent. Chaque proposition **dit pourquoi** (étiquettes et mots en commun). Les niveaux de confiance sont « très probable » (score ≥ 0,45 et marge ≥ 0,08 sur la deuxième), « probable » (≥ 0,25) et « possible ».
14. **Jamais de rangement automatique non demandé** : l'application propose, l'utilisateur décide.

### Modèles de matière

14. **Une structure en un clic** : « Nouvelle matière » crée d'un coup *Réseaux* avec *Cours*, *TD*, *TP*, *Révisions*, *Examens* — ou un projet, un semestre, une année. La structure se décrit par un **texte indenté** (un niveau = deux espaces), que l'utilisateur modifie avant de valider ; `{nom}` est remplacé par le nom saisi. Un aperçu en direct montre l'arbre, signale les erreurs ligne par ligne et compte les dossiers à créer.
15. **Tout ou rien, sans doublon** : la création se fait en une transaction côté Go (`CreateFolderTree`) ; un dossier de même nom (casse ignorée) déjà présent au même endroit est **réutilisé**, pas dupliqué — appliquer deux fois le même modèle, ou un modèle sur une matière existante, complète la structure sans rien casser. Bornes vérifiées dans le backend : 200 dossiers, 100 caractères par nom, profondeur maximale en comptant celle du dossier d'accueil.
16. **Modèles personnels** : « Enregistrer comme modèle… » garde la structure dans les réglages locaux (un réglage par modèle, 6 au plus, texte de 380 caractères au plus pour tenir dans la limite de 512 octets d'un réglage). Ils se suppriment depuis la boîte. Aucun modèle n'est envoyé ni partagé.
17. Accès : le bouton de la section *Dossiers*, l'action *Ajouter une structure* d'un dossier (la structure s'y crée), et la palette de commandes. L'arbre se trie par nom : l'ordre du modèle n'est pas conservé.

## Options considérées

- **Étiquettes seules** : déjà là, mais elles ne structurent pas un cours ; les dossiers restent le modèle mental des étudiants.
- **Suggestion par modèle de langage** : exclue (CDC V1 §2.2) et contraire à « local et léger ».
- **Plongements (embeddings) locaux** : meilleurs sur les textes courts, mais ajoutent un modèle de dizaines de Mo à embarquer ; le TF-IDF suffit tant qu'il y a quelques notes par dossier, et reste explicable.
- **Un nom de dossier global unique** (ancien comportement) : incompatible avec l'imbrication.
- **Chemin des dossiers en texte (`A/B/C`)** dans l'export : ambigu si un nom contient « / » ; un tableau ne l'est pas.

## Conséquences

- Les propositions sont **faibles sur un dossier vide ou presque** et sur une note très courte : l'interface le dit (« pas de proposition ») plutôt que d'inventer.
- Le calcul relit toutes les notes (jusqu'à 20 000 caractères chacune) ; il est fait à la demande et regroupé après une saisie. Acceptable jusqu'à quelques milliers de notes ; au-delà, un index incrémental serait à prévoir.
- Une sauvegarde de schéma 4 ne peut plus être restaurée par une version antérieure (refus clair, ADR-004).
- La profondeur maximale et les règles de déplacement sont vérifiées côté Go : l'interface n'est pas la seule garde.
