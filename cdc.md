# CAHIER DES CHARGES — NOTECORE V1

**Version :** 1.0  
**Statut :** Spécification initiale  
**Type de produit :** Application de bureau de prise de notes  
**Cible :** Étudiants, développeurs et apprenants en informatique

## 1. Présentation du projet

### 1.1. Contexte

Les étudiants et développeurs utilisent souvent plusieurs outils pour conserver leurs cours, documenter leurs projets, noter des idées, mémoriser des notions techniques et organiser leurs apprentissages.

Une simple application de notes permet de stocker des informations, mais ne facilite pas nécessairement leur organisation, leur recherche et leur réutilisation dans le temps.

NoteCore vise à proposer une application de prise de notes structurée, rapide, utilisable hors ligne et adaptée aux contenus techniques : cours, documentation, extraits de code, algorithmes et notes de projet.

### 1.2. Vision du produit

NoteCore doit devenir progressivement un espace de travail personnel permettant de prendre des notes, de les organiser, de les retrouver et, dans les versions futures, de les transformer en supports de révision et en exercices pratiques.

La V1 doit établir une base fiable avant d'introduire des fonctionnalités pédagogiques plus avancées.

### 1.3. Objectifs de la V1

- Créer, modifier, organiser et retrouver facilement des notes.
- Proposer une interface graphique simple, cohérente et agréable.
- Faciliter la rédaction de cours et de contenus techniques.
- Enregistrer automatiquement les modifications.
- Conserver les données localement et permettre leur exportation.
- Préparer une architecture extensible pour les futures fonctionnalités de révision et de challenges.

### 1.4. Public cible

- Étudiants en informatique et en génie logiciel.
- Développeurs débutants et expérimentés.
- Étudiants souhaitant centraliser leurs cours et leurs documents de travail.
- Utilisateurs recherchant un outil de notes utilisable sans connexion Internet.

## 2. Périmètre fonctionnel

### 2.1. Fonctionnalités incluses

La V1 comprend :

- La gestion des notes.
- L'organisation par dossiers.
- La recherche textuelle.
- Les favoris et les étiquettes.
- Un éditeur de texte compatible Markdown.
- La sauvegarde automatique.
- Le thème clair et le thème sombre.
- Des raccourcis clavier essentiels.
- L'exportation des notes en Markdown.
- La sauvegarde et la restauration des données.

### 2.2. Fonctionnalités exclues de la V1

Pour éviter une première version trop complexe, les fonctionnalités suivantes ne sont pas prévues :

- Comptes utilisateurs et authentification.
- Synchronisation cloud et collaboration en temps réel.
- Intelligence artificielle générative.
- Système complet de flashcards et de répétition espacée.
- Éditeur de code avec exécution et correction automatique.
- Système de challenges avec classement et statistiques.
- Graphe complexe de connaissances et recommandations automatiques.
- Application mobile.
- Marketplace d'extensions.

Ces fonctionnalités pourront être étudiées après validation de la V1.

## 3. Spécifications fonctionnelles

### 3.1. Gestion des notes

L'utilisateur doit pouvoir :

- Créer une nouvelle note.
- Modifier le titre et le contenu d'une note.
- Supprimer une note après confirmation.
- Consulter la date de création et la date de dernière modification.
- Épingler une note pour y accéder rapidement.
- Retrouver une note récemment modifiée.
- Écrire et conserver des contenus contenant des accents, des listes, des liens et du code.

Chaque note possède au minimum :

- Un identifiant unique.
- Un titre.
- Un contenu.
- Un dossier de rattachement facultatif.
- Une date de création.
- Une date de modification.
- Un état favori ou épinglé.
- Une liste d'étiquettes facultative.

Les notes sans titre explicite doivent rester accessibles et recevoir un titre par défaut jusqu'à leur renommage.

### 3.2. Organisation

L'utilisateur doit pouvoir :

- Créer et renommer des dossiers.
- Supprimer un dossier sans supprimer accidentellement ses notes.
- Déplacer une note d'un dossier à un autre.
- Consulter toutes ses notes.
- Consulter uniquement ses favoris.
- Filtrer ses notes par étiquette.
- Retrouver rapidement les notes récemment modifiées.

Les règles de suppression d'un dossier doivent être explicites. La suppression d'un dossier ne doit jamais entraîner une perte silencieuse des notes qu'il contient.

### 3.3. Recherche

