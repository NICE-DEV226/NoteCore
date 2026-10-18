# ADR-007 - Révision espacée : des cartes écrites dans les notes

## Statut

Accepté pour la V2. Le cahier des charges V1 (§2.2, §10) excluait les flashcards et la répétition espacée ; la V2 les réintroduit, sous la forme réduite décrite ici (voir `docs/cahier-des-charges-v2.md`).

## Contexte

Une application de notes encourage la relecture, le surlignage et le résumé. La synthèse de Dunlosky et al. (2013, *Psychological Science in the Public Interest*) classe ces techniques parmi les moins utiles, et place le test de rappel (*practice testing*) et la pratique espacée (*distributed practice*) en tête. NoteCore doit donc aider à **se tester sur ce qu'on a écrit**.

Les outils existants séparent la note et la carte (Anki, plugins Obsidian) : l'utilisateur recopie, puis doit garder les deux synchronisées — quelle version gagne, que faire d'une suppression. Le cahier des charges interdit déjà ce défaut (§5.3 : « ne pas maintenir deux copies indépendantes d'une même note »).

## Décision

1. **Les cartes vivent dans le Markdown de la note.** Deux syntaxes, rien d'autre :
   - trou : `{{c1::réponse}}` ou `{{c1::réponse::indice}}` — le numéro regroupe les trous révisés ensemble ;
   - question : une ligne `Question :: Réponse`.
   Les blocs de code et le code en ligne sont ignorés.
2. **La base ne conserve que l'état d'apprentissage** (tables `cards` et `review_log`, migration 3). Le recto et le verso sont **dérivés** du texte à chaque enregistrement de la note, dans la même transaction (règle d'or d'ADR-003). Il n'existe pas de seconde copie du texte à synchroniser.
3. **Identité d'une carte** : `c<N>` pour un trou — le numéro est écrit par l'utilisateur, modifier le texte ne perd donc pas la progression ; `q:<empreinte>` pour une question (empreinte du texte normalisé : casse et espaces ignorés).
4. **Une carte dont le texte disparaît n'est pas détruite** : elle est mise de côté (`orphaned_at`) et purgée après 30 jours. Retoucher une question en la tapant, ou annuler une frappe, ne détruit donc pas l'historique ; une carte qui réapparaît est réactivée telle qu'elle était.
5. **Planification : FSRS (v6)**, par la bibliothèque `github.com/open-spaced-repetition/go-fsrs/v4` (MIT, compatible avec la licence du projet). Poids par défaut, rétention visée réglable (70 à 99 %, défaut 90 %), dispersion des intervalles activée. Aucun calcul n'est fait hors du backend Go.
6. **Réglages** : nouvelles cartes par jour (défaut 20) et rétention visée, dans la table `settings`, bornés par le backend.
7. **Annulation** : chaque réponse conserve dans `review_log.before` l'état exact de la carte ; annuler la dernière réponse le rétablit sans approximation.
8. **Sauvegarde et restauration** : `cards` et `review_log` entrent dans le schéma 3. Une sauvegarde de schéma 1 ou 2 reste restaurable (tables absentes ignorées) ; après restauration, les cartes sont reconstruites depuis les notes (`Maintain`).
9. **Export Markdown inchangé** : la syntaxe des cartes reste dans le texte exporté (lisible, et reconnue par plusieurs extensions de révision de la communauté).

## Options considérées

- **Cartes stockées à part, avec un lien vers la note** : comportement d'Anki. Rejeté : deux copies, et le problème de cohérence que le CDC interdit.
- **Cartes dans des fichiers `.md` séparés** : portable, mais multiplie les fichiers et les cas de renommage/suppression (§5.3).
- **SM-2** : simple et connu, mais un seul facteur de facilité par carte ; FSRS modélise stabilité, difficulté et probabilité de rappel, et fournit directement la probabilité de rappel utilisée pour la maîtrise par dossier.
- **Réimplémenter FSRS** : rejeté, 21 paramètres à reproduire sans écart ; la bibliothèque officielle est testée et maintenue.
- **Génération des cartes par IA** : exclue (CDC V1 §2.2 ; contraire à la promesse « local et privé »).

## Conséquences

- Nouvelle dépendance Go (MIT). Elle est compilée dans le binaire : aucune requête réseau.
- `UpdateNote` appelle `review.SyncNote` quand le contenu change ; le coût est une analyse linéaire du texte.
- Une note est bornée à 500 cartes (borne anti-abus, pas une limite d'usage).
- Au premier lancement après mise à niveau, `Maintain` indexe les notes existantes qui contiennent `::` ; l'opération est idempotente et signalée par `review.indexed`.
- La maîtrise par dossier ou étiquette est la moyenne de la probabilité de rappel FSRS des cartes déjà apprises ; ce n'est pas une note scolaire.
- Limite connue : les trous d'un même numéro sur plusieurs lignes sont réunis en une carte ; un paragraphe très long donne un recto long.

## À ne pas faire (sans nouvelle décision)

- Exécuter du code de l'utilisateur côté backend Go (surface d'attaque).
- Envoyer des cartes ou l'historique vers un service distant.
