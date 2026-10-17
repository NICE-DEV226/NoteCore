# ADR-005 - Pilote SQLite en Go pur (modernc.org/sqlite)

## Statut

Accepté pour la phase 2.

## Contexte

Le backend doit ouvrir SQLite dans le répertoire utilisateur, exécuter
des transactions courtes et maintenir un index FTS5, sur plusieurs
plateformes de bureau. Deux familles de pilotes existent : les
bindings cgo (mattn/go-sqlite3) et les implémentations Go pur
(modernc.org/sqlite).

## Décision

Utiliser `modernc.org/sqlite` (Go pur), avec un pool d'une seule
connexion et des PRAGMA posés à l'ouverture (WAL, synchronous=NORMAL,
foreign_keys=ON, busy_timeout=5000).

## Raisons

- Le cgo coûte des microsecondes par appel et confisque des threads ;
  inutile pour des transactions courtes sérialisées.
- Le build croisé multiplateforme sans toolchain C est nettement plus
  simple — un livrable exigé par le CDC (§11).
- Le pool réduit à une connexion sérialise toutes les écritures : la
  règle « transactions courtes, pas de concurrence d'écriture » est
  garantie par construction, plus par vigilance.

## Conséquences

- Les performances brutes en écriture sont inférieures à cgo ; le
  périmètre (single-user, transactions courtes) les rend hors sujet —
  à revoir avec pprof si un scénario réel les remet en cause.
- La version embarquée du moteur SQLite est fixée par le module Go ;
  toute évolution du moteur passe par un `go get`, donc par un ADR.
- Le tokeniseur FTS5 s'appelle `unicode61 remove_diacritics 1` afin
  d'autoriser la recherche par accents retirés ; testé dans
  `service_test.go`.
