# Phase 4: Prerequisites and Instrumentation - Context

**Gathered:** 2026-04-13
**Status:** Ready for planning

<domain>
## Phase Boundary

Préparer l'environnement de test du package `web/webdav/` pour accueillir les tests lourds des Phases 5 à 7 :

1. **Éliminer la race préexistante FOLLOWUP-01** qui pollue `go test -race` sur `./web/webdav/...` (race entre `pkg/config.UseViper` au setup test N et la goroutine `AntivirusTrigger` lancée par `stack.Start` au test N-1 — goroutines survivent entre tests).
2. **Widen `newWebdavTestEnv`** de `*testing.T` à `testing.TB` pour supporter les benchmarks (`*testing.B`) qui apparaîtront en Phase 5.
3. **Fournir 3 helpers de mesure** nécessaires aux tests LARGE (Phase 5) et CONC (Phase 7) : sampler mémoire concurrent, drain streaming SHA-256, générateur de fixture multi-GB sans binaires checked-in.

Ce phase est **infrastructural** — il ne livre aucune capacité user-facing. Son succès se mesure à la capacité des Phases 5-7 à écrire leurs tests sans réinventer l'instrumentation.

</domain>

<decisions>
## Implementation Decisions

### DEBT-01 — Race fix scope (Claude's discretion, with preference rule)

Décision principale : **commencer par le fix le plus petit qui rend `go test -race -count=1 ./web/webdav/...` propre, escalader uniquement si le petit fix casse d'autres tests.**

Ordre de préférence :

1. **Option A (préférée) : Gate l'enregistrement du `AntivirusTrigger` en mode test.** Ajouter un check `config.IsInTest()` ou env var `COZY_DISABLE_AV_TRIGGER` dans le chemin qui enregistre le trigger (probablement `model/stack/main.go` ou `model/job/mem_scheduler.go`). En test, pas de trigger → pas de goroutine survivante → pas de race. Petit patch localisé. N'aide pas les autres packages si eux aussi frappent la race, mais closera FOLLOWUP-01 pour notre scope.

2. **Option B (escalation) : Ajouter `stack.Shutdown()` / `scheduler.Stop()` dans `t.Cleanup()` de `testutils.NewSetup`.** Plus invasif (touche `testutils/test_utils.go`, `model/stack/main.go`, `model/job/mem_scheduler.go`), mais bénéfice à tous les packages de cozy-stack qui utilisent le harness. À adopter SI Option A se révèle insuffisante (race reste) ou casse autre chose.

3. **Option C (hors scope v1.2) : Protection RWMutex/atomic des globals `config.*`.** Racine propre mais touche `pkg/config` dans toute la stack. Si on en arrive là, ça devient un chantier séparé ("stack hardening") et on garde DEBT-01 en Option B ou en ajoutant une annotation `-atomic` guard locale à `FsURL()`.

La ré-évaluation se fait au planning de Phase 4 une fois que le plan a lu le code en détail. Si Option A s'avère trivial et propre, on reste là. Si elle demande des acrobaties, on passe à Option B.

**Critère de succès commun aux 3 options :** `go test -race -count=1 ./web/webdav/...` produit zéro `WARNING: DATA RACE`. Pas de régression de `go test ./...` à la racine.

### `testing.Short()` blanket skip (Claude's discretion, with preference)

Contexte : `web/webdav/testutil_test.go:35` a actuellement :
```go
if testing.Short() {
    t.Skip("webdav integration tests require a cozy test instance")
}
```
Ce skip s'applique à **tous** les tests qui construisent un `newWebdavTestEnv`. Combiné avec CI-03 (Phase 8) qui veut passer `-short` à `go-tests.yml`, cela signifierait que **tous** les tests WebDAV sautent en CI — pas seulement les heavy.

Décision : **Option A (préférée) : supprimer ce blanket skip.**

Raisons :
- `testutils.NeedCouchdb(t)` ligne 42 est déjà appelé juste après, et lui gère proprement le cas "pas de CouchDB disponible" (il skip si `COZY_COUCHDB_URL` absent).
- Le blanket `testing.Short()` skip est donc redondant avec `NeedCouchdb` pour sa raison d'origine.
- Une fois retiré, `-short` ne skip QUE les tests qui portent leur propre opt-in `if testing.Short() { t.Skip }` — comportement attendu par CI-03.
- Les callers locaux qui veulent "tests rapides sans CouchDB" peuvent continuer à utiliser `-short` — les tests integration sauteront via `NeedCouchdb` s'ils n'ont pas `COZY_COUCHDB_URL`.

