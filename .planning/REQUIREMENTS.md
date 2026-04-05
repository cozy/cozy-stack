# Requirements: Cozy WebDAV

**Defined:** 2026-04-05
**Core Value:** Un utilisateur peut connecter OnlyOffice mobile ou l'app Fichiers iOS à son Cozy et naviguer, lire, écrire, déplacer et supprimer ses fichiers comme avec n'importe quel stockage cloud WebDAV.

## v1 Requirements

Requirements pour la release initiale. Chaque requirement est mappé à une phase du roadmap.

### Routing & Discovery

- [ ] **ROUTE-01**: WebDAV endpoint principal exposé sur `/dav/files/`
- [ ] **ROUTE-02**: Route de compatibilité `/remote.php/webdav/*` → redirect 308 vers `/dav/files/*`
- [x] **ROUTE-03**: Normalisation des chemins (trailing slash, URL decoding, `path.Clean`, assertion du préfixe contre path traversal)
- [ ] **ROUTE-04**: OPTIONS répond avec `DAV: 1`, `Allow: <liste des méthodes>`, pas d'authentification requise
- [x] **ROUTE-05**: Exposition de l'arborescence `/files/` uniquement (jamais `/settings`, `/apps`, `/shared`, etc.)

### Authentication

- [ ] **AUTH-01**: OAuth Bearer token dans header `Authorization: Bearer <token>` — utilise l'infrastructure existante `middlewares.GetRequestToken`
- [ ] **AUTH-02**: OAuth token accepté aussi dans le champ password de Basic Auth (username ignoré) — convention Cozy existante, maximise la compatibilité client
- [ ] **AUTH-03**: Réponse 401 `WWW-Authenticate: Basic realm="Cozy"` sur requête non authentifiée (hors OPTIONS)
- [ ] **AUTH-04**: Traduction du token en permissions Cozy existantes (pas de bypass des hooks de sécurité)
- [ ] **AUTH-05**: Scope permission vérifié — l'utilisateur doit avoir les droits sur l'arborescence `/files/`

### Read Operations

- [ ] **READ-01**: PROPFIND Depth: 0 sur collection (dossier) — retourne les propriétés du dossier
- [ ] **READ-02**: PROPFIND Depth: 0 sur resource (fichier) — retourne les propriétés du fichier
- [ ] **READ-03**: PROPFIND Depth: 1 sur collection — retourne le dossier + ses enfants directs
- [ ] **READ-04**: PROPFIND Depth: infinity bloqué avec 403 Forbidden (prévention DoS)
- [x] **READ-05**: PROPFIND retourne les 9 propriétés live standards : `resourcetype`, `getlastmodified` (RFC 1123), `getcontentlength`, `getetag` (md5sum VFS, double-quoted), `getcontenttype`, `displayname`, `creationdate` (ISO 8601), `supportedlock` (vide), `lockdiscovery` (vide)
- [x] **READ-06**: PROPFIND XML utilise le namespace `D:` préfixé (`xmlns:D="DAV:"`) — compatibilité Windows Mini-Redirector
- [ ] **READ-07**: PROPFIND streaming XML pour les gros dossiers (pas de buffer complet en mémoire, utilisation de `DirIterator`)
- [ ] **READ-08**: GET sur fichier — streaming via `vfs.ServeFileContent` (support Range, ETag, chunked)
- [ ] **READ-09**: HEAD sur fichier — mêmes headers que GET sans body
- [ ] **READ-10**: GET sur collection retourne 405 Method Not Allowed (ou page HTML de navigation, à décider)

### Write Operations

