# Phase 3: COPY, Compliance, and Documentation - Context

**Gathered:** 2026-04-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Finaliser la surface WebDAV v1 : implémenter COPY (fichier + dossier récursif) conforme RFC 4918 §9.8, valider la conformité RFC 4918 Class 1 via le test suite `litmus` en strict zéro-skip, et documenter la feature pour les utilisateurs et opérateurs. Cible : sign-off complet pour la milestone v1 (53/53 requirements).

Hors scope (maintenu depuis Phase 1/2) : LOCK/UNLOCK, PROPPATCH, dead properties, quota properties, métriques WebDAV dédiées, rate limiting WebDAV-spécifique, Digest Auth, app-specific passwords, Swift server-side COPY. Tout reste reporté à v2.

**Scope reduction en Phase 3** : validation manuelle iOS Files est **déferée à v1.1** (décision explicite durant discussion). REQUIREMENTS.md doit refléter ce changement — iOS Files compatibility devient "best-effort, non vérifié manuellement, couvert indirectement par litmus Class 1 strict".

</domain>

<decisions>
## Implementation Decisions

### TDD litmus — stratégie d'exécution

- **Exécution 100% locale pour cette phase** — CI (`.github/workflows/system-tests.yml`) **non touchée** en Phase 3. Intégration CI reportée post-v1 (probable v1.1). Le cycle TDD se fait via un script bash ou une cible Makefile `make test-litmus` (naming à décider par le planner).
- **Critère de succès : Class 1 strict, zéro skip** — les 5 suites litmus standard (`basic`, `copymove`, `props`, `http`, `locks`) doivent toutes être green. Même la suite `locks` doit retourner un comportement propre que litmus accepte : pas de `SKIP`, pas de fail. Concrètement cela signifie que les méthodes LOCK/UNLOCK doivent retourner un code propre (501 Not Implemented ou 405 Method Not Allowed avec `Allow:` header) que litmus interprète correctement.
- **Note importante**: avant la discussion, la décision Phase 1/2 était "pas de LOCK/UNLOCK v1". Elle reste valide — on n'implémente PAS le locking. Mais on doit s'assurer que le handler retourne un status code que litmus `locks` suite ne signale pas comme un échec. À vérifier en recherche phase : si ce n'est pas possible, on bascule vers "locks suite skipped via LITMUS_TESTS env var" avec justification documentée.

### TDD litmus — découpage des commits

- **Un plan TDD par famille litmus** — chaque suite qui révèle des gaps devient son propre plan (`03-0X-PLAN.md`). Exemple de découpage probable :
  - `03-01-PLAN.md` — Script d'init litmus + stack running + first run pour faire la liste des gaps
  - `03-02-PLAN.md` — Implémentation COPY fichier + dossier (conforme RFC 4918 §9.8)
  - `03-03-PLAN.md` — Litmus `basic` suite green
  - `03-04-PLAN.md` — Litmus `copymove` suite green
  - `03-05-PLAN.md` — Litmus `props` suite green
  - `03-06-PLAN.md` — Litmus `http` suite green
  - `03-07-PLAN.md` — Litmus `locks` suite green (via 501/405 propres ou skip justifié)
  - `03-08-PLAN.md` — Documentation (`docs/webdav.md` + TOC)
  - Ordre à confirmer par le planner — le gros gap (COPY) doit être implémenté AVANT de lancer litmus `copymove`.
- **Chaque plan suit RED → GREEN → REFACTOR avec commits séparés** — non-négociable, hérité de Phase 1/2.

### TDD litmus — setup instance et routes

- **Instance fraîche horodatée à chaque run** — le script crée une instance Cozy jetable (nom type `litmus-YYYYMMDD-HHMMSS.localhost:8080`), génère un token app via `cozy-stack instances token-app`, lance litmus, et détruit l'instance en fin de run. Reproductibilité parfaite, pas de pollution des instances de dev (`brice`, `alice`).
- **Les deux routes testées** — litmus est exécuté DEUX fois dans chaque run :
  1. Contre `/dav/files/` (route native)
  2. Contre `/remote.php/webdav/` (route Nextcloud, via le proxy `/tmp/cozy-webdav-proxy.go` qui réécrit le `Host` header)
  Les deux runs doivent être green. Cela garantit anti-régression pour la fix de route du commit `7c9ab3a59` (`fix(webdav): serve /remote.php/webdav/* directly instead of 308 redirect`) et détecte toute divergence entre les deux handlers.
