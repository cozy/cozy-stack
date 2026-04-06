# Cozy WebDAV

## What This Is

An API WebDAV pour la stack Cozy qui expose l'espace fichiers de l'utilisateur (`/files/`) comme un filesystem réseau standard. Permet aux clients WebDAV tiers (OnlyOffice mobile, iOS Files, explorateurs de fichiers) d'accéder aux données personnelles Cozy via le protocole WebDAV (RFC 4918).

## Core Value

Un utilisateur peut connecter OnlyOffice mobile ou iOS Files à son Cozy et naviguer, lire, écrire, déplacer et supprimer ses fichiers comme avec n'importe quel stockage cloud WebDAV.

## Requirements

### Validated

- ✓ API fichiers existante (CRUD, arborescence, métadonnées) — existing
- ✓ Système de permissions et partages — existing
- ✓ VFS (Virtual File System) avec abstraction stockage — existing
- ✓ Authentification OAuth — existing
- ✓ Architecture multi-tenant (instances isolées) — existing
- ✓ Endpoint WebDAV principal sur `/dav/files` — validated Phase 1: foundation
- ✓ Route de compatibilité `/remote.php/webdav` → 308 redirect — validated Phase 1: foundation
- ✓ Authentification double : app-specific passwords (Basic) ET OAuth Bearer — validated Phase 1: foundation
- ✓ Exposition de l'arborescence `/files/` uniquement — validated Phase 1: foundation
- ✓ Réponses XML conformes RFC 4918 (multistatus, propriétés DAV) — validated Phase 1: foundation
- ✓ Délégation au VFS et aux fonctions existantes — validated Phase 1: foundation
- ✓ Méthodologie TDD stricte : RED→GREEN→REFACTOR par commits — validated Phase 1: foundation

- ✓ Support des méthodes d'écriture : PUT, DELETE, MKCOL, MOVE — validated Phase 2: write-operations
- ✓ Tests d'intégration E2E gowebdav pour toutes les méthodes write — validated Phase 2: write-operations

### Active

- [ ] COPY (fichier + dossier récursif) — Phase 3
- [ ] Compatibilité vérifiée avec OnlyOffice mobile (Phase 3)
- [ ] Compatibilité vérifiée avec iOS/iPadOS Files app (Phase 3)
- [ ] Suite litmus RFC 4918 Class 1 compliance — Phase 3
- [ ] Documentation des endpoints, exemples d'usage, compatibilités

### Out of Scope

- Locking (LOCK/UNLOCK) — la stack Cozy ne le supporte pas, la plupart des clients fonctionnent sans
- DeltaV (versioning WebDAV) — complexité disproportionnée, pas critique pour v1
- CalDAV / CardDAV — protocoles séparés, hors périmètre
- Extensions Microsoft propriétaires — sauf si nécessaire pour la compatibilité OnlyOffice
- Accès aux données app ou settings via WebDAV — uniquement `/files/`
- Application mobile ou frontend — c'est une API serveur uniquement

## Context

- **Codebase** : cozy-stack, monolithe Go multi-tenant avec Echo v4, CouchDB, VFS interface-backed
- **API existante** : L'API fichiers JSON:API dans `web/files/` est la référence. Le WebDAV est une API secondaire qui doit déléguer au VFS et aux fonctions existantes.
- **Principe clé** : ne pas dupliquer la logique métier. Si une fonction existante est trop lente pour le cas WebDAV, l'améliorer ou en créer une nouvelle adaptée, mais jamais contourner les mécanismes de sécurité.
- **Clients cibles v1** : OnlyOffice mobile, iOS/iPadOS Files app
- **Route principale** : `/dav/files` avec redirect de compatibilité depuis `/remote.php/webdav`

## Constraints

- **Stack** : Go, intégration dans le monolithe cozy-stack existant (pas de service séparé)
- **Standard** : RFC 4918 — sous-ensemble pragmatique (pas de locking, pas de DeltaV)
- **Sécurité** : Authentification obligatoire, pas de listing anonyme, protection contre le crawling PROPFIND (profondeur limitée, pagination)
- **Performance** : Pagination des listings volumineux, limites de requêtes, pas de full-tree PROPFIND
- **Méthodologie** : TDD strict — tests écrits avant le code, commits séparés RED/GREEN/REFACTOR
- **Architecture** : Délégation au VFS existant, pas d'accès direct CouchDB/Swift depuis le handler WebDAV

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Pas de locking v1 | Stack Cozy ne le supporte pas, clients fonctionnent sans | — Pending |
| `/dav/files` comme route principale | Propre, cohérent avec la stack, pas de faux `.php` | — Pending |
| Redirect `/remote.php/webdav` → `/dav/files` | Compatibilité clients qui hardcodent le chemin Nextcloud | — Pending |
| Auth double (Basic + OAuth) | Basic Auth via app-specific passwords pour les clients simples, OAuth pour les clients avancés | — Pending |
| Exposition `/files/` uniquement | Sécurité, simplicité, pas de fuite de données app | — Pending |
| API secondaire, délégation au VFS | Évite la duplication de logique métier, cohérence avec la stack | — Pending |

## Current State

Phase 2 (write-operations) complete — full read-write WebDAV surface: PUT (streaming, conditional If-Match/If-None-Match), DELETE (soft-trash via TrashFile/TrashDir), MKCOL (single-dir), MOVE (Overwrite semantics, Destination parsing, trash-then-rename). 5/5 plans, 15/15 Phase 2 requirements. 9 E2E subtests via gowebdav. Phase 1 regression clean. Next: Phase 3 (COPY, litmus compliance, documentation).

---
*Last updated: 2026-04-06 after Phase 2 completion*