**Critère de succès :** après Phase 4, `go test -short ./web/webdav/...` (avec `COZY_COUCHDB_URL` absent) skip les tests integration mais ne fail pas. `go test ./web/webdav/...` (avec `COZY_COUCHDB_URL` présent) les exécute tous.

### DEBT-02 — `newWebdavTestEnv` signature widening

Changer la signature de `*testing.T` vers `testing.TB` pour supporter `*testing.B`. Changement trivial, à faire en premier dans la Phase 4 parce que les autres helpers (INSTR-01/02/03) devraient déjà accepter `testing.TB` — sinon on change tout deux fois.

Ordre d'exécution imposé : **DEBT-02 avant INSTR-01/02/03**.

**Critère de succès :** `newWebdavTestEnv` accepte un `*testing.B` sans erreur de compilation. Tous les callers `*testing.T` existants continuent de compiler sans changement.

### Helper file organization (Claude's discretion, with preference)

Décision : **Option A (préférée) : un fichier `web/webdav/testhelpers_test.go` unique regroupant les 3 helpers INSTR.**

Raisons :
- 3 helpers, ~100-200 LOC combinés — tient largement dans un fichier.
- `testutil_test.go` reste centré sur `newWebdavTestEnv` et les seed functions (`seedFile`, `seedDir`, `seedFileInDir`) — séparation des rôles : "fixtures de setup de l'environnement" vs "outils de mesure et génération".
- Si les helpers grossissent significativement en Phase 7 (CONC) ou en v1.3, Phase 7 pourra les splitter en sous-fichiers sans friction — c'est une opération triviale.

### INSTR-01 — Memory sampler API (Claude's discretion, with preference)

Décision : **Option A (préférée) : closure wrapper + peak only.**

Signature proposée :
```go
// measurePeakHeap runs fn and concurrently samples runtime.MemStats.HeapInuse
// every 100ms. Returns the peak HeapInuse observed during fn's execution.
// The first sample is taken before fn starts; the last after fn returns.
func measurePeakHeap(tb testing.TB, fn func()) uint64
```

Raisons :
- Cas d'usage principal : `assert.Less(measurePeakHeap(t, putOp), uint64(128 << 20))` — une ligne, lisible.
- 100ms interval donne 10-100 samples sur un transfert 1 GB qui prend 2-10 secondes. Suffisant pour détecter un peak.
- Peak seul suffit pour les asserts de Phase 5. Si plus tard un test a besoin de la courbe complète pour debug, on peut ajouter un second helper `sampleHeapCurve()` sans casser `measurePeakHeap`.
- Aucune gestion Start/Stop côté appelant — rien à oublier.

**Implementation note pour le planner :** garder l'array de samples interne (pour debug on-demand), exposer uniquement `Peak()`. Si le test fail, logger tous les samples via `tb.Logf` dans le helper au moment du fail (via `tb.Helper()` + `tb.Cleanup` ou assertion-side callback).

### INSTR-02 — Drain helper (planner discretion within constraints)

Contraintes dures (research-based) :
- **JAMAIS** `io.ReadAll`, `httpexpect.Body().Raw()`, ni aucun buffer accumulant tout le body sur un gros transfert.
- Pattern : `io.TeeReader(body, sha256.New())` → `io.Copy(io.Discard, teeReader)`.
- Retourne au minimum le SHA-256 hex et le nombre de bytes lus.

Signature recommandée (à valider au planning) :
```go
// drainStreaming reads r fully, computes SHA-256 on the fly, returns
// (checksum_hex, n_bytes_read, err). Never allocates the full body as []byte.
func drainStreaming(r io.Reader) (string, int64, error)
```

### INSTR-03 — Fixture generator (planner discretion within constraints)

Contraintes dures :
- **Zéro fichier binaire checked-in** dans git. Les fixtures sont générées en mémoire, streamées.
- Déterminisme : pour une taille N donnée, deux appels produisent les mêmes bytes (pour reproductibilité des asserts de SHA-256 entre PUT et GET).