La recherche doit permettre de retrouver une note à partir :

- De son titre.
- De mots présents dans son contenu.
- D'une étiquette, lorsque cette fonctionnalité est utilisée.

Les résultats doivent afficher le titre et un extrait pertinent du contenu.

La recherche doit être disponible depuis l'interface principale et ne doit pas nécessiter de connexion Internet.

Pour la V1, une recherche textuelle fiable suffit. Aucun moteur sémantique ou système d'intelligence artificielle n'est nécessaire.

### 3.4. Éditeur

L'éditeur doit permettre la rédaction de notes avec les éléments suivants :

- Texte normal.
- Titres et sous-titres.
- Listes ordonnées et non ordonnées.
- Liens.
- Blocs de code.
- Mise en évidence du texte lorsque prise en charge par l'éditeur choisi.

Le contenu doit être compatible avec Markdown.

L'interface peut proposer un aperçu du rendu Markdown. L'édition et la prévisualisation doivent rester simples et ne pas transformer l'application en environnement de développement complet.

Le contenu Markdown doit être conservé sans perte lors de l'enregistrement, de la réouverture et de l'exportation.

Des images et des fichiers locaux peuvent être joints à une note. Une pièce jointe est stockée localement, dans les données de l'application, sans service externe ni connexion réseau. Elle appartient à une seule note et la suit : supprimer la note emporte ses pièces jointes. Un volume maximum est appliqué par pièce jointe. Le Markdown exporté reste lisible sans NoteCore.

### 3.5. Sauvegarde automatique

L'application doit enregistrer automatiquement les modifications après une courte période sans saisie.

Le mécanisme doit :

- Éviter une écriture en base de données à chaque caractère.
- Afficher clairement l'état de sauvegarde.
- Signaler une erreur lorsque l'enregistrement échoue.
- Éviter qu'une fermeture normale entraîne la perte des dernières modifications confirmées comme enregistrées.

En cas de fermeture brutale, l'application doit limiter autant que possible la perte de données et signaler tout contenu dont la sauvegarde n'a pas pu être confirmée.

### 3.6. Apparence et préférences

L'application doit proposer :

- Un thème clair.
- Un thème sombre.
- Une interface adaptée aux tailles de fenêtre courantes.
- Des raccourcis clavier pour créer, rechercher et enregistrer une note.
- Une préférence de thème conservée après redémarrage.

Les préférences sont stockées localement.

### 3.7. Exportation et restauration

L'utilisateur doit pouvoir :

- Exporter une note individuelle en fichier Markdown.
- Exporter plusieurs notes ou un ensemble de notes.
- Effectuer une sauvegarde de ses données.
- Restaurer une sauvegarde valide.

Le format d'exportation doit rester lisible sans NoteCore. Les pièces jointes sont exportées à côté de leurs notes, dans un dossier associé, et les références sont réécrites en chemins relatifs.

La restauration doit être validée avant application et ne doit pas écraser silencieusement des données existantes. Elle doit conserver les pièces jointes déjà présentes dans la sauvegarde.

## 4. Spécifications de l'interface utilisateur

### 4.1. Principes de conception

L'interface doit privilégier :

- La lisibilité.
- La rapidité d'accès aux notes.
- Une hiérarchie visuelle claire.
- Un nombre limité d'éléments visibles simultanément.
- Des interactions prévisibles.
- Une utilisation confortable pendant de longues sessions de rédaction.

L'application ne doit pas ressembler à un terminal, à un tableau de bord surchargé ou à un éditeur de code complet.

### 4.2. Structure de la fenêtre principale

L'interface est organisée en trois zones.

**Zone A — Barre latérale**

Elle contient :

- Le nom et l'identité visuelle de NoteCore.
- Le bouton de création d'une note.
- Les accès à toutes les notes, aux favoris et aux notes récentes.
- La liste des dossiers.
- L'accès aux étiquettes.
- Les paramètres.

**Zone B — Liste des notes**

Elle contient :

- Le titre de chaque note.
- Un extrait du contenu.
- La date de modification.
- Un indicateur de favori si nécessaire.
- La recherche et les filtres.

Cette zone peut afficher un état vide lorsqu'aucune note ne correspond à la recherche.

**Zone C — Éditeur**

Elle contient :

