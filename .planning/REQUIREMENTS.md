# Requirements: Cozy WebDAV — Milestone v1.2 Robustness

**Defined:** 2026-04-13
**Core Value:** Au-delà de la conformité litmus (v1.1), prouver que le serveur WebDAV est correct dans les scénarios réels que le standard ne teste pas — gros fichiers streaming, transferts interrompus sans corruption, accès concurrents de petite échelle avec sémantique déterministe.

**Explicit non-goal:** Ce milestone ne fait **pas** de tests de charge / performance à grande échelle. Voir section *Out of Scope*.

## v1.2 Requirements

### Instrumentation (INSTR)

Prérequis de correction pour les tests de gros fichiers et concurrence. Pas d'instrumentation de perf.

- [x] **INSTR-01** : Helper de mesure mémoire concurrent — sampler `runtime.ReadMemStats` pendant une opération en vol, retourne peak `HeapInuse` observé. Objectif : prouver le streaming, pas mesurer la vitesse.
- [x] **INSTR-02** : Helper streaming-SHA256 drain — pattern `io.TeeReader(body, sha256.New())` → `io.Discard`, consomme un gros body côté test sans accumulation mémoire. Proscrit `io.ReadAll` et `httpexpect.Body().Raw()` sur les gros transferts.
- [x] **INSTR-03** : Fixture streaming — générateur `io.LimitReader(rand.Reader, N)` + seed déterministe, zero fixture binaire checked-in dans git.

### Gros volumétrie (LARGE)

Tests de correction sous gros volume. Aucune assertion de débit ou latence.

- [x] **LARGE-01** : PUT d'un fichier de 1 GB end-to-end via gowebdav — assert que le peak `HeapInuse` serveur pendant le transfert reste < 128 MB. Prouve le streaming, ne mesure pas la vitesse.
- [x] **LARGE-02** : GET d'un fichier de 1 GB end-to-end en streaming côté client — même ceiling mémoire serveur, checksum body vérifié via INSTR-02.

### Transferts interrompus (INTERRUPT)

- [ ] **INTERRUPT-01** : Un PUT interrompu mid-transfer sur un path vierge laisse le VFS dans un état propre — un GET subséquent retourne 404, et il n'existe ni blob orphelin ni doc CouchDB orphelin.
- [ ] **INTERRUPT-02** : Un PUT interrompu mid-transfer en overwrite d'un fichier existant laisse le fichier original intact, avec son ETag original. La rollback ne supprime jamais le fichier pré-existant.
- [ ] **INTERRUPT-03** : Un PUT avec un header `Content-Range` est rejeté explicitement avec 501 Not Implemented et un header `Allow` listant les méthodes supportées. Pas de corruption silencieuse (sparse file).

### Byte-range GET (RANGE)

- [ ] **RANGE-01** : Un GET avec `Range: bytes=X-Y` retourne 206 Partial Content avec un header `Content-Range` correct et le corps attendu.
- [ ] **RANGE-02** : Un GET avec `Range: bytes=X-Y,A-B` retourne 206 avec un body `multipart/byteranges` bien formé (headers de parts inclus).
- [ ] **RANGE-03** : Un GET avec un range invalide (ex : `bytes=999999999-`) retourne 416 Range Not Satisfiable avec un header `Content-Range: bytes */{size}`.

### Concurrence de correction (CONC)

**Tests de correction uniquement, 2-3 goroutines maximum, pas de boucles de load.**

- [ ] **CONC-01** : Deux clients qui font PUT simultanément sur le même path produisent un résultat déterministe — premier à Close gagne, second reçoit une erreur claire (jamais 500). Le blob perdant est nettoyé côté Swift/VFS.
- [ ] **CONC-02** : Un conflit CouchDB (409 MVCC) côté stack est mappé vers HTTP 409 Conflict ou 503 Service Unavailable côté client — jamais 500 Internal Server Error.
- [ ] **CONC-03** : Un PROPFIND émis pendant un PUT en vol retourne un état cohérent du fichier cible — soit les anciennes métadonnées, soit les nouvelles, jamais un mix (pas de lecture sale).
- [ ] **CONC-04** : Chaque test concurrent utilise `goleak.VerifyNone(t)` en cleanup pour détecter les goroutines qui survivent au test.

### Dette technique (DEBT)

- [x] **DEBT-01** : La race préexistante dans `pkg/config` / `model/stack` / `model/job.AntivirusTrigger` (FOLLOWUP-01 hérité de v1.1) est corrigée. `go test -race -count=1 ./web/webdav/...` s'exécute proprement (zéro WARNING: DATA RACE).
- [x] **DEBT-02** : `web/webdav/testutil_test.go` — la signature de `newWebdavTestEnv` passe de `*testing.T` à `testing.TB` pour supporter les benchmarks (`*testing.B`). Aucun breakage des callers existants.

### Intégration CI (CI)