- **Script d'init** — à mettre dans `scripts/webdav-litmus.sh` (ou équivalent cohérent avec le layout du repo). Le planner décide du nom exact et de la structure.

### COPY fichier — sémantique

- **Overwrite: T + destination existante → trash-then-copy** : on déplace la destination existante dans `.cozy_trash` (via `vfs.TrashFile`) puis on appelle `vfs.CopyFile(olddoc, newdoc)` pour écrire la nouvelle copie. L'utilisateur peut récupérer l'ancien fichier depuis la trash. **Symétrique à la décision MOVE de Phase 2** — préserve la cohérence "DELETE = trash, tout overwrite passe par la trash".
- **Overwrite: F + destination existante → 412 Precondition Failed** — conforme RFC 4918 §9.8.5. Body XML via `sendWebDAVError` (réutilise Phase 1 error builder).
- **Overwrite header absent → T par défaut** — conforme RFC 4918 §10.6 : "If this header is not included in the request, the server MUST default to behaving as though an Overwrite header with a value of T was included." **Identique à MOVE de Phase 2** — contourne le bug `x/net/webdav` #66059.
- **Cozy Notes special case** — pour tout fichier dont `olddoc.Mime == consts.NoteMimeType`, utiliser `note.CopyFile(inst, olddoc, newdoc)` au lieu de `fs.CopyFile(olddoc, newdoc)`. **Réplique exactement la branche de `web/files/files.go:397-402`**. Sans cette special case, les notes copiées perdraient leurs images attachées.
- **Construction du newdoc** — utiliser `vfs.CreateFileDocCopy(olddoc, destinationDirID, copyName)` comme le fait `web/files/files.go:373`. Ce helper gère déjà la copie des métadonnées (mime, tags, cozyMetadata, etc.).

### COPY dossier — sémantique récursive

- **Implémentation : `vfs.Walk` + `CopyFile` par fichier** — conforme REQUIREMENTS.md COPY-02. Pattern :
  1. Créer le dossier de destination via `vfs.Mkdir` (single level, pas `MkdirAll` — même raison que MKCOL en Phase 2, race condition dans `vfs.MkdirAll`)
  2. `vfs.Walk(fs, srcPath, walkFn)` qui visite récursivement le sous-arbre
  3. Dans `walkFn` : pour chaque `FileDoc` rencontré, calculer le chemin destination relatif, créer les sous-dossiers manquants, appeler `fs.CopyFile(olddoc, newdoc)` (ou `note.CopyFile` si applicable).
  4. Collecter les erreurs per-child pour le 207 Multi-Status (cf. décision suivante).
- **Pas d'optimisation Swift server-side en v1** — même si `vfsswift/impl_v3.go:606` expose `CopyFileFromOtherFS`, on reste sur le walk simple pour v1. Optimisation reportée à v2 (déjà dans `ADV-V2-04` de REQUIREMENTS.md).
- **Erreur mid-copy → 207 Multi-Status** conforme RFC 4918 §9.8.8. Le body XML contient un `<D:response>` par chemin qui a échoué avec son code HTTP. Les fichiers déjà copiés avec succès **restent en place** — pas de rollback. **C'est ce que litmus `copymove` strict attend** — obligatoire pour zéro-skip.
- **Depth header supporté** :
  - `Depth: 0` sur un dossier → copie le dossier vide (juste le container, sans le contenu)
  - `Depth: infinity` sur un dossier → copie récursive complète
  - `Depth` absent → `infinity` par défaut (RFC 4918 §9.8.3 : "A client may submit a Depth header on a COPY on a collection with a value of '0' or 'infinity'. The COPY instruction can be used without a Depth header, in which case Depth:infinity is assumed.")
  - `Depth: 1` **refusé** avec 400 Bad Request — RFC 4918 interdit explicitement `Depth: 1` pour COPY/MOVE.
- **Pas de cap dur** sur le nombre de fichiers ou la profondeur — cohérent avec la décision PROPFIND de Phase 1 ("streaming illimité, pas de cap dur"). Le quota global Cozy plafonne naturellement les copies massives.

### Tests de comportement clients (TEST-05) — approche pragmatique

