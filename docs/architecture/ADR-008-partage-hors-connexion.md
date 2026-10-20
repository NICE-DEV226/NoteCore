# ADR-008 - Partage de notes sans connexion, import, lecture des fichiers joints

## Statut

Accepté (V2.1). Complète ADR-006 (pièces jointes) et ADR-004 (sauvegardes). Le CDC V1 excluait la synchronisation et la collaboration (§2.2) ; ceci n'en est pas une : **aucun compte, aucun serveur, aucune copie qui reste synchronisée**. Un partage est un geste explicite, ponctuel, entre deux personnes.

## Contexte

Deux manques :

1. NoteCore exportait (Markdown, sauvegarde) mais **n'importait pas** : impossible de recevoir les notes d'un camarade, ni de reprendre celles d'un autre outil. Les étudiants échangent des cours en salle, sans Internet fiable.
2. Une pièce jointe se voyait comme une pastille : on ne pouvait ni la lire, ni parcourir les fichiers de toutes les notes ; un lien de fichier ouvrait seulement « Enregistrer sous ».

## Décision

### Partage

1. **Le paquet `.notecore`** : une archive zip (`manifest.json` en dernier, `notes/NNNNNN.md`, `attachments/<id>`). Il porte titre, étiquettes, épinglage, **nom** du dossier (les identifiants ne voyagent pas) et les pièces jointes. L'état d'apprentissage des cartes n'est pas transmis : il est personnel, les cartes se recalculent depuis le texte (ADR-007).
2. **Trois périmètres** : une note, un dossier, toutes les notes. **Deux formats** : paquet `.notecore` (réimportable à l'identique) ou archive Markdown `.zip` (lisible partout).
3. **Mot de passe facultatif** (paquet `.notecore` seulement) : AES-256-GCM, clé dérivée par PBKDF2-SHA256, 600 000 itérations, sel et nonce aléatoires ; l'en-tête est authentifié. Bibliothèque standard uniquement (`crypto/pbkdf2`, Go 1.24+), aucune dépendance. Le mot de passe n'est jamais stocké.
4. **Deux moyens de transmission, sans Internet** :
   - *Fichier* : enregistré par le dialogue système (écriture atomique) ; tout moyen de transmettre un fichier convient (clé USB, Bluetooth, dossier partagé…).
   - *Réseau local* : NoteCore ouvre un serveur HTTP éphémère sur le port choisi par le système, **un seul fichier, derrière un jeton aléatoire de 128 bits, pour un seul téléchargement, 10 minutes au plus** ; un lien et un QR code sont affichés. Un navigateur de téléphone peut le télécharger (page d'accueil minimale) ; NoteCore le reçoit par « Importer → Depuis un autre appareil ».
5. **Le serveur ne répond qu'au bon jeton** (404 muet sinon, comparaison à temps constant), refuse toute méthode autre que GET/HEAD, vide les données de la mémoire à la clôture et s'arrête seul (téléchargement terminé, délai écoulé, arrêt manuel). Un nouvel envoi ferme le précédent.
6. **La réception n'accepte que des adresses du réseau local** (privées ou locales de lien ; le bouclage est refusé), en `http`, sans proxy ni redirection, chemin `/s/…` seulement : la promesse « aucune connexion à Internet » est tenue par le code, pas par la bonne volonté. La vérification et la connexion partagent **la même résolution** (connexion ouverte sur l'adresse validée) : un serveur DNS qui répondrait « local » au contrôle puis « public » à la connexion (DNS rebinding) est sans effet.

### Import

7. **Entrées reconnues** : paquet `.notecore`, archive d'export NoteCore (`notecore-export`, dossiers conservés), archive de fichiers Markdown d'un autre outil (sous-dossiers, `__MACOSX` et fichiers cachés ignorés), fichier Markdown seul. L'en-tête YAML (`title`, `tags` en liste en ligne, avec guillemets ou à tirets) est lu.
8. **Importer n'écrase, ne supprime et ne modifie jamais** une note existante. Une note de même titre **et** de même contenu est ignorée (les identifiants de pièces jointes sont neutralisés pour la comparaison, un réimport est donc reconnu) ; une note de même titre mais de contenu différent est ajoutée, son titre complété de « (importée) ». Les dossiers sont retrouvés par nom ou créés.
9. **Une note à moitié importée n'existe pas** : en cas d'erreur sur une note, elle est supprimée ; un paquet invalide n'écrit rien.
10. **Bornes anti-archive hostile**, avant toute écriture : 256 Mio par paquet, 20 000 entrées, 512 Mio décompressés, 20 Mio par note, 10 Mio par pièce jointe, version de paquet inconnue refusée.
11. Un paquet protégé n'est pas une erreur : l'interface demande le mot de passe et reprend l'import **sans retélécharger** (un lien d'envoi ne sert qu'une fois).

### Lire les fichiers joints

12. **Visionneuse intégrée** : images (zoom), PDF (**pdf.js**, version « legacy », Apache-2.0, chargé à la demande), Markdown, texte et code, tableaux CSV/TSV, contenu d'une archive `.zip` (liste sans extraction), audio, vidéo. Rien n'est exécuté : le texte passe par `textContent`, le Markdown par le moteur échappé de NoteCore, le PDF par pdf.js dans son worker.
13. **Vue « Fichiers »** : toutes les pièces jointes de toutes les notes, filtre par type et par nom, vignettes d'images chargées à l'approche de l'écran, retour à la note d'origine.
14. **Ouvrir avec le système** : copie temporaire (0600, dossier propre à l'ouverture, marquée « zone Internet » sous Windows pour que SmartScreen et le mode protégé d'Office s'appliquent), puis ouverture par l'application par défaut ; les copies sont supprimées à la fermeture et, au démarrage, si elles datent de plus de 24 h. **Liste blanche** de types passifs (documents PDF/texte/ODF/OOXML sans macros, images, son, vidéo, archives) ; tout le reste est refusé — une liste noire oublie toujours une extension (`.pyw`, `.jnlp`, `.appref-ms`…). L'extension double (`facture.pdf.exe`) est jugée sur sa fin ; les formats Office à macros et le SVG (scripts dans un navigateur) sont exclus. Les autres types se lisent dans NoteCore quand c'est possible, ou s'enregistrent avec « Enregistrer sous… ».

## Options considérées

- **Synchronisation entre appareils** : exclue (CDC §2.2) ; un état partagé à maintenir est exactement ce que NoteCore évite.
- **Découverte automatique des voisins (mDNS)** : agréable, mais ajoute une dépendance et des règles de pare-feu ; lien + QR code suffisent, fonctionnent avec un téléphone sans NoteCore, et sont plus simples à sécuriser.
- **Serveur permanent** : rejeté ; un port ouvert en continu serait une surface d'attaque pour un usage ponctuel.
- **Chiffrement par défaut** : rejeté ; le mot de passe est un souci de plus pour l'utilisateur dans le cas courant (réseau de confiance). Il est proposé, et conseillé sur un Wi-Fi partagé.
- **Visionneuse PDF du système (WebView)** : WebKitGTK n'en fournit pas ; pdf.js donne le même rendu partout, au prix d'environ 1,8 Mo.

## Conséquences

- Nouvelle dépendance frontend : `pdfjs-dist` (Apache-2.0), `qrcode-generator` (MIT). Aucune dépendance Go.
- Sous Windows, la première ouverture d'un envoi sur le réseau local déclenche l'invite du pare-feu (réseaux privés) : c'est attendu.
- Le réseau local n'étant pas chiffré, un tiers présent sur le même Wi-Fi qui connaîtrait le lien avant l'autre appareil pourrait télécharger le paquet. Le jeton aléatoire, l'usage unique, le délai court et le mot de passe facultatif limitent ce risque ; il reste à connaître.
- Les dates de création et de modification d'une note importée sont celles de l'import.
- Limite connue : les images relatives d'un fichier Markdown importé depuis un autre outil ne sont pas rapatriées (seul le texte l'est).