- [ ] **CI-01** : Un workflow `.github/workflows/webdav-litmus.yml` exécute litmus contre les deux routes (`/dav/files/` + `/remote.php/webdav/`) à chaque push sur `master` et chaque PR qui touche `web/webdav/`. Utilise l'image Docker `owncloud/litmus` (éviter `apt install litmus` qui installe 0.13 obsolète). Inclut un wait-loop sur CouchDB `/_up`.
- [ ] **CI-02** : Les résultats litmus apparaissent dans le summary GitHub Actions (tableau pass/fail par suite) et sont visibles depuis les PR checks.
- [ ] **CI-03** : Les tests lourds v1.2 (LARGE en Phase 5, CONC si nécessaire en Phase 7) portent le garde `if testing.Short() { t.Skip(...) }`. Le workflow `go-tests.yml` existant est modifié pour passer `-short` (CI rapide par défaut, PRs tous packages). Un nouveau workflow `.github/workflows/webdav-heavy.yml` exécute la suite complète sans `-short`, avec timeout 30 min, sur push master et sur PRs touchant `web/webdav/**`. Les runs locaux (`go test ./...` sans flag) restent exhaustifs par défaut.

### Validation manuelle (VAL)

- [ ] **VAL-01** : iOS/iPadOS Files app — checklist documentée de ~10 étapes (connexion, listing, upload depuis Photos, download vers Files, rename, move, compat Pages/Numbers/Keynote) exécutée sur un device iOS réel, résultats tracés dans `v1.2-MANUAL-VALIDATION-IOS.md`.

## v2 Requirements

Tracked mais non scopés pour v1.2.

### Dead property persistence (DEADPROP)

- **DEADPROP-01** : PROPPATCH properties persistées dans CouchDB (actuellement in-memory, perdues au restart)

### Nextcloud compatibility (NEXTCLOUD)

- **NEXTCLOUD-01** : API OCS minimale (`/ocs/v1.php/cloud/capabilities` + `/ocs/v1.php/cloud/user`) pour que Nextcloud Desktop Sync et OO Desktop en mode Nextcloud fonctionnent contre Cozy
- **NEXTCLOUD-02** : Chunked upload Nextcloud-style (`/remote.php/dav/uploads/{user}/{id}/{chunk}` + MOVE-to-finalize)

### WebDAV Class 2 (LOCK)

- **LOCK-01** : LOCK/UNLOCK support — ouvre la compat macOS Finder en read-write

## Out of Scope

Explicitement exclus. Raisonnement documenté pour prévenir le scope creep.

| Feature | Reason |
|---------|--------|
| **Load testing (N clients massivement parallèles)** | Requiert infrastructure dédiée pour être représentatif — réseau contrôlé, volumes réalistes, monitoring stack. Un `go test` in-process produit des chiffres qui ne transposent pas en production. |
| **Benchmark throughput / latency sous charge** | Même raison. Milestone Performance dédié plus tard si un besoin produit concret émerge. |
| **Tests d'endurance (heures / jours)** | Même raison. Relève de l'ops cozy-stack global, pas du package WebDAV. |
| **Capacity planning (connexions simultanées max)** | Relève de la config cozy-stack globale (ulimits, Echo workers), pas du WebDAV. |
| **Benchmark comparatif vs autres serveurs WebDAV** | Pas de besoin produit identifié. Hors scope d'un milestone de correction. |
| **Chunked upload Nextcloud-style** | Protocole propriétaire, hors clients target v1/v1.1/v1.2. Déféré à v2 sous NEXTCLOUD-02. |
| **Dead property persistence CouchDB** | Scope v2 (DEADPROP-01). v1.2 reste sur store in-memory. |
| **LOCK/UNLOCK (Class 2)** | Scope v2 (LOCK-01). |
| **API OCS Nextcloud** | Scope v2 (NEXTCLOUD-01). Nécessite un chantier à part. |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| INSTR-01 | Phase 4 | Complete |
| INSTR-02 | Phase 4 | Complete |
| INSTR-03 | Phase 4 | Complete |
| LARGE-01 | Phase 5 | Complete |
| LARGE-02 | Phase 5 | Complete |
| INTERRUPT-01 | Phase 6 | Pending |
| INTERRUPT-02 | Phase 6 | Pending |
| INTERRUPT-03 | Phase 6 | Pending |
| RANGE-01 | Phase 6 | Pending |
| RANGE-02 | Phase 6 | Pending |
| RANGE-03 | Phase 6 | Pending |
| CONC-01 | Phase 7 | Pending |
| CONC-02 | Phase 7 | Pending |
| CONC-03 | Phase 7 | Pending |
| CONC-04 | Phase 7 | Pending |
| DEBT-01 | Phase 4 | Complete |
| DEBT-02 | Phase 4 | Complete |
| CI-01 | Phase 8 | Pending |
| CI-02 | Phase 8 | Pending |
| CI-03 | Phase 8 | Pending |
| VAL-01 | Phase 9 | Pending |

**Coverage:**
- v1.2 requirements: 21 total
- Mapped to phases: 21/21 ✓
- Unmapped: 0

---
*Requirements defined: 2026-04-13*
*Last updated: 2026-04-13 — traceability filled by roadmapper*