Signature recommandée (à valider au planning) :
```go
// largeFixture returns an io.Reader producing n deterministic pseudo-random bytes.
// The seed is hardcoded so repeated calls with the same n produce identical output —
// this lets us compare SHA-256 before PUT and after GET without persisting the bytes.
func largeFixture(n int64) io.Reader
```

Choix à faire au planning : seed fixe (plus simple) vs seed dérivé du nom du test (plus isolé). **Préférence : seed fixe `0x434F5A59` ("COZY")** — la taille fait déjà varier les bytes produits, et deux tests concurrents lisant la même fixture size auront le même stream mais c'est OK vu qu'ils comparent leur propre hash.

### Claude's Discretion

Tous les sous-choix d'implémentation pour DEBT-01 (choix final de l'option A/B selon l'investigation du code), la forme exacte des signatures INSTR-02/03, la granularité des samples INSTR-01 (modifiable après Phase 5 si ça se révèle insuffisant), et l'emplacement exact du patch DEBT-01 (gate au niveau registration trigger vs gate au niveau mem_scheduler startup).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Research pour Phase 4

- `.planning/research/STACK.md` — Libs et tooling pour tests robustness v1.2 ; confirme stdlib suffisant, pas de nouvelle dépendance Go modulaire.
- `.planning/research/FEATURES.md` — Décrit INSTR-01/02/03 et leurs contraintes dures (pas de `io.ReadAll`, TeeReader+sha256, deterministic fixture).
- `.planning/research/ARCHITECTURE.md` — Trace de `put.go:104` → `swiftFileCreationV3.Write` ; ordre de build recommandé (DEBT-01 → INSTR helpers → LARGE → CONC).
- `.planning/research/PITFALLS.md` — §1 (body-accumulating helpers), §2 (RSS post-hoc trap), §4 (concurrent test flakiness). La Phase 4 doit *produire* les helpers qui évitent §1.
- `.planning/research/SUMMARY.md` — Synthèse ; Phase 4 = "Prerequisites and Instrumentation", prérequis pour tout le reste.

### Race FOLLOWUP-01

- `.planning/phases/01-foundation/01-VALIDATION.md` — Décrit la race précisément avec stack trace complète (`pkg/config/config/config.go` + `model/job/trigger_antivirus.go:102` + `model/stack/main.go:104` + `tests/testutils/test_utils.go`). Le planner doit lire ça avant de toucher au fix.
- `.planning/phases/01-foundation/01-VERIFICATION.md` §*Known Caveat* — Disposition user-approved, provisional slot "01.1-race-harness".
- `.planning/STATE.md` §*Accumulated Context > Architecture Decisions (v1.1, still active)* — Noté que Phase 8 dépend de Phase 4 pour race-free baseline.

### Milestone et exigences

- `.planning/REQUIREMENTS.md` — DEBT-01, DEBT-02, INSTR-01, INSTR-02, INSTR-03 (définitions formelles).
- `.planning/ROADMAP.md` §*Phase 4* — Success criteria (5 items testables).
- `.planning/PROJECT.md` §*Active — v1.2 Robustness* §*Explicitly NOT in scope* — Rappel que v1.2 est correction, pas performance/load.

### Code à lire impérativement

- `web/webdav/testutil_test.go` — `newWebdavTestEnv` ligne 33 (signature à widen) et ligne 35 (blanket skip à retirer) et ligne 42 (`NeedCouchdb` qui reste la garde).
- `pkg/config/config/config.go` — Globals mutés par `UseViper` / `UseTestFile`.
- `model/job/trigger_antivirus.go` — Ligne ~102, la goroutine qui lit `FsURL()` sur timer.
- `model/job/mem_scheduler.go` — Ligne ~59, où le trigger est enregistré/lancé.
- `model/stack/main.go` — Ligne ~104, `stack.Start` qui lance le scheduler.
- `tests/testutils/test_utils.go` — `NewSetup`, `GetTestInstance`, `NeedCouchdb`.

### Standards Go pertinents

