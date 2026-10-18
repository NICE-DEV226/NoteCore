# Cahier des charges — NoteCore V2 : réviser ses notes

**Version :** 2.0 (spécification) · **Statut :** première tranche implémentée (§4)
**Complète** le cahier des charges V1 : tout ce qui n'est pas modifié ici reste valable.

## 1. Pourquoi une V2

La V1 fait ce que font déjà de nombreux outils : écrire, classer, retrouver. Ses critères de réussite (V1 §12) portent sur la fiabilité, pas sur ce qui distingue NoteCore.

Le point de départ de la V2 est celui-ci : **écrire et relire ses notes est l'une des façons d'apprendre les moins efficaces.** La revue de Dunlosky et al. (2013) range la relecture, le surlignage et le résumé parmi les techniques peu utiles, et le test de rappel et la pratique espacée en tête. Une application de notes qui ne fait que stocker encourage les premières.

**Idée directrice : NoteCore est le carnet qui fait réviser ce qu'on y écrit.** Public inchangé : étudiants et développeurs.

Ce qui est établi et ce qui ne l'est pas : l'efficacité du rappel espacé est solidement documentée ; en revanche, je n'ai trouvé aucune enquête chiffrée sur l'abandon des outils concurrents. Les frictions décrites ci-dessous viennent de discussions d'utilisateurs, pas de mesures. La validation auprès d'étudiants (§7) reste donc nécessaire.

## 2. Périmètre

### 2.1 Inclus dans la V2

- **Cartes écrites dans les notes** (trou, question/réponse) — ADR-007.
- **Séance de révision** au clavier, planifiée par FSRS.
- **Tableau de bord** : cartes à voir, série de jours, activité sur 14 jours, maîtrise par dossier et par étiquette.
- **Personnalisation du texte et de l'interface** : 23 polices embarquées, taille, interligne, graisse, largeur, accent, densité, préréglages.
- **Refonte de l'interface** : accueil, palette de commandes, modes Éditer / Scinder / Aperçu, concentration.

### 2.2 Exclus (inchangé)

IA générative, comptes et synchronisation, collaboration, application mobile, marketplace.

### 2.3 Reporté après validation (V2.x)

| Piste | Condition pour la lancer |
|---|---|
| Bibliothèque de code : extraction des blocs de code de toutes les notes, filtre `lang:` | La révision est adoptée |
| Liens entre notes `[[note]]` et rétroliens | Demande observée |
| Exécution Python/JavaScript dans une note (WebAssembly, hors ligne) | À valider d'abord : poids du runtime embarqué, comportement de WebKitGTK et de WebView2 |
| Exercices à tests cachés, branchés sur la révision | L'exécution fonctionne |
| Corbeille (suppression annulable) | Chantier séparé : une suppression emporte les pièces jointes et les cartes |

## 3. Spécifications fonctionnelles

### 3.1 Écrire une carte

| Syntaxe | Effet |
|---|---|
| `{{c1::mot caché}}` | Cache le passage. Les trous de même numéro sont révisés ensemble. |
| `{{c1::mot::indice}}` | Idem, l'indice s'affiche à la place du blanc. |
| `Question :: Réponse` | Une ligne = une carte (une puce ou une citation en début de ligne est ignorée). |

- Les blocs de code et le code en ligne ne produisent jamais de carte.
- Une syntaxe incomplète (`{{c1::sans fin`) reste du texte : l'utilisateur ne voit jamais d'erreur pendant qu'il tape.
- L'éditeur propose deux boutons : cacher la sélection (numéro libre suivant), insérer une question.
- Dans l'aperçu d'une note, la réponse d'un trou reste lisible, mise en évidence.

### 3.2 Réviser

- Ouverture : bouton « Réviser » (avec nombre de cartes), accueil, palette.
- Ordre : apprentissage échu, révisions du jour, nouvelles cartes (limite quotidienne), apprentissage proche (20 min).
- Clavier : `Espace` retourne, `1` à `4` répondent (À revoir, Difficile, Bien, Facile), `Z` annule la dernière réponse, `Échap` quitte.
- Chaque bouton affiche le délai qui en résulte (« 10 min », « 8 j »).
- Fin de séance : bilan et prochaine échéance.