- **OnlyOffice mobile : litmus Class 1 strict + E2E gowebdav = TEST-05 validé par transitivité**. Justification : le bug `LoginComponent null` de OnlyOffice mobile v9.3.1 est un bug **client**, pas serveur. Si litmus passe en strict ET les E2E gowebdav Phase 2 passent, le serveur est conforme. Un fix OnlyOffice v9.3.2+ permettra le test manuel réel, mais il ne bloque pas la phase.
- **iOS Files : DEFERRED à v1.1** — **⚠️ réduction de scope explicite**. REQUIREMENTS.md doit être mis à jour par le planner pour refléter que "iOS/iPadOS Files compatibility" devient : "best-effort, couvert indirectement par litmus Class 1 strict, validation manuelle déférée à v1.1". La roadmap v1 ne liste plus iOS Files comme critère de succès bloquant.
- **Tests client fake** — si litmus passe en strict et qu'il reste du budget, créer `web/webdav/clients_test.go` (nouveau fichier) avec des sous-tests reproduisant des séquences HTTP spécifiques. Exemple : `TestOnlyOffice_OpenEditSave` qui enchaîne `OPTIONS → PROPFIND / → PROPFIND /Documents → GET file.docx → PUT file.docx → PROPFIND`. Capture HAR ou inspection de la séquence réelle OnlyOffice comme référence. **C'est du bonus, pas un gate** — si la planification est trop chargée, ces tests sont les premiers à sauter.
- **Ordre TDD strict** :
  1. D'abord litmus local green en strict (c'est ce qui révèle les vrais gaps RFC)
  2. Ensuite éventuellement les tests client fake
  3. Documentation en dernier (mais peut être parallèle à litmus si plans séparés)

### Documentation (DOC-01 à DOC-04)