- `testing.TB` interface (pkg.go.dev) — superset de `*testing.T` et `*testing.B`.
- `runtime.ReadMemStats` (pkg.go.dev) — champ `HeapInuse` (non `HeapAlloc` — on veut l'arena committed).
- `go test -short` flag convention — idiomatique stdlib.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- **`testutils.NewSetup(t, name)`** — fabrique un `*TestSetup` avec CouchDB et un cleanup automatique. Déjà le point d'entrée de `newWebdavTestEnv`. Réutilisable directement si on `Widens` à `testing.TB` (à vérifier — si `NewSetup` prend `*testing.T`, il faudra aussi widen côté amont ou wrapper).
- **`testutils.NeedCouchdb(t)`** — skip si `COZY_COUCHDB_URL` absent. C'est la bonne garde, elle remplace fonctionnellement le blanket `testing.Short()` skip pour le cas "pas d'infra".
- **`testutils.CreateTestClient(t, ts.URL)`** — produit un `*httpexpect.Expect`. Réutilisé tel quel.
- **Echo `httptest.Server`** (via `setup.GetTestServer`) — `.CloseClientConnections()` disponible pour les tests d'interruption Phase 6 (utile à savoir même si on ne l'utilise pas en Phase 4).

### Established Patterns

- **Test setup minimal** — chaque test integration ouvre sa propre instance Cozy jetable via `NewSetup`. Pas de shared fixture entre tests. Conséquence : la goroutine `AntivirusTrigger` du test N pollue le test N+1 — c'est EXACTEMENT FOLLOWUP-01.
- **Test helpers dans `testutil_test.go`** — un seul fichier aujourd'hui, avec `newWebdavTestEnv` + seed helpers. Précédent faible (un seul fichier), donc pas de convention forte qui empêche d'en créer un deuxième.
- **`t.Cleanup`** — déjà utilisé pour `ts.Close()`. Pattern standard pour les tear-downs. Le fix DEBT-01 Option B utiliserait ce même pattern.

### Integration Points

- **`testutil_test.go` → tous les *_test.go** — toute modification de `newWebdavTestEnv` signature se propage à ~15 fichiers de tests. `testing.TB` au lieu de `*testing.T` est strictement ascendant (tout caller `*testing.T` passe), donc pas de breakage.
- **`model/stack/Start` ← `testutils.GetTestInstance`** — c'est ici que le scheduler + AntivirusTrigger sont lancés. C'est le point d'injection du fix DEBT-01 Option A (gate du trigger) ou B (Shutdown dans Cleanup).
- **`webdav-heavy.yml` futur (Phase 8) ← tests LARGE Phase 5** — les tests LARGE qu'on prépare consommeront les helpers INSTR de Phase 4. L'API des helpers doit être assez ergonomique pour que les tests LARGE ne réinventent rien.

</code_context>

<specifics>
## Specific Ideas

- Pattern de cleanup user-référencé implicitement lors de la décision "Option A minimale" sur DEBT-01 : rester pragmatique, ne pas partir sur un refactoring ambitieux de `pkg/config` au milieu d'un milestone WebDAV. Si le fix minimal suffit, on ship ; sinon on escalade par petits incréments.
- Convention `testing.Short()` est idiomatique Go stdlib — aucune autre mécanique de gating ne devrait être introduite (pas d'env var `COZY_SKIP_WEBDAV_TESTS` ou équivalent).
- Seed fixture `0x434F5A59` ("COZY" en ASCII hex) est une préférence esthétique, pas une contrainte technique. N'importe quel seed constant suffirait.

</specifics>

<deferred>
## Deferred Ideas

- **Protection RWMutex/atomic sur `pkg/config` globals (DEBT-01 Option C)** — pourrait être son propre milestone "stack hardening" séparé, hors scope WebDAV. Ne pas attaquer en v1.2.
- **Helper sampleHeapCurve() avec historique complet des samples** — si un test LARGE fail et qu'on a besoin de la courbe mémoire pour debug. On attend Phase 5 pour décider si c'est nécessaire. Pour l'instant, `measurePeakHeap` + log interne sur fail suffit.
- **Seed dérivé de `t.Name()` pour les fixtures** — pourrait isoler les fixtures entre tests, mais ajoute de la complexité. À reconsidérer si on observe des tests concurrents qui se marchent dessus.
- **Helper `drainCounted` séparé de `drainStreaming`** (sans SHA-256, juste byte count) — pas besoin pour nos cas actuels. On verra si Phase 5 ou 7 en demande un.

</deferred>

---

*Phase: 04-prerequisites-and-instrumentation*
*Context gathered: 2026-04-13*