### 3.3 Suivi

- Accueil : cartes à voir (à réviser / nouvelles), série, activité sur 14 jours.
- Maîtrise par dossier et par étiquette : moyenne de la probabilité de rappel des cartes déjà apprises.
- Dans l'éditeur : nombre de cartes de la note et de cartes à réviser.

### 3.4 Réglages

- Nouvelles cartes par jour (0 à 100 dans l'interface, 500 au plus côté backend), défaut 20.
- Rétention visée (70 à 99 %), défaut 90 %.

### 3.5 Personnalisation

- Polices distinctes pour l'interface, le texte des notes, les titres et le code (catalogue : `frontend/src/lib/fonts.ts`) ; polices du système en repli.
- Taille, interligne, graisse, espacement des lettres, espace entre paragraphes, largeur de colonne, taille du titre, alignement.
- Couleur d'accent (teinte libre), densité des listes, forme des angles, correcteur orthographique.
- Six préréglages : Papier, Livre, Développeur, Lecture facilitée, Cahier, Dense.
- Tous ces réglages sont **locaux et sans effet sur le contenu des notes** (V1 §3.4, §6.5). Ils sont enregistrés dans un seul objet JSON de la table `settings`.

## 4. État d'avancement

| Élément | État |
|---|---|
| Schéma 3 (`cards`, `review_log`), restauration, migration | Fait, testé |
| Analyse des cartes, synchronisation transactionnelle, orphelines | Fait, testé |
| Planification FSRS, file d'attente, limite quotidienne, annulation | Fait, testé |
| Maîtrise par dossier / étiquette, statistiques | Fait, testé |
| Séance de révision, accueil, éditeur, réglages | Fait, vérifié sur le mock navigateur |
| Vérification dans l'application Wails réelle | **À faire** |
| Essais auprès d'étudiants | **À faire** (§7) |

## 5. Critères de validation

1. Écrire `{{c1::x}}` dans une note crée une carte ; supprimer la syntaxe la met de côté ; la réécrire la réactive avec sa progression.
2. Modifier le texte d'un trou ne remet pas sa progression à zéro.
3. Une réponse déplace l'échéance de la carte ; annuler la dernière réponse rétablit l'état exact d'avant.
4. La limite de nouvelles cartes par jour est respectée.
5. Supprimer une note supprime ses cartes ; supprimer un dossier ne supprime rien.
6. Une sauvegarde restaurée conserve cartes et historique ; une sauvegarde d'avant la V2 reste restaurable et les cartes sont reconstruites depuis les notes.
7. La mise à niveau depuis la V1 conserve une copie de la base avant la migration 3 (V1 critère 12).
8. Aucune fonction de la V2 n'émet de requête réseau.
9. Un export Markdown reste lisible hors de NoteCore.

## 6. Risques

| Risque | Parade |
|---|---|
| La syntaxe des cartes paraît technique | Boutons dans l'éditeur ; exemples dans l'accueil et les réglages |
| Une question retapée perd son historique | Cartes mises de côté 30 jours, réactivées si le texte revient |
| Un paragraphe long donne un recto illisible | Une carte = une ligne ; limite connue, à observer en essais |
| Dépendance FSRS | Bibliothèque officielle, MIT, épinglée en `v4.0.0` |
| Poids des polices (≈ 1,1 Mo embarqués) | Chargées à l'usage seulement ; sous-ensemble latin |

## 7. Validation auprès des utilisateurs (avant de poursuivre)

Mettre la version entre les mains de cinq étudiants pendant deux semaines. Mesurer : combien écrivent au moins dix cartes, combien reviennent après sept jours, quelle étape de l'écriture d'une carte les bloque. Si peu de cartes sont écrites, le problème est l'écriture, pas l'ordonnancement : on corrige avant d'ajouter des fonctions.