- **Un seul fichier `docs/webdav.md`** — style `docs/nextcloud.md` (fichier monolithique existant qui documente une feature d'intégration). Ajout dans `docs/toc.yml` obligatoire.
- **Langue : anglais** — cohérent avec TOUS les autres fichiers de `docs/` (files.md, nextcloud.md, office.md, auth.md, sharing.md, etc. sont tous en anglais). Audience : contributeurs cozy-stack, intégrateurs, devs clients tiers. PROJECT.md/REQUIREMENTS.md restent en français (convention interne planning), `docs/` reste en anglais (convention publique repo).
- **Structure suggérée** (le planner peut ajuster) :
  1. Introduction & concepts (what is cozy-stack WebDAV, scope, what it is NOT)
  2. Endpoints & routes (`/dav/files/*`, `/remote.php/webdav/*` compatibility)
  3. Authentication (OAuth Bearer, Basic with token-as-password)
  4. Supported methods — narrative + table + **exemples curl inline dans la narration** (préférence explicite utilisateur : chaque méthode a son bloc de narration avec 1-2 `curl` d'exemple directement dans le texte, pas juste une table sèche)
  5. Client configuration (step-by-step texte pour OnlyOffice mobile, rclone, Fichiers iOS, curl)
  6. Compatibility notes & limitations (no LOCK/UNLOCK, PROPFIND Depth restrictions, Finder read-only without LOCK, `.cozy_trash` read-only, etc.)
  7. Troubleshooting (common errors, authentication debugging, how to see logs)
- **Pas de screenshots** — exemples de config clients sont textuels uniquement (captures périment trop vite, alourdissent le repo).
- **OpenAPI (DOC-04)** — vérifier d'abord si le repo a d'autres specs OpenAPI (grep `openapi`, `swagger`). Si oui, ajouter un spec pour WebDAV. Si non, DOC-04 est satisfait par le `docs/webdav.md` narrative + table (REQUIREMENTS dit "OpenAPI **ou équivalent**").

### Testing discipline — hérité

- **TDD RED → GREEN → REFACTOR** avec commits séparés : non-négociable, hérité de Phase 1/2
- **Jamais de mock VFS** dans les tests handlers — utiliser le VFS de test (afero/mem-backed) via le harness `newWebdavTestEnv` de Phase 1
- **Tests d'intégration `gowebdav`** pour COPY : étendre `web/webdav/gowebdav_integration_test.go` avec un nouveau sous-test `SuccessCriterion6_Copy` (ou équivalent)
- **Tests litmus = tests d'intégration externes** — ils ne remplacent PAS les tests Go unitaires et d'intégration. Les deux couches coexistent.

### Claude's Discretion

- Exact naming du script litmus (`scripts/webdav-litmus.sh`, `Makefile` cible, etc.)
- Exact layout du code COPY (un fichier `copy.go` / `copy_test.go`, ou greffé sur `move.go` ?)
- Forme précise du 207 Multi-Status error XML pour les échecs partiels de COPY dir
- Forme précise du `LITMUS-GAPS.md` s'il est nécessaire (en cas d'échec d'atteinte du "zéro skip")
- Structure exacte du helper Walk → Copy (typed walker vs closure)
- Ordonnancement précis des plans à l'intérieur de la phase (sous la contrainte "litmus d'abord, clients fake ensuite, docs en dernier ou parallèle")
- Détails du cleanup d'instance litmus (signal trap, defer, etc.)
- Forme précise des exemples curl dans `docs/webdav.md` (verbose, avec `-i` ou pas, tokens masqués, etc.)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project et requirements (MANDATORY)
- `.planning/PROJECT.md` — Vision, core value, constraints, key decisions, **mettre à jour** après Phase 3 pour refléter "Validated" sur COPY et litmus
- `.planning/REQUIREMENTS.md` — Phase 3 requirements : COPY-01/02/03, DOC-01/02/03/04, TEST-05/06/07. **⚠️ À mettre à jour en Phase 3 : iOS Files validation déférée à v1.1**
- `.planning/ROADMAP.md` — Phase 3 success criteria (4 critères : COPY, litmus, E2E scenario, docs)

### Phase 1 + Phase 2 context (MANDATORY — décisions carry forward)
- `.planning/phases/01-foundation/01-CONTEXT.md` — Auth, path safety, XML format, error format, audit logging, trash read-only, TDD discipline, `.cozy_trash` read-only via WebDAV
- `.planning/phases/02-write-operations/02-CONTEXT.md` — DELETE = trash, MOVE trash-then-rename (→ COPY applique le même pattern), Overwrite=T default (contourne bug #66059), MKCOL = Mkdir (pas MkdirAll), error mapping VFS → HTTP, lock-token If: headers ignorés, audit logging pour writes
- `.planning/phases/01-foundation/01-VERIFICATION.md` — Phase 1 verification, known caveats (FOLLOWUP-01 sur `-race`)
- `.planning/phases/02-write-operations/02-VERIFICATION.md` — Phase 2 verification et tests passing

### Research (lire avant planification si pas encore fait)
- `.planning/research/SUMMARY.md` — Global synthesis
- `.planning/research/STACK.md` — Library choices, gowebdav v0.12.0 pour tests, custom handlers (pas x/net/webdav)
- `.planning/research/ARCHITECTURE.md` — VFS integration, build order
- `.planning/research/FEATURES.md` — WebDAV methods required per client, compat matrix
- `.planning/research/PITFALLS.md` — 20 pitfalls (Overwrite bug #66059, MkdirAll race, Finder lock tokens, etc.)

### Codebase (MANDATORY — understand WebDAV + VFS code layout)
- `.planning/codebase/ARCHITECTURE.md` — Multi-tenant, VFS interface, Echo middleware
- `.planning/codebase/STRUCTURE.md` — `web/{domain}/` pattern
- `.planning/codebase/CONCERNS.md` — `vfs.MkdirAll` race condition
- `.planning/codebase/TESTING.md` — Test harness, instance setup patterns
- `.planning/codebase/CONVENTIONS.md` — Go style, error handling, naming

### Code références (VFS COPY primitives + patterns)
- `model/vfs/vfs.go:95-97` — `VFS.CopyFile(olddoc, newdoc *FileDoc) error` — interface method
- `model/vfs/vfs.go:129-132` — `CopyFileFromOtherFS` (optimisation v2, non utilisée en v1)
- `model/vfs/vfs.go:622` — `vfs.Walk(fs Indexer, root string, walkFn WalkFn) error` — recursive walker
- `model/vfs/vfs.go:618` — `WalkFn` type signature
- `model/vfs/vfsafero/impl.go:250` — `aferoVFS.CopyFile` implementation
- `model/vfs/vfsswift/impl_v3.go:259` — `swiftVFSV3.CopyFile` implementation
- `web/files/files.go:358-408` — **Reference implementation** pour le pattern `CopyFile` JSON:API : `CreateFileDocCopy` + `ConflictName` + `note.CopyFile` branch + `fs.CopyFile`. **À étudier avant de coder le handler WebDAV** — le planner doit décider quelles parties réutiliser vs adapter.
- `web/webdav/move.go` — Pattern trash-then-rename de Phase 2, à répliquer en trash-then-copy pour COPY
- `web/webdav/write_helpers.go:parseDestination` — Valide + decode le Destination header pour les deux routes (`/dav/files` et `/remote.php/webdav`), réutilisable pour COPY

### WebDAV tests existants à étendre
- `web/webdav/gowebdav_integration_test.go` — E2E harness Phase 1/2, à étendre avec `SuccessCriterion6_Copy` (ou nom équivalent)
- `web/webdav/testutil_test.go` — Helpers `newWebdavTestEnv`, `seedFile`, etc.

### RFC et external specs (MANDATORY)
- **RFC 4918** (WebDAV HTTP Extensions) — https://www.rfc-editor.org/rfc/rfc4918
  - §9.8 COPY Method (full spec, destination semantics, Depth handling)
  - §9.8.3 COPY for Collections (Depth semantics)
  - §9.8.5 Status Codes (201, 204, 207, 403, 409, 412, 423, 502, 507)
  - §9.8.8 207 Multi-Status response body format for partial failures
  - §10.4 Destination header
  - §10.6 Overwrite header (T default)
- **x/net/webdav bug #66059** — Overwrite absent defaults to F (wrong) — already worked around in Phase 2 move handler
- **litmus test suite** — `man litmus` local, https://github.com/tolsen/litmus (source), 5 suites : basic, copymove, props, http, locks
- **RFC 2518** (original WebDAV) — historical reference, superseded by 4918

### Documentation references (pour écrire docs/webdav.md)
- `docs/nextcloud.md` — Style reference : intro, config, compatibility notes, troubleshooting
- `docs/files.md` — L'API files JSON:API documentée de manière parallèle
- `docs/office.md` — Intégration OnlyOffice côté stack, à croiser avec la section OnlyOffice de docs/webdav.md
- `docs/toc.yml` — Table of contents à mettre à jour avec l'entrée `webdav.md`

### CI (pour plus tard, post-v1)
- `.github/workflows/system-tests.yml` — Slot naturel pour litmus CI quand on décidera d'ajouter le gate (installe déjà CouchDB 3.3.3 + Ruby 2.7 + Mailhog + Go 1.25, appelle `make system-tests`). Extension future : ajouter une cible Makefile `test-litmus` appelée depuis `system-tests`. **Hors scope Phase 3.**

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- **`model/vfs/vfs.go:VFS.CopyFile(olddoc, newdoc)`** — primitive VFS qui copie le contenu d'un fichier. Implémentée par `aferoVFS` et `swiftVFSV3`. Signature identique à ce que `web/files/files.go:401` utilise déjà.
- **`model/vfs/vfs.go:CreateFileDocCopy(olddoc, destinationDirID, copyName)`** — helper qui construit un `FileDoc` de destination à partir d'une source (copie les métadonnées pertinentes, reset ID/rev, etc.). Utilisé par `web/files/files.go:373`.
- **`model/vfs/vfs.go:Walk(fs, root, walkFn)`** — walker récursif fileset à la `filepath.Walk`. `WalkFn(name, dir, file, err)` permet de traiter dossiers et fichiers uniformément. **Fondation pour COPY récursif**.
- **`web/files/files.go:CopyFile` handler** — reference implementation complète : mapping source → destination, détection notes, ConflictName, CopyFile ou note.CopyFile, retour FileData. **À étudier mais NE PAS réutiliser directement** — c'est un handler JSON:API avec des paramètres différents (file-id, DirID, Name) du Destination header WebDAV.
- **`note.CopyFile(inst, olddoc, newdoc)`** — package `model/note`, handle la copie spéciale des notes (avec images attachées). À importer dans le handler WebDAV COPY.
- **`web/webdav/write_helpers.go:parseDestination`** — Déjà en place pour MOVE, **supporte les deux préfixes** (`/dav/files` ET `/remote.php/webdav`) depuis le commit `7c9ab3a59`. Réutilisable tel quel pour COPY.
- **`web/webdav/move.go`** — Pattern trash-then-rename de Phase 2, le plus proche de ce dont COPY a besoin. Le handler COPY peut s'inspirer de la structure : `parseDestination`, vérification existence, branche Overwrite, action VFS, code succès.
- **`web/webdav/errors.go:sendWebDAVError`** — construction XML RFC 4918 §8.7 pour les erreurs. Réutilisable pour 412, 409, 403, 507.
- **`web/webdav/handlers.go:handlePath`** — dispatcher méthode → handler. Ajouter un case `"COPY"` qui appelle le nouveau `handleCopy`.
- **`web/webdav/testutil_test.go:newWebdavTestEnv`** — harness Phase 1/2, fournit un test env avec Echo + httptest server + instance Cozy + token. À réutiliser pour les tests COPY.
- **`web/webdav/gowebdav_integration_test.go`** — fichier E2E Phase 1/2, à étendre avec les tests COPY.

### Established Patterns

- **handlePath dispatcher** : method switch, chaque méthode WebDAV a son handler dédié. Phase 3 ajoute `handleCopy` au dispatch.
- **Trash-then-X pour les overwrites** : pattern Phase 2 (DELETE = trash, MOVE Overwrite=T = trash-then-rename). COPY Overwrite=T applique le même pattern → trash-then-copy. **Cohérence critique pour la philosophie safety-first de Cozy**.
- **VFS write pattern** : `fs.CreateFile(newdoc, olddoc)` pour PUT. Pour COPY on utilise `fs.CopyFile(olddoc, newdoc)` qui est l'équivalent sans avoir à streamer depuis le body HTTP (le VFS lit directement le fichier source).
- **Error handling** : VFS errors sont typed (`os.ErrNotExist`, `os.ErrExist`, `vfs.ErrFileTooBig`, etc.). `mapVFSWriteError` de Phase 2 mappe déjà vers HTTP — à étendre si nécessaire pour COPY-specific errors.
- **Test harness** : `newWebdavTestEnv` + `seedFile` + `env.E.Request(...)` via httpexpect — pattern hérité de Phase 1/2.
- **Instance-aware middleware** : `NextcloudRoutes` est wrappée avec un middleware qui injecte `c.Set("instance", inst)` — le handler COPY doit fonctionner identiquement sous les deux routes.

### Integration Points

- **`web/webdav/handlers.go`** : ajouter case `"COPY"` dans le switch `handlePath`, appelant `handleCopy(c)` (nouveau)
- **Nouveau fichier `web/webdav/copy.go`** (pressenti, Claude's Discretion) : `handleCopy` + helpers spécifiques COPY + 207 Multi-Status builder
- **Nouveau fichier `web/webdav/copy_test.go`** : tests unitaires handler + table-driven cases (Overwrite T/F, source file/dir, dest existante/absente, notes, partial failures)
- **`web/webdav/gowebdav_integration_test.go`** : nouveau sous-test E2E COPY via gowebdav
- **`web/webdav/clients_test.go`** (nouveau, bonus) : scénarios clients OnlyOffice/rclone/curl
- **`scripts/webdav-litmus.sh`** (ou équivalent Makefile cible, Claude's Discretion) : script d'orchestration local pour le TDD litmus (create instance, generate token, run stack, run litmus x2 routes, cleanup)
- **`docs/webdav.md`** (nouveau) : documentation utilisateur/opérateur
- **`docs/toc.yml`** : ajouter entrée pour `webdav.md`
- **`web/webdav/handlers.go:webdavMethods`** / **`web/webdav/options.go`** : le `Allow:` header OPTIONS doit déjà inclure `COPY` depuis Phase 2 (le plan 02-05 a ajouté les méthodes write). **À vérifier** — si `COPY` n'est pas dans l'Allow, l'ajouter fait partie de la wave 1 de Phase 3.
- **`.planning/REQUIREMENTS.md`** : update Phase 3 status + reflect iOS Files deferral to v1.1

### Deferred pour v2 (rappel)

- **Swift server-side COPY** (`CopyFileFromOtherFS`) — optimisation multi-backend, `ADV-V2-04` déjà dans REQUIREMENTS.md
- **LOCK/UNLOCK** — hors scope v1 complet
- **Quota properties** — v2
- **Métriques WebDAV** — v2
- **CI litmus integration** — post-v1 (v1.1 probable)
- **Test manuel OnlyOffice** — bloqué par bug client v9.3.1, reprend quand fix upstream dispo
- **Test manuel iOS Files** — déféré à v1.1 (décision explicite Phase 3)

</code_context>

<specifics>
## Specific Ideas

- **"TDD très complets en utilisant des batteries de test"** — formulation explicite de l'utilisateur. Traduction directe : litmus est la batterie de test, l'approche est strict zéro-skip. C'est la décision la plus structurante de la phase.
- **"On fait les tests TDD en local, on verra plus tard pour la CI"** — CI litmus explicitement reportée post-v1. Pas de `.github/workflows/` modification en Phase 3. Implication : `make test-litmus` (ou équivalent) doit être exécutable uniquement par le dev qui a litmus installé localement.
- **Litmus est déjà installé localement** — `/usr/bin/litmus`, version 0.13, via `sudo apt install litmus`. Validé par l'utilisateur post-crash.
- **Les deux routes doivent passer la même suite** — réflexe anti-régression sur la fix de route commit `7c9ab3a59`. C'est la manière la plus forte de vérifier que la route Nextcloud n'a pas divergé de la native.
- **Symétrie MOVE ↔ COPY** — le handler COPY doit être structurellement jumeau de MOVE (même parseDestination, même gestion Overwrite, même trash-then-X). Seul le verbe VFS change (`ModifyFileMetadata` → `CopyFile`).
- **Notes Cozy ne sont PAS transparentes au niveau WebDAV** — un `.cozy-note` est exposé comme un fichier via WebDAV, mais sa copie nécessite le special case `note.CopyFile` pour ne pas perdre les images. C'est le piège principal du handler COPY.
- **"Narrative + table + exemples curl inline dans la narration"** — note explicite utilisateur pour `docs/webdav.md`. Le format n'est pas "table sèche + blocs de code séparés" mais "paragraphe qui explique + table récap + `curl` au milieu du texte dans des code blocks courts".
- **Anglais pour docs/**, **français pour .planning/** — convention du repo, respecter strictement.
- **iOS Files deferral est une scope reduction explicite** — le planner DOIT updater REQUIREMENTS.md pour refléter ce changement. Ce n'est pas un simple oubli, c'est une décision consciente prise durant cette discussion.

</specifics>

<deferred>
## Deferred Ideas

- **CI integration de litmus** — reportée post-v1 (v1.1 probable). Slot technique identifié : `.github/workflows/system-tests.yml` + Makefile cible. Décision consciente : "on voit plus tard pour la CI".
- **Test manuel OnlyOffice mobile** — bloqué par bug client v9.3.1 (`LoginComponent null`). Reprend quand fix upstream dispo. v1.1 ou v1.0.1.
- **Test manuel iOS Files** — déféré à v1.1. iOS Files reste une cible de compat, mais la validation manuelle n'est plus un gate de la v1.
- **Swift server-side COPY** — optimisation `CopyFileFromOtherFS` pour éviter le download-reupload inter-backend. Gain probable significatif pour les gros fichiers. Déjà tracé comme `ADV-V2-04` dans REQUIREMENTS.md.
- **LITMUS-GAPS.md** — si malgré les efforts on ne peut PAS atteindre "Class 1 strict zéro skip" (ex: la suite `locks` refuse obstinément de valider un 501/405 propre), créer un document qui explique pourquoi chaque gap restant est inévitable. Non désiré mais safety net documentaire.
- **Client fake tests "OnlyOffice full sequence"** — reproduction HAR exacte du workflow OnlyOffice. Bonus si budget, sinon déféré.
- **OpenAPI spec dédié pour WebDAV** — seulement si le repo a d'autres specs OpenAPI. Sinon `docs/webdav.md` satisfait DOC-04 ("OpenAPI ou équivalent").
- **Documentation bilingue FR/EN** — pas dans v1. Convention `docs/` reste anglais uniquement.
- **Cap dur sur COPY récursif** — pas en v1. À évaluer si des problèmes de DoS ou d'OOM émergent en prod.

</deferred>

---

*Phase: 03-copy-compliance-and-documentation*
*Context gathered: 2026-04-11*