- Le titre modifiable.
- L'éditeur Markdown.
- Les commandes de mise en forme essentielles.
- L'indication de l'état de sauvegarde.
- Les informations secondaires utiles, sans encombrer l'espace de rédaction.

### 4.3. États de l'interface

Les états suivants doivent être prévus :

- Aucune note créée.
- Aucune note sélectionnée.
- Note en cours de modification.
- Recherche sans résultat.
- Sauvegarde en cours.
- Sauvegarde réussie.
- Erreur de sauvegarde.
- Confirmation avant suppression.
- Erreur de restauration ou d'importation.

Chaque état doit être compréhensible sans nécessiter de connaissances techniques.

## 5. Architecture et choix techniques

### 5.1. Technologies proposées

- **Application de bureau :** Wails v2.
- **Langage backend :** Go.
- **Interface :** Svelte et TypeScript.
- **Stockage structuré :** SQLite.
- **Format de contenu :** Markdown.
- **Styles :** CSS ou une solution légère compatible avec Svelte.

Ces technologies constituent la proposition de référence pour la V1. Leur choix définitif doit être confirmé avant l'implémentation.

### 5.2. Responsabilités des composants

**Frontend — Svelte et TypeScript**

- Affichage de l'interface.
- Navigation entre les vues.
- Gestion des interactions utilisateur.
- Éditeur et aperçu Markdown.
- Recherche et filtres dans l'interface.
- Affichage des états de sauvegarde et des erreurs.

**Backend — Go**

- Gestion des notes et des dossiers.
- Validation des opérations.
- Lecture et écriture des données.
- Gestion des sauvegardes et des exportations.
- Accès contrôlé à SQLite et au système de fichiers.

**SQLite**

- Stockage des métadonnées des notes.
- Gestion des dossiers, étiquettes et favoris.
- Indexation des données nécessaires à la recherche.
- Conservation des préférences et des informations de l'application.

**Markdown**

- Conservation du contenu des notes dans un format portable.
- Préservation des titres, listes, liens et blocs de code.
- Facilitation de l'exportation et d'une éventuelle migration.

### 5.3. Organisation du stockage

Le modèle de stockage doit être défini avant le développement.

Une approche possible consiste à conserver les contenus Markdown dans des fichiers locaux et les métadonnées dans SQLite.

Cette approche impose de gérer correctement les renommages, les suppressions, les déplacements, les sauvegardes et les éventuelles modifications externes.

Une autre possibilité est de conserver le contenu directement dans SQLite et de proposer l'exportation Markdown.

Le choix final doit privilégier la fiabilité et la simplicité de maintenance. La V1 ne doit pas maintenir deux copies indépendantes d'une même note sans mécanisme de cohérence clairement défini.

### 5.4. Organisation logique du code

Le projet doit séparer au minimum :

- La gestion des notes.
- La gestion des dossiers et des étiquettes.
- La recherche.
- Le stockage et les migrations de données.
- L'exportation et la restauration.
- Les préférences de l'application.
- L'interface utilisateur.

Les opérations de stockage ne doivent pas être dispersées dans les composants de l'interface.

## 6. Exigences non fonctionnelles

### 6.1. Fonctionnement hors ligne

Les fonctions essentielles doivent fonctionner sans accès à Internet.

Aucun compte distant ne doit être nécessaire pour créer, consulter, modifier ou rechercher des notes.

### 6.2. Fiabilité

- Les notes doivent être conservées après fermeture et redémarrage.
- Les opérations destructrices doivent être contrôlées.
- Les erreurs d'écriture doivent être visibles.
- Les migrations de base de données doivent préserver les données existantes.
- Les sauvegardes doivent pouvoir être vérifiées avant restauration.

### 6.3. Performance

L'application doit rester fluide avec plusieurs milliers de notes contenant des textes de taille raisonnable.

La recherche et l'ouverture d'une note ne doivent pas bloquer durablement l'interface.

Les performances exactes seront évaluées sur une machine de référence lors des tests.

### 6.4. Sécurité et confidentialité

- Les données sont conservées localement par défaut.
- Aucune transmission automatique du contenu des notes vers un serveur distant.
- Les entrées importées doivent être validées.
- Le rendu des contenus Markdown doit prévenir l'exécution de contenu actif non autorisé.
- Les erreurs techniques ne doivent pas exposer inutilement le contenu des notes.

### 6.5. Portabilité

Les notes exportées doivent pouvoir être lues par d'autres éditeurs Markdown.