- [ ] **WRITE-01**: PUT — upload streaming (pas de buffer complet), utilise `vfs.CreateFile`/`io.Copy`
- [ ] **WRITE-02**: PUT crée le fichier ou overwrite si existe déjà
- [ ] **WRITE-03**: PUT support `If-Match` et `If-None-Match` (conditional requests basés sur ETag)
- [ ] **WRITE-04**: PUT sur chemin dont le parent n'existe pas retourne 409 Conflict
- [ ] **WRITE-05**: DELETE sur fichier — suppression via `vfs.DestroyFile`
- [ ] **WRITE-06**: DELETE sur collection — suppression récursive via `vfs.DestroyDirAndContent`
- [ ] **WRITE-07**: MKCOL — création de dossier via `vfs.Mkdir` (un seul niveau, pas `MkdirAll` à cause de la race condition existante)
- [ ] **WRITE-08**: MKCOL sur chemin dont le parent n'existe pas retourne 409 Conflict
- [ ] **WRITE-09**: MKCOL sur chemin existant retourne 405 Method Not Allowed

### Move & Copy

- [ ] **MOVE-01**: MOVE fichier — rename/reparent via `vfs.ModifyFileMetadata` avec `DocPatch` (nom + dirID)
- [ ] **MOVE-02**: MOVE dossier — via `vfs.ModifyDirMetadata`
- [ ] **MOVE-03**: MOVE header `Overwrite` absent est traité comme `T` par défaut (conforme RFC 4918, contourne le bug `x/net/webdav` #66059)
- [ ] **MOVE-04**: MOVE `Overwrite: F` avec destination existante retourne 412 Precondition Failed
- [ ] **MOVE-05**: MOVE `Destination` header URL-decoded et validé contre path traversal
- [ ] **COPY-01**: COPY fichier — via `vfs.CopyFile`
- [ ] **COPY-02**: COPY dossier — walk récursif + `CopyFile` par fichier (acceptable pour v1)
- [ ] **COPY-03**: COPY respecte les mêmes sémantiques `Overwrite` que MOVE

### Security

- [ ] **SEC-01**: Toutes les méthodes sauf OPTIONS nécessitent une authentification valide
- [x] **SEC-02**: Path traversal prevention — `path.Clean` + assertion du préfixe `/files/` après normalisation
- [ ] **SEC-03**: Limite de profondeur/taille sur PROPFIND (PROPFIND Depth infinity bloqué, pagination Depth 1 pour très gros dossiers)
- [ ] **SEC-04**: Logs d'audit pour les tentatives d'accès hors `/files/` et les PROPFIND Depth infinity
- [x] **SEC-05**: Content-Length obligatoire sur toutes les réponses (Finder strict)

### Documentation

- [ ] **DOC-01**: Documentation des endpoints WebDAV dans `docs/` (méthodes supportées, auth, exemples)
- [ ] **DOC-02**: Exemples de configuration pour OnlyOffice mobile et iOS Files
- [ ] **DOC-03**: Notes de compatibilité (Finder read-only, pas de locking, limites PROPFIND)
- [ ] **DOC-04**: Spécification OpenAPI ou équivalent si le repo en a pour les autres APIs

### Testing (TDD strict)

- [x] **TEST-01**: Tests unitaires XML (marshalling/unmarshalling) — écrits AVANT les structs
- [x] **TEST-02**: Tests unitaires path mapping (normalisation, traversal, edge cases)
- [ ] **TEST-03**: Tests d'intégration par méthode WebDAV utilisant `studio-b12/gowebdav` comme client
- [x] **TEST-04**: Tests d'intégration auth (Basic avec token, Bearer, 401, scopes)
- [ ] **TEST-05**: Tests de comportement : scénarios OnlyOffice mobile et iOS Files (open → read → write → save)
- [ ] **TEST-06**: Suite litmus WebDAV compliance exécutée contre l'implémentation (RFC 4918 Class 1)
- [ ] **TEST-07**: Tous les commits suivent le cycle RED→GREEN→REFACTOR séparément

## v2 Requirements

Reportés pour une future release.

### Auth Extensions

- **AUTH-V2-01**: App-specific passwords (mécanisme complet côté Cozy, puis utilisation en WebDAV)
- **AUTH-V2-02**: Digest Auth pour compatibilité élargie

### Advanced WebDAV

- **ADV-V2-01**: LOCK/UNLOCK (Class 2) — si la stack Cozy évolue pour le supporter
- **ADV-V2-02**: PROPPATCH pour propriétés custom (dead properties)
- **ADV-V2-03**: Quota properties (`quota-available-bytes`, `quota-used-bytes`)
- **ADV-V2-04**: Swift server-side COPY (optimisation si backend Swift le permet)

### Observability

- **OBS-V2-01**: Métriques WebDAV (par méthode, latence, erreurs)
- **OBS-V2-02**: Dashboard Grafana dédié

## Out of Scope

| Feature | Reason |
|---------|--------|
| LOCK/UNLOCK v1 | La stack Cozy ne supporte pas le locking, OnlyOffice fonctionne sans |
| DeltaV (RFC 3253) | Complexité disproportionnée, pas critique |
| CalDAV / CardDAV | Protocoles séparés, hors périmètre |
| Extensions Microsoft propriétaires | Pas nécessaires pour les cibles OnlyOffice + iOS Files |
| Accès aux données app / settings | Sécurité — uniquement `/files/` exposé |
| SEARCH (RFC 5323) | Non demandé par les cibles v1 |
| App mobile / frontend | API serveur uniquement |
| Proxy/CDN pour WebDAV | Pas nécessaire au lancement |
| Rate limiting WebDAV-spécifique | Le rate limiting global Cozy s'applique déjà |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| ROUTE-01 | Phase 1 | Pending |
| ROUTE-02 | Phase 1 | Pending |
| ROUTE-03 | Phase 1 | Complete |
| ROUTE-04 | Phase 1 | Pending |
| ROUTE-05 | Phase 1 | Complete |
| AUTH-01 | Phase 1 | Pending |
| AUTH-02 | Phase 1 | Pending |
| AUTH-03 | Phase 1 | Pending |
| AUTH-04 | Phase 1 | Pending |
| AUTH-05 | Phase 1 | Pending |
| READ-01 | Phase 1 | Pending |
| READ-02 | Phase 1 | Pending |
| READ-03 | Phase 1 | Pending |
| READ-04 | Phase 1 | Pending |
| READ-05 | Phase 1 | Complete (01-02) |
| READ-06 | Phase 1 | Complete (01-02) |
| READ-07 | Phase 1 | Pending |
| READ-08 | Phase 1 | Pending |
| READ-09 | Phase 1 | Pending |
| READ-10 | Phase 1 | Pending |
| SEC-01 | Phase 1 | Pending |
| SEC-02 | Phase 1 | Complete |
| SEC-03 | Phase 1 | Pending |
| SEC-04 | Phase 1 | Pending |
| SEC-05 | Phase 1 | Complete |
| TEST-01 | Phase 1 | Complete |
| TEST-02 | Phase 1 | Complete |
| TEST-04 | Phase 1 | Complete |
| WRITE-01 | Phase 2 | Pending |
| WRITE-02 | Phase 2 | Pending |
| WRITE-03 | Phase 2 | Pending |
| WRITE-04 | Phase 2 | Pending |
| WRITE-05 | Phase 2 | Pending |
| WRITE-06 | Phase 2 | Pending |
| WRITE-07 | Phase 2 | Pending |
| WRITE-08 | Phase 2 | Pending |
| WRITE-09 | Phase 2 | Pending |
| MOVE-01 | Phase 2 | Pending |
| MOVE-02 | Phase 2 | Pending |
| MOVE-03 | Phase 2 | Pending |
| MOVE-04 | Phase 2 | Pending |
| MOVE-05 | Phase 2 | Pending |
| TEST-03 | Phase 2 | Pending |
| COPY-01 | Phase 3 | Pending |
| COPY-02 | Phase 3 | Pending |
| COPY-03 | Phase 3 | Pending |
| DOC-01 | Phase 3 | Pending |
| DOC-02 | Phase 3 | Pending |
| DOC-03 | Phase 3 | Pending |
| DOC-04 | Phase 3 | Pending |
| TEST-05 | Phase 3 | Pending |
| TEST-06 | Phase 3 | Pending |
| TEST-07 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 53 total
- Mapped to phases: 53
- Unmapped: 0

---
*Requirements defined: 2026-04-05*
*Last updated: 2026-04-04 after roadmap creation*
