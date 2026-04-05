# Phase 1: Foundation - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Endpoint WebDAV read-only dans `web/webdav/` : routing (`/dav/files/` + redirect `/remote.php/webdav/*`), authentification (OAuth Bearer via header ou champ password Basic Auth), path safety (normalisation + anti-traversal), PROPFIND Depth 0/1 avec streaming XML, GET/HEAD avec streaming VFS, OPTIONS. Toutes les invariants correctness et sécurité (ETag, format de dates, namespace XML, Content-Length, path traversal) sont verrouillés avant d'aborder l'écriture en Phase 2.

Hors scope de cette phase : toutes les méthodes write (PUT, DELETE, MKCOL, MOVE, COPY, PROPPATCH), LOCK/UNLOCK (jamais en v1), COPY + docs + litmus (Phase 3).

</domain>

<decisions>
## Implementation Decisions

### Package et organisation
- Code dans `web/webdav/` — parallèle à `web/files/`
- Handlers custom Echo v4 (`echo.HandlerFunc`), pas `golang.org/x/net/webdav` ni `emersion/go-webdav`
- XML via `encoding/xml` stdlib
- Pas de nouveau package `model/` — toute la logique métier est déjà dans `model/vfs/`
- Claude's discretion : split des fichiers Go (un par méthode, ou groupés ? Nom des fichiers)

### Routing
- Route principale : `/dav/files/*` (MkcolAsGetMethod non — on ne gère pas MKCOL en phase 1)
- Route de compatibilité : `/remote.php/webdav/*` → redirect **308 Permanent Redirect** vers `/dav/files/*` (308 préserve la méthode, critique pour WebDAV car 301/302 risquent un fallback GET)
- Normalisation : URL decode → `path.Clean` → assertion du préfixe `/files/` après normalisation
- Rejet immédiat (avant tout appel VFS) de : `..`, `%2e%2e`, null bytes, prefixes `/settings`, `/apps`, etc.
- Scope exposé : uniquement l'arborescence `/files/` du VFS de l'instance

### GET sur collection
- Retourne **405 Method Not Allowed** avec header `Allow:` listant les méthodes supportées sur une collection (OPTIONS, PROPFIND, HEAD)
- Pas de page HTML de navigation, pas de redirect Cozy Drive
- Rationale : comportement RFC 4918 strict, simple, prévisible, aucun client WebDAV n'en a besoin

### Authentification
- OAuth Bearer dans `Authorization: Bearer <token>` — utilise `middlewares.GetRequestToken` existante
- OAuth token accepté aussi dans le champ password de Basic Auth (username ignoré) — convention Cozy existante, maximise la compatibilité client
- Réponse 401 : `WWW-Authenticate: Basic realm="Cozy"` (pas `Bearer realm=...` car les clients WebDAV connaissent mieux Basic)
- OPTIONS est **la seule méthode sans auth requise** — RFC 4918 permet la découverte du serveur
- Scope permission vérifié via l'infrastructure Cozy existante (pas de bypass)

### PROPFIND — propriétés et format
- 9 propriétés live supportées : `resourcetype`, `getlastmodified` (RFC 1123 / `http.TimeFormat`), `getcontentlength`, `getetag` (md5sum VFS double-quoted, **jamais** `_rev` CouchDB), `getcontenttype` (depuis VFS metadata, fallback `application/octet-stream`), `displayname` (nom de fichier URL-encoded), `creationdate` (ISO 8601), `supportedlock` (element vide), `lockdiscovery` (element vide)
- XML namespace : `xmlns:D="DAV:"` avec préfixe `D:` partout (jamais `xmlns="DAV:"` par défaut — Windows Mini-Redirector refuse)
- Depth:0 et Depth:1 uniquement ; Depth:infinity → **403 Forbidden**
- `supportedlock`/`lockdiscovery` retournés vides (pas de locking en v1) mais présents pour compat client