La structure des données doit faciliter une future migration vers d'autres versions de NoteCore.

## 7. Modèle de données initial

Les entités principales sont :

**Note**
- id
- title
- content ou content_path, selon le choix de stockage
- folder_id, facultatif
- is_pinned
- created_at
- updated_at

**Folder**
- id
- name
- created_at
- updated_at

**Tag**
- id
- name

**NoteTag**
- note_id
- tag_id

**Setting**
- key
- value

Les contraintes d'intégrité, les index et les règles de suppression devront être définis dans le schéma SQLite.

Le modèle doit rester suffisamment simple pour la V1 tout en permettant l'ajout futur de métadonnées pédagogiques.

## 8. Critères de validation

La V1 sera considérée comme fonctionnellement acceptable si les tests suivants réussissent :

1. Une note peut être créée, modifiée, renommée et supprimée.
2. Les notes restent disponibles après fermeture et redémarrage de l'application.
3. Les modifications sont sauvegardées automatiquement et l'état de sauvegarde est compréhensible.
4. Les notes peuvent être classées dans des dossiers et marquées comme favorites.
5. Une recherche retrouve les notes à partir de leur titre et de leur contenu.
6. Les caractères accentués, les listes et les blocs de code sont conservés.
7. Une note exportée en Markdown reste lisible hors de l'application.
8. Une sauvegarde valide peut être restaurée sans perte silencieuse de données.
9. Une erreur de sauvegarde est signalée à l'utilisateur.
10. Les fonctionnalités essentielles sont utilisables hors ligne.
11. L'interface reste utilisable sur les tailles de fenêtre prévues.
12. Le projet peut être construit et lancé à partir d'une procédure documentée.

## 9. Phases de développement

### Phase 1 — Fondations
- Initialisation de Wails, Go, Svelte et TypeScript.
- Mise en place du stockage.
- Définition du modèle de données.
- Première fenêtre et navigation.

### Phase 2 — Prise de notes
- Création, modification et suppression des notes.
- Éditeur Markdown.
- Sauvegarde automatique.
- Gestion des erreurs.

### Phase 3 — Organisation
- Dossiers.
- Favoris.
- Étiquettes.
- Notes récentes.
- Recherche.

### Phase 4 — Finition
- Thèmes clair et sombre.
- Raccourcis clavier.
- Exportation.
- Sauvegarde et restauration.
- Tests et correction des problèmes.

### Phase 5 — Validation
- Tests fonctionnels.
- Tests de persistance et de restauration.
- Essais auprès de quelques étudiants et développeurs.
- Collecte des retours et priorisation des corrections.

## 10. Évolutions envisagées après la V1

Ces fonctionnalités appartiennent à la vision future de NoteCore et ne doivent pas retarder la sortie de la première version.

**Révision**
- Transformer une note en questions.
- Créer des flashcards.
- Planifier les révisions.
- Suivre les notions maîtrisées et celles à revoir.

**Pratique informatique**
- Challenges d'algorithmique.
- Exercices de programmation.
- Éditeur de code dédié.
- Exécution et tests automatisés.
- Historique des tentatives et progression.

**Organisation intelligente**
- Suggestions de notes liées.
- Recherche plus avancée.
- Détection de doublons potentiels.
- Liens entre cours, exercices et projets.

Ces extensions devront répondre à des besoins observés et faire l'objet de spécifications séparées.

## 11. Livrables attendus

- Code source de l'application.
- Schéma et migrations de la base de données.
- Procédure d'installation et de développement.
- Instructions de compilation et de lancement.
- Tests des fonctions essentielles.
- Documentation des choix techniques.
- Version installable pour les plateformes retenues.
- Procédure d'exportation, de sauvegarde et de restauration.

## 12. Conditions de réussite

La réussite de NoteCore V1 ne dépend pas du nombre de fonctionnalités ajoutées, mais de sa capacité à devenir un outil quotidien fiable.

L'utilisateur doit pouvoir ouvrir l'application, écrire rapidement, organiser ses notes, les retrouver sans difficulté et être certain que ses données sont conservées.

La V1 doit fournir une base technique propre, sans anticiper inutilement toutes les fonctionnalités futures.

**Décision de périmètre :** la prise de notes et son organisation constituent le cœur de la V1. Les révisions et les challenges de programmation restent des axes d'évolution, à développer après validation des usages fondamentaux.