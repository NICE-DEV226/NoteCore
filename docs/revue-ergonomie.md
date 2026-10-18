# Revue d'ergonomie — NoteCore v0.1.x

Passe d'ergonomie menée sur la première livraison (v0.1.0), avant publication.
Chaque point listé ici a été corrigé et livré dans **v0.1.1**. L'objectif
n'est pas d'empiler des fonctionnalités, mais de supprimer les situations où
l'utilisateur perd du travail, hésite, ou ne sait pas ce que l'application
vient de faire.

Les corrections sont regroupées par gravité : d'abord ce qui pouvait faire
perdre des notes, ensuite ce qui bloquait une action, ensuite ce qui
trompait l'utilisateur.

---

## 1. Ce qui pouvait faire perdre du travail

### 1.1 Fermeture de la fenêtre pendant une frappe

**Constat.** La note en cours de modification reposait sur un enregistrement
automatique de 800 ms. Fermer la fenêtre pendant cet intervalle, ou changer
d'onglet, pouvait partir avant que l'écriture n'ait lieu : la dernière
saisie disparaissait sans avertissement.

**Correctif.** Garde-fous sur `beforeunload`, `pagehide` et
`visibilitychange` : tant que des modifications sont en attente, la
fermeture est retenue. Le sélecteur de note voisine après suppression
repart des notes réellement affichées, et non d'une liste périmée.

### 1.2 Restauration sans prévisualisation

**Constat.** Le flux de restauration ouvrait le sélecteur de fichier, puis
restaurait. Une archive corrompue, tronquée, ou produite par une version
plus récente n'était détectée qu'après coup — au mieux par un message
d'erreur, au pire en écrasant la base courante.

**Correctif.** La restauration est scindée en deux temps :

1. **Inspection** — l'archive est vérifiée sans rien écrire : manifeste
   présent, empreinte SHA-256 cohérente, schéma compatible.
2. **Application** — après confirmation explicite, la base courante est
   d'abord copiée à côté (`notecore.db.avant-restauration-<date>`), puis
   la sauvegarde est rétablie.

L'utilisateur voit ce qu'il va rétablir avant de s'y engager, et dispose
d'une issue de secours même après avoir confirmé.

### 1.3 Mise à niveau qui détruirait la base

**Constat.** Une version plus récente applique ses migrations de schéma sur
la base déjà présente chez l'utilisateur. En cas de migration défaillante,
rien ne permettait de revenir à l'état d'avant.

**Correctif.** L'ouverture de la base prend une copie
(`VACUUM INTO`, permissions `0600`, nom daté) **avant la moindre
écriture**, uniquement quand une migration est réellement requise et que la
base existait déjà. Le chemin de cette copie est annoncé dans l'application
au lancement : un fichier n'est jamais laissé sur le disque sans le dire.
Voir `storage.OpenWithReport` et `MigrationReport`.

---

## 2. Ce qui bloquait une action

### 2.1 Export et restauration qui échouaient sans motif

**Constat.** Les dialogs système Wails exigent le contexte de cycle de vie
de l'application. Le code retombait sur `context.Background()` quand ce
contexte n'était pas encore fourni : les exports et la restauration
échouaient silencieusement, sans message exploitable.

**Correctif.** L'absence de contexte lève une erreur franche
(`bindings.callContext`), et `main.go` branche explicitement
`OnStartup` sur l'initialisation des bindings. Retirer ce branchage fait
échouer le démarrage plutôt que de produire une application qui a l'air
marcher et n'exporte rien.

### 2.2 Barre de mise en forme active en aperçu

**Constat.** Les commandes de mise en forme restaient cliquables pendant
l'aperçu, où elles ne peuvent rien faire. L'utilisateur cliquait, voyait
rien se passer, et concluait à un dysfonctionnement.

**Correctif.** La barre est désactivée en aperçu et un rappel visible
l'indique. Les commandes qui restent utiles là (`Image`, `Joindre`) le
demeurent.

### 2.3 Dossiers : échec effacé

**Constat.** Un doublon ou un nom vide refermait le formulaire en faisant
disparaître ce qui avait été tapé, sans explication.

**Correctif.** Les opérations sur les dossiers renvoient leur succès, le
formulaire reste ouvert en cas d'échec, et l'erreur est affichée en ligne
avec une croix de fermeture.

---

## 3. Ce qui trompait l'utilisateur

### 3.1 Images liées à une URL

**Constat.** Le cahier des charges exige une application hors ligne. La
commande d'image proposait d'en insérer une depuis une URL : une note
ainsi constituée s'affichait chez l'auteur et se brisait partout ailleurs.

**Correctif.** L'import se fait depuis une image du poste, avec aperçu
intégré ; le type de fichier est déduit côté backend (lecture des octets,
repli sur l'extension). À l'export, les images sont copiées à côté du
`.md`, de sorte que le dossier exporté reste lisible sans NoteCore.

### 3.2 Recherche : résultats périmés

**Constat.** Modifier la requête laissait les anciens résultats affichés le
temps du nouvel appel. L'utilisateur pouvait lire comme pertinents des
résultats qui ne correspondaient plus à ce qu'il avait tapé.

**Correctif.** Les anciens résultats sont purgés dès que la requête change,
un état de chargement explicite remplace le vide, et les états vides sont
distincts selon la vue (aucune note / aucun résultat pour cette recherche),
avec l'action à suivre.

### 3.3 Pièces jointes invisibles

**Constat.** Une pièce jointe supprimée laissait ses références dans le
texte, sous forme de liens morts.

**Correctif.** Les pièces jointes sont listées dans l'éditeur. En supprimer
une retire aussi les références `attachment:<id>` correspondantes.

---

## 4. Ce qui a été affiné

- **Listes à puces** : bascule correcte sur les trois états (entrée nue,
  liste existante, sélection multi-lignes).
- **Indentation** : `Tab` / `Shift+Tab` fonctionnent sur plusieurs lignes.
- **Titre** : focalisé à la création d'une note, pour ne pas avoir à
  cliquer avant d'écrire.

---

## Méthode

Revue manuelle du parcours réel — création, écriture, recherche, export,
sauvegarde, restauration, mise à niveau — confrontée au cahier des charges.
Chaque constat est daté et rattaché à un correctif commité ; rien n'est
considéré réglé tant que `go test ./...` et `npm run check` ne sont pas au
vert sur la correction correspondante.