### Gros dossiers (streaming PROPFIND)
- Stream XML via `xml.Encoder.EncodeElement()` + `DirIterator` du VFS (pas de buffer complet)
- **Pas de cap dur** sur le nombre d'items retournés en Depth:1 — streaming illimité, mémoire bornée par batch
- Taille de batch DirIterator : **200 items** (override du défaut Cozy 100, validé par l'utilisateur pour équilibrer latence CouchDB et taille batch)
- Si le DirIterator existant n'expose pas une API cursor-friendly adéquate, il faudra l'étendre (nouvelle méthode VFS) — à confirmer en recherche phase

### Trash et dossiers spéciaux
- **Corbeille `.cozy_trash`** : visible en read-only via WebDAV. Elle apparaît dans le listing racine et son contenu est listable/téléchargeable, mais aucune opération write (réservée à Phase 2, et même là, elle reste en lecture seule — cohérent avec le comportement Cozy)
- **Sharings reçus** : visibles normalement via WebDAV, avec les mêmes permissions que dans Cozy Drive. Gérés par le système de permissions Cozy existant — pas de logique spécifique WebDAV
- Claude's discretion : comment distinguer read-only vs writable dans les réponses PROPFIND (ex: propriété customisée ou simple comportement 403 sur write en Phase 2)

### GET / HEAD
- GET fichier : délégué à `vfs.ServeFileContent` qui appelle `http.ServeContent` en interne → Range, ETag, Last-Modified, chunked transfer gérés automatiquement
- HEAD : mêmes headers que GET, body vide (géré nativement par `http.ServeContent`)
- GET sur collection : 405 (cf. décision plus haut)
- `Content-Length` obligatoire sur toutes les réponses (Finder strict) — géré par `ServeContent` pour les fichiers, calculé explicitement pour les réponses PROPFIND streamées (via `Transfer-Encoding: chunked` si impossible à calculer à l'avance)

### Format des erreurs
- Body XML conforme **RFC 4918 §8.7** : `<D:error xmlns:D="DAV:"><D:condition-element/></D:error>`
- Exemples :
  - 403 Depth:infinity : `<D:error><D:propfind-finite-depth/></D:error>`
  - 412 If-Match (phase 2) : `<D:error><D:lock-token-matches-request-uri/></D:error>` ou équivalent
- Content-Type : `application/xml; charset="utf-8"`
- Rationale : conforme RFC, les clients WebDAV sérieux le parsent, cohérent avec les réponses PROPFIND

### Audit logging
- Logger au niveau **WARN** dans le logger cozy-stack existant (pas de canal audit séparé en v1)
- Champs structurés standard : `instance`, `source_ip`, `user_agent`, `method`, `raw_url`, `normalized_path`, `token_hash` (jamais le token brut)
- Événements loggés :
  1. **Path traversal tentatives** — rejets par normalisation (`..`, `%2e%2e`, null bytes, préfixes système)
  2. **PROPFIND Depth:infinity** — tentatives de crawling/DoS
  3. **403 hors scope** — token valide qui tente d'accéder hors `/files/`
- Événements **NON loggés** : 401 non authentifié (trop bruyant — comportement normal de découverte du realm par les clients WebDAV)
- Pas de dashboard dédié, pas de métrique — reporté à v2

### Testing (TDD strict)
- Cycle **RED → GREEN → REFACTOR** avec commits séparés pour chaque étape (obligatoire, non négociable)
- Tests unitaires XML : marshalling/unmarshalling écrits **AVANT** les structs
- Tests unitaires path mapping : normalisation, traversal, edge cases (trailing slash, URL encoding, Unicode)
- Tests d'intégration : client `studio-b12/gowebdav` v0.12.0 — à ajouter au `go.mod` en début de phase
- Scaffolding tests d'intégration : réutiliser les helpers existants de `web/files/files_test.go` (test instance, setup/teardown, authenticated client)
- Litmus compliance suite : pas en Phase 1 (reporté à Phase 3)
- Jamais de mock du VFS dans les tests handlers — utiliser le VFS de test (afero/mem-backed)

### Claude's Discretion
- Split des fichiers Go dans `web/webdav/` (un fichier par méthode, ou regroupé)
- Nom exact des types Go pour les XML structs (respecter la convention cozy-stack existante dans `pkg/webdav/` s'il existe)
- Stratégie exacte pour détecter/exprimer le readonly sur trash dans PROPFIND
- Helper functions pour le path mapping (peut être inline ou extrait)
- Exact error XML content beyond RFC 4918 precondition elements

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Projet et requirements
- `.planning/PROJECT.md` — Vision, core value, contraintes, décisions clés
- `.planning/REQUIREMENTS.md` — 53 requirements v1 avec IDs, Phase 1 = 28 requirements (ROUTE-01..05, AUTH-01..05, READ-01..10, SEC-01..05, TEST-01/02/04)
- `.planning/ROADMAP.md` — Phase 1 success criteria

### Research (MANDATORY — lire avant toute planification)
- `.planning/research/SUMMARY.md` — Synthèse globale, roadmap implications
- `.planning/research/STACK.md` — Choix librairies (custom handlers + encoding/xml), pourquoi pas x/net/webdav ou emersion/go-webdav, gowebdav v0.12.0 pour tests
- `.planning/research/ARCHITECTURE.md` — Intégration Echo v4 / VFS, path mapping, auth flow, pagination stratégie, build order dépendances
- `.planning/research/FEATURES.md` — Méthodes WebDAV requises par client, properties PROPFIND, compat matrix
- `.planning/research/PITFALLS.md` — 20 pitfalls catégorisés : path traversal CVE-2023-39143, x/net/webdav bug #66059, md5sum vs _rev, Windows namespace bug, macOS Finder read-only, iOS ATS, Depth infinity DoS

### Codebase existant (MANDATORY pour comprendre l'intégration)
- `.planning/codebase/STRUCTURE.md` — Layout `web/`, convention `web/{domain}/` avec un fichier par concept
- `.planning/codebase/ARCHITECTURE.md` — Patterns multi-tenant, VFS interface-backed, middleware chain Echo
- `.planning/codebase/CONVENTIONS.md` — Style Go, error handling, naming
- `.planning/codebase/TESTING.md` — Patterns de tests existants, helpers disponibles, instance de test
- `.planning/codebase/CONCERNS.md` — Connus : race condition `vfs.MkdirAll` (pertinent Phase 2), autres dettes

### RFC et specs externes
- **RFC 4918** (WebDAV HTTP Extensions) — https://www.rfc-editor.org/rfc/rfc4918
  - §8.7 Error responses XML format
  - §9.1 PROPFIND (Depth header, multistatus response)
  - §14 XML element definitions (9 live properties)
  - §18 Class 1 compliance requirements
- **RFC 7232** (HTTP Conditional Requests) — If-Match / If-None-Match semantics (pertinent Phase 2 mais à avoir en tête)
- **RFC 1123** — Date format for `getlastmodified` (Go : `http.TimeFormat`)

### Code existant cozy-stack à réutiliser (identifié par architecture research)
- `web/middlewares/permissions.go:69-71` — `GetRequestToken()` extrait le token de Bearer header OU champ password Basic Auth
- `model/vfs/file.go:251` — `ServeFileContent` pour GET/HEAD streaming
- `model/vfs/couchdb_indexer.go` — `DirBatch` + `DirIterator` pour PROPFIND pagination
- `model/vfs/vfs.go` — `DirOrFileByPath` pour path mapping
- `web/files/` — pattern de référence pour la structure d'un domaine HTTP Cozy
- `pkg/webdav/webdav.go` — structs XML pré-existants (vérifier s'ils sont réutilisables ou à remplacer)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `middlewares.GetRequestToken()` : extrait déjà le token OAuth depuis Bearer header OU champ password Basic Auth. Réutilisable tel quel — seule la réponse 401 doit être WebDAV-spécifique (`WWW-Authenticate: Basic realm="Cozy"` au lieu de l'erreur JSON:API).
- `vfs.ServeFileContent` (`model/vfs/file.go:251`) : wrap de `http.ServeContent`. Gère Range, ETag, Last-Modified, chunked. Utilisable tel quel pour GET/HEAD.
- `vfs.DirOrFileByPath` : mapping chemin → document VFS. Utilisable pour résoudre les URLs WebDAV.
- `vfs.DirBatch` / `DirIterator` (`model/vfs/couchdb_indexer.go`) : pagination CouchDB. Fondation pour le streaming PROPFIND Depth:1.
- `web/files/files_test.go` : helpers de test (setup instance, auth, client). Réutilisables pour les tests d'intégration WebDAV.
- `pkg/webdav/webdav.go` : structs XML WebDAV pré-existants (à vérifier : couverture, conformité namespace D:, compatibilité avec nos besoins).

### Established Patterns
- **Domain packages** : chaque domaine HTTP a son package `web/{domain}/` avec `routes.go` + handlers (`web/files/`, `web/auth/`, etc.). Pattern à suivre pour `web/webdav/`.
- **Route registration** : chaque package expose une fonction `Routes(router *echo.Group)` appelée par `web/routing.go`. À reproduire.
- **Error handling** : convention Cozy = retourner des `error` typés, middleware traduit en HTTP. Pour WebDAV, on devra custom l'error-handler pour retourner du XML RFC 4918 §8.7 au lieu de JSON:API.
- **Middleware chain** : `middlewares.LoadAuth → middlewares.AllowWholeType` etc. Chain à assembler pour les routes WebDAV (sauf OPTIONS qui bypass auth).
- **Instance context** : `middlewares.GetInstance(c)` retourne l'instance Cozy. `inst.VFS()` retourne le VFS de l'instance. Pattern universel dans cozy-stack.

### Integration Points
- `web/routing.go` : ajouter un `webdav.Routes(...)` call dans la composition des routes
- `model/vfs/` : pas de modification a priori, mais possible extension si `DirIterator` ne suffit pas pour le streaming PROPFIND (à confirmer en recherche phase)
- `go.mod` : ajouter `github.com/studio-b12/gowebdav@v0.12.0` en `require` pour les tests d'intégration
- `pkg/webdav/` : vérifier existence et réutilisabilité du code pré-existant

</code_context>

<specifics>
## Specific Ideas

- "L'API WebDAV est une API secondaire, l'API fichiers Cozy reste la référence" — jamais bypasser la logique métier
- "Si une fonction existante pénalise fortement les perfs, l'améliorer ou en créer une nouvelle adaptée — jamais dupliquer la logique"
- Cible concrète v1 : OnlyOffice mobile + app Fichiers iOS native (confirmé par l'utilisateur, correction de la recherche qui disait iOS Files ne supportait pas WebDAV)
- Méthodologie non négociable : TDD strict avec commits RED/GREEN/REFACTOR séparés pour chaque cycle

</specifics>

<deferred>
## Deferred Ideas

- **App-specific passwords** : infrastructure à construire côté Cozy, mais reportée à v2 (v1 = OAuth Bearer uniquement)
- **Locking (LOCK/UNLOCK)** : hors scope v1 entièrement (stack Cozy ne le supporte pas)
- **PROPPATCH et dead properties** : v2
- **Quota properties** (`quota-available-bytes`, `quota-used-bytes`) : v2
- **Métriques WebDAV dédiées** et dashboard Grafana : v2
- **Rate limiting WebDAV-spécifique** : non nécessaire (global Cozy s'applique déjà)
- **Alerting automatique sur les audit logs** : v2
- **Digest Auth** : v2 si besoin
- **Cap dur / pagination sur PROPFIND** : pas en v1 (streaming illimité suffit), à évaluer si problèmes de DoS en prod

</deferred>

---

*Phase: 01-foundation*
*Context gathered: 2026-04-05*
