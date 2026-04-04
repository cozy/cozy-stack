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

### Active

- [ ] Endpoint WebDAV principal sur `/dav/files`
- [ ] Route de compatibilité `/remote.php/webdav` → redirect 301 vers `/dav/files`
- [ ] Support des méthodes WebDAV : PROPFIND, GET, PUT, DELETE, MKCOL, COPY, MOVE, OPTIONS, HEAD
- [ ] Authentification double : app-specific passwords (Basic Auth) ET OAuth Bearer tokens
- [ ] Exposition de l'arborescence `/files/` uniquement (pas les données app, settings, etc.)
- [ ] Réponses XML conformes RFC 4918 (multistatus, propriétés DAV)
- [ ] Compatibilité vérifiée avec OnlyOffice mobile
- [ ] Compatibilité vérifiée avec iOS/iPadOS Files app
- [ ] Délégation au VFS et aux fonctions existantes de la stack (pas d'accès direct aux données)
- [ ] Étude de faisabilité technique documentée (variantes WebDAV, bibliothèques Go, limites)
- [ ] Documentation des endpoints, exemples d'usage, compatibilités
- [ ] Tests exhaustifs : unitaires, intégration, comportement WebDAV via clients standards
- [ ] Méthodologie TDD stricte : tests avant le code, cycle RED→GREEN→REFACTOR

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

---
*Last updated: 2026-04-04 after initialization*
