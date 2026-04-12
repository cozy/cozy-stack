# Cozy WebDAV

## What This Is

Une implémentation WebDAV RFC 4918 Class 1 strict intégrée dans cozy-stack. Expose l'arborescence `/files/` de chaque instance Cozy comme un filesystem réseau standard accessible via deux routes : `/dav/files/` (native) et `/remote.php/webdav/` (compatibilité Nextcloud). Permet aux clients WebDAV tiers (rclone, iOS Files, OnlyOffice mobile en mode WebDAV, gestionnaires de fichiers Nautilus/Dolphin) de naviguer, lire, écrire, déplacer, copier et supprimer les fichiers personnels.

## Core Value

Un utilisateur peut monter son Cozy comme un lecteur réseau WebDAV depuis n'importe quel client compatible RFC 4918 Class 1 et manipuler ses fichiers avec les opérations POSIX usuelles.

## Requirements

### Validated

**Infrastructure préexistante**
- ✓ API fichiers existante (CRUD, arborescence, métadonnées) — existing
- ✓ Système de permissions et partages — existing
- ✓ VFS (Virtual File System) avec abstraction stockage — existing
- ✓ Authentification OAuth — existing
- ✓ Architecture multi-tenant (instances isolées) — existing

**v1.1 — WebDAV RFC 4918 Class 1 (shipped 2026-04-12)**
- ✓ Endpoint WebDAV principal sur `/dav/files/` — v1.1
- ✓ Route de compatibilité `/remote.php/webdav/` servie par les mêmes handlers — v1.1 (remplace le 308 redirect initialement prévu, car certains clients strippent l'Authorization header en redirect)
- ✓ Authentification double : Basic (token-as-password) et OAuth Bearer — v1.1
- ✓ Exposition de l'arborescence `/files/` uniquement — v1.1
- ✓ Réponses XML conformes RFC 4918 (multistatus, propriétés DAV, Multi-Status 207 body pour partial failures) — v1.1
- ✓ 10 méthodes WebDAV : OPTIONS, PROPFIND, PROPPATCH, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE — v1.1
- ✓ PUT streaming avec conditional If-Match / If-None-Match — v1.1
- ✓ DELETE soft-trash (via `.cozy_trash`) — v1.1
- ✓ MKCOL single-dir avec garde `Content-Length > 0` → 415 — v1.1
- ✓ MOVE avec Overwrite semantics (absent=T, F=412, T=trash-then-rename) — v1.1
- ✓ COPY fichier + dossier récursif via vfs.Walk (avec 207 Multi-Status pour partial failures) — v1.1
- ✓ PROPPATCH minimal avec dead-property store in-memory — v1.1 (persistence CouchDB = v2)
- ✓ Délégation intégrale au VFS existant (pas de duplication de logique métier) — v1.1
- ✓ Méthodologie TDD stricte : RED→GREEN par commits séparés — v1.1
- ✓ Tests d'intégration E2E gowebdav (6 success-criteria sub-tests) — v1.1
- ✓ Suite litmus RFC 4918 Class 1 : 63/63 pass sur les deux routes — v1.1
- ✓ Documentation endpoints complète (docs/webdav.md, 587 lignes, 27 exemples curl) — v1.1

### Active

_Aucun requirement actif. Lance `/gsd:new-milestone` pour définir v1.2._

### Deferred (tracked for v1.2+)

- Validation manuelle iOS/iPadOS Files app — best-effort v1.1 (couverte transitivement par litmus), formal sign-off reporté
- Validation manuelle OnlyOffice mobile — serveur validé expérimentalement contre v9.1.0 APK (pré-régression), bloqué en release par bug client upstream v9.2.0+ (traqué comme "App token login name does not match") — attend v9.3.2+
- Intégration CI de litmus — aujourd'hui manuelle uniquement via `make test-litmus`
- **FOLLOWUP-01** — race préexistante dans `pkg/config` / `model/stack` / `model/job` test harness (hors WebDAV, reproductible sur master). Recommandé comme première tâche de v1.2.

### Out of Scope

- Locking (LOCK/UNLOCK, RFC 4918 Class 2) — stack Cozy ne supporte pas, Finder macOS reste donc read-only
- DeltaV (RFC 3253 versioning WebDAV) — complexité disproportionnée
- CalDAV / CardDAV — protocoles séparés
- Extensions Microsoft propriétaires (MS-WebDAV-Extensions)
- API OCS Nextcloud (`/ocs/v1.php/*`) — nécessaire pour les clients "officiels" Nextcloud (Desktop Sync, Nextcloud Files mobile) et pour le mode Nextcloud d'OnlyOffice. Décision explicite de rester au pur WebDAV en v1.
- Persistance des dead properties via CouchDB — en mémoire uniquement en v1
- SEARCH (RFC 5323)
- Rate limiting WebDAV-spécifique (rate limiting global Cozy s'applique déjà)

## Context

- **Codebase** : `cozy-stack`, monolithe Go multi-tenant avec Echo v4, CouchDB, VFS interface-backed. Le package WebDAV vit dans `web/webdav/` (~2311 LOC production + ~2760 LOC tests).
- **API existante** : L'API fichiers JSON:API dans `web/files/` reste la référence fonctionnelle. Le WebDAV est une API secondaire qui délègue au VFS partagé.
- **Principe clé respecté v1.1** : zéro duplication de logique métier. Quand un besoin nouveau est apparu (ex : COPY d'un Cozy Note), on a branché sur la fonction existante (`note.CopyFile`) plutôt que de réimplémenter.
- **Tests** : trois niveaux empilés — unit tests in-process, E2E gowebdav (vrai client WebDAV Go), litmus (compliance tester externe). Plus une validation manuelle humaine par milestone.
- **Instances de test** : une instance live (`user-webdav.localhost:8080`) sert au dev manuel ; le script litmus crée/détruit des instances jetables.

## Constraints

- **Stack** : Go (1.25+), intégration dans le monolithe cozy-stack (pas de service séparé)
- **Standard** : RFC 4918 Class 1 — sous-ensemble pragmatique (pas de locking, pas de DeltaV)
- **Sécurité** : Authentification obligatoire sur tout sauf OPTIONS, pas de listing anonyme, path traversal rejeté avant le VFS, Depth:infinity bloqué pour éviter le crawling récursif
- **Performance** : Pagination des listings volumineux, PUT en streaming (pas de buffer complet mémoire), ETag consistency entre PROPFIND et PUT/GET
- **Méthodologie** : TDD strict — tests écrits avant le code, commits RED/GREEN séparés (enforcé dans VERIFICATION par comptage de commits)
- **Architecture** : Délégation au VFS, pas d'accès direct CouchDB/Swift depuis les handlers WebDAV

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Pas de locking v1 | Stack Cozy ne supporte pas, litmus auto-skip quand `DAV: 1` | ✓ Good — litmus 63/63 sans locking |
| `/dav/files/` comme route principale | Propre, cohérent avec la stack, pas de faux `.php` | ✓ Good |
| `/remote.php/webdav/` servi par les **mêmes handlers** (pas de 308 redirect) | Le 308 redirect initialement prévu a été retiré en commit `7c9ab3a59` — plusieurs clients strippent l'Authorization header lors du follow-redirect | ✓ Good — décision post-initiale correcte |
| Auth double (Basic + OAuth) | Basic Auth token-as-password pour clients simples (username ignoré), OAuth Bearer pour clients avancés | ✓ Good — tous les clients testés fonctionnent |
| Exposition `/files/` uniquement | Sécurité, simplicité, pas de fuite de données app/settings | ✓ Good |
| API secondaire, délégation au VFS | Zéro duplication de logique métier | ✓ Good — COPY délègue à fs.CopyFile / note.CopyFile |
| PROPPATCH in-memory (dead-property store) | Litmus props exige PROPPATCH qui "writes", mais CouchDB persistence = scope v2. Store in-memory avec `movePropsForPath` pour MOVE | ⚠ Revisit en v2 — propriétés perdues au restart |
| Pas d'API OCS Nextcloud | Scope explicite — OCS est un chantier à part (capabilities, user, shares). Accepter que les clients "full Nextcloud" ne fonctionnent pas | ✓ Good — documenté dans `docs/webdav.md` |
| Litmus CI déféré post-v1 | Paquet peu maintenu, bénéfice marginal vs tests Go qui couvrent les mêmes invariants | ✓ Good |
| Ship Phase 1 avec race pré-existante (FOLLOWUP-01) | Race dans `pkg/config` hors WebDAV, reproductible sur master, user-approved | — Pending — à traiter en v1.2 Task 0 |

## Current State

**Milestone v1.1 shipped 2026-04-12.** La stack cozy-stack dispose désormais d'un serveur WebDAV RFC 4918 Class 1 complet, monté sur deux routes, validé par litmus 63/63 et par une validation manuelle humaine via OnlyOffice Documents Android v9.1.0. Tous les 53 requirements v1 sont satisfaits avec accord triple-source (REQUIREMENTS ↔ VERIFICATION ↔ SUMMARY). La dette technique est limitée à FOLLOWUP-01 (race préexistante hors WebDAV, user-approved).

**Prochaines étapes envisageables pour v1.2** (à scoper via `/gsd:new-milestone`) :
- Traiter FOLLOWUP-01 en premier (race harness)
- Validation manuelle iOS Files app (formal sign-off)
- Validation manuelle OnlyOffice mobile quand v9.3.2+ sort
- Potentiellement : intégration CI de litmus, persistence dead-properties, API OCS partielle (capabilities-only pour améliorer la compat clients Nextcloud)

---
*Last updated: 2026-04-12 after v1.1 milestone completion*
