# Phase 5: Large-File Streaming Proof - Context

**Gathered:** 2026-04-14
**Status:** Ready for planning

<domain>
## Phase Boundary

Prouver — de façon mesurée, reproductible, capturée dans la suite de tests — que le serveur WebDAV streame les gros fichiers sans accumuler le corps en mémoire. Deux tests end-to-end, via `gowebdav`, avec assertion dure sur le peak `HeapInuse` serveur :

1. `TestPut_LargeFile_Streaming` — PUT de 1 GiB, peak heap < 128 MB.
2. `TestGet_LargeFile` — GET de 1 GiB, SHA-256 du body téléchargé correspond à l'upload, peak heap < 128 MB.

**Hors scope (non-goal roadmap):** débit, latence, comparaison avec d'autres serveurs. Les chiffres MB/s sont informatifs — aucun seuil pass/fail sur la vitesse.

</domain>

<decisions>
## Implementation Decisions

### Test file organization

**Décision : Option A — un seul fichier `web/webdav/large_test.go`.**

Les deux tests (`TestPut_LargeFile_Streaming` + `TestGet_LargeFile`) vivent dans `web/webdav/large_test.go`. Symétrique avec `testhelpers_test.go` créé en Phase 4 — un fichier "thème" (LARGE, heavy) plutôt que "opération" (PUT, GET). Signal visuel clair que c'est la suite lourde.

### CI short-mode guard (anticipation Phase 8 CI-03)

**Décision : Option A — ajouter `if testing.Short() { t.Skip(...) }` dès maintenant en Phase 5.**

Chaque test LARGE commence par :
```go
if testing.Short() {
    t.Skip("LARGE test: skipped in -short mode")
}
```

Raison : Phase 8 CI-03 exige explicitement que tous les tests lourds s'auto-skippent en `-short`. L'ajouter en Phase 5 évite à Phase 8 de revenir toucher ces fichiers. Réduit le diff et les risques de conflits.

### Fixture size

**Décision : Option A — 1 GiB constant, pas d'override env var.**

```go
const largeFileSize = 1 << 30 // 1 GiB
```

Reproductibilité > commodité d'itération. Le test prouve exactement ce que le spec dit. Pour une itération locale rapide, le dev utilise `-run TestOther` ou `-short`. Pas de knob `COZY_WEBDAV_LARGE_SIZE` à maintenir.

### GET test setup path

**Décision : Option A — PUT via HTTP en setup, puis GET mesuré.**

Structure du test :
```go
func TestGet_LargeFile(t *testing.T) {
    if testing.Short() { t.Skip(...) }
    env := newWebdavTestEnv(t, nil)
    // Setup: PUT 1 GiB via gowebdav, no heap assertion here
    putLargeFixture(t, env, "/large.bin")
    // Measured: GET 1 GiB, assert heap + SHA-256
    peak := measurePeakHeap(t, func() {
        r := gowebdavClient.ReadStream("/large.bin")
        sum, n, err := drainStreaming(r)
        require.NoError(t, err)
        require.Equal(t, int64(1<<30), n)
        require.Equal(t, expectedSum, sum)
    })
    require.Less(t, peak, uint64(128<<20))
}
```

Raisons :
- Réalisme maximum : full HTTP-in-then-HTTP-out, même chemin qu'un client réel.
- Pas de plomberie VFS directe — les deux tests utilisent exactement la même API.
- Le SHA-256 attendu se calcule une fois à partir de `largeFixture(1<<30)` (seed déterministe `0x434F5A59` de Phase 4), donc pas de recalcul onéreux.
- Coût : ~60s par test GET au lieu de ~30s. Acceptable vu le scope "heavy suite".

### Heap assertion shape

**Décision : Option A — strict hard fail à 128 MB, avec `runtime.GC()` avant mesure, one-shot + log de tous les samples sur échec.**

Trois sous-décisions :

1. **Seuil** : `require.Less(peak, uint64(128<<20))`. Pas de tolérance. Si ça flake en CI, c'est un vrai signal — on élargirait le seuil intentionnellement via un ADR/commit documenté, pas via une bande de tolérance silencieuse.

2. **Baseline GC** : appeler `runtime.GC()` juste avant l'exécution de `fn()` dans `measurePeakHeap`. Sans ça, le peak du test N peut être gonflé par le garbage non-collecté du test N-1. Le spec de `measurePeakHeap` existant en Phase 4 permet déjà "le premier sample est pris avant `fn`" — on ajoute `runtime.GC()` juste avant ce premier sample.

3. **Sur échec** : pas de retry. `measurePeakHeap` logge la courbe complète des samples via `tb.Logf` sur échec (ce comportement est déjà prévu dans la décision Phase 4 pour `measurePeakHeap`). Un retry boucle cacherait les fuites mémoire intermittentes.

**Implication pour le planner** : étendre `measurePeakHeap` pour appeler `runtime.GC()` avant le premier sample. Petit patch sur `testhelpers_test.go` (~2 lignes). Compatible ascendant avec les tests existants de Phase 4 (ils deviennent juste un poil plus déterministes).

### Benchmarks

**Décision : Option A — pas de `Benchmark*`, logger MB/s directement dans `Test*` via `t.Logf`.**

À l'intérieur de chaque `Test*`, après la mesure :
```go
mbps := float64(largeFileSize) / (1 << 20) / duration.Seconds()
t.Logf("LARGE transfer: %.1f MB/s (peak heap %s)", mbps, humanize(peak))
```

Raisons :
- Roadmap dit explicitement "informative only, no pass/fail on speed".
- Zéro machinerie en plus : pas de `BenchmarkXxx`, pas de `b.SetBytes`, pas de 60s de runtime supplémentaire.
- Les chiffres sont visibles dans `go test -v` — suffisant pour détecter une régression perf éventuelle à l'œil.

### Claude's Discretion

- Nom exact du helper de setup PUT (p.ex. `putLargeFixture`, `seedLargeFile`) — choix stylistique.
- Format exact du log MB/s (décimales, unités).
- Mécanique de calcul du SHA-256 attendu : précalculé en `TestMain`/`init()`, ou calculé à chaque test via `drainStreaming(largeFixture(...))`. Préférence légère pour précalcul si deux tests partagent la même constante, mais non-bloquant.
- Gestion de `gowebdav` client : instanciation par test vs helper partagé. Pas d'enjeu fort.
- Timeout HTTP côté client pour 1 GiB sur loopback — valeur raisonnable au jugement du planner (p.ex. 5min).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 4 artifacts (prérequis directs)

- `.planning/phases/04-prerequisites-and-instrumentation/04-CONTEXT.md` — Décisions sur `measurePeakHeap`, `drainStreaming`, `largeFixture` (seed `0x434F5A59`), organisation `testhelpers_test.go`.
- `.planning/phases/04-prerequisites-and-instrumentation/04-SUMMARY.md` — État final Phase 4 : race fermée, `newWebdavTestEnv` accepte `testing.TB`.
- `.planning/phases/04-prerequisites-and-instrumentation/04-03-SUMMARY.md` — Signatures exactes des 3 helpers tels que livrés.
- `.planning/phases/04-prerequisites-and-instrumentation/04-VERIFICATION.md` — Must-haves verified, helpers prêts à l'emploi.
- `web/webdav/testhelpers_test.go` — Code source des helpers INSTR (à lire pour connaître signatures et comportement exacts).

### Research v1.2

- `.planning/research/FEATURES.md` — Définit LARGE-01/02 et les contraintes dures (pas de `io.ReadAll`, pas de `httpexpect.Body().Raw()`, SHA-256 via `TeeReader`).
- `.planning/research/PITFALLS.md` §1 — Body-accumulating helpers interdits. §2 — Piège "RSS post-hoc" : mesurer pendant le transfert, pas après.
- `.planning/research/ARCHITECTURE.md` — Trace de `put.go:104` → `swiftFileCreationV3.Write`. Confirme que le chemin PUT WebDAV est bien streaming côté serveur aujourd'hui.
- `.planning/research/STACK.md` — `gowebdav` est déjà une dépendance (déjà utilisée dans les tests Phase 1-3). `runtime.ReadMemStats.HeapInuse` est le bon champ (pas `HeapAlloc`).

### Milestone et exigences

- `.planning/REQUIREMENTS.md` §LARGE — Définitions formelles LARGE-01 (PUT 1 GB peak < 128 MB) et LARGE-02 (GET 1 GB peak < 128 MB, SHA-256 matched).
- `.planning/ROADMAP.md` §Phase 5 — 4 success criteria (les deux tests passent, pas de `io.ReadAll`, pas de fixture binaire).
- `.planning/PROJECT.md` §Active — v1.2 est correctness, pas performance. Rappel du non-goal "throughput/latency informative only".

### Code existant à lire

- `web/webdav/testutil_test.go` — `newWebdavTestEnv` (point d'entrée des tests integration).
- `web/webdav/testhelpers_test.go` — les 3 helpers INSTR livrés en Phase 4 (à étendre pour `runtime.GC()` avant `fn`).
- `web/webdav/put_test.go` — conventions existantes pour tests PUT (setup, gowebdav client, assertions).
- `web/webdav/get_test.go` — idem pour GET.
- `web/webdav/put.go` ligne ~104 — handler PUT WebDAV, chemin à prouver streaming.
- `web/webdav/get.go` — handler GET WebDAV, chemin à prouver streaming.

### Phase 8 anticipée

- `.planning/ROADMAP.md` §Phase 8 CI-03 — Critère qui justifie l'ajout du `testing.Short()` guard dès Phase 5.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (livrés Phase 4)

- **`measurePeakHeap(tb testing.TB, fn func()) uint64`** — retourne le peak `HeapInuse` observé pendant `fn()`. Ticker 100ms, samples internes, log complet via `tb.Logf` sur échec. **À étendre en Phase 5 : ajouter `runtime.GC()` avant le premier sample.**
- **`drainStreaming(r io.Reader) (string, int64, error)`** — consomme un `io.Reader` avec `TeeReader` + `sha256`, retourne `(hex, n, err)`. Jamais `io.ReadAll`. Utilisé tel quel pour `TestGet_LargeFile`.
- **`largeFixture(n int64) io.Reader`** — seed `0x434F5A59`, retourne un `io.Reader` déterministe de N bytes, zéro fichier binaire. Utilisé tel quel pour générer le body PUT et le SHA-256 attendu.
- **`newWebdavTestEnv(tb testing.TB, overrideRoutes func(*echo.Group))`** — accepte maintenant `testing.TB` (Phase 4 DEBT-02). Les benchmarks pourraient être ajoutés plus tard sans refactor de signature. Env var `COZY_DISABLE_AV_TRIGGER` est déjà `Setenv` dedans — pas de race pendant les tests LARGE.

### Established Patterns

- **Client `gowebdav`** — déjà utilisé dans `put_test.go`/`get_test.go` existants. Instancié avec base URL = test server URL. Réutiliser le même pattern d'init.
- **Assertions via `require` (testify)** — convention du package. `require.NoError`, `require.Equal`, `require.Less`.
- **Un test = une instance Cozy** — via `newWebdavTestEnv`. Pas de shared fixture. Chaque LARGE test setup + teardown sa propre instance, ~1s d'overhead acceptable sur un test qui dure 30-60s.

### Integration Points

- **`web/webdav/put.go` handler** — point où le body PUT est lu. Doit être streaming (déjà confirmé par `.planning/research/ARCHITECTURE.md`). Le test prouve que ça reste streaming.
- **`web/webdav/get.go` handler** — point où le body GET est écrit. Même contrainte.
- **Phase 8 CI workflow `webdav-heavy.yml`** (futur) — va exécuter ces tests sans `-short`. La machine CI doit avoir au moins 2 GB RAM. Pas une décision Phase 5 mais bon à savoir.

</code_context>

<specifics>
## Specific Ideas

- Le log MB/s dans `Test*` n'est pas une simple courtoisie — c'est le signal que le planner et le reviewer utiliseront pour spotter une régression perf accidentelle sans qu'il y ait un gate CI. Format lisible (`%.1f MB/s`) préférable à du raw.
- Le choix "one-shot + log samples on fail" est cohérent avec la philosophie v1.2 : "we measure reality, we don't tune the measurement until it passes". Si un test échoue, on veut le matériau brut pour enquêter.
- Le PUT-en-setup du test GET ne doit PAS asserter sur le heap — ce serait un test caché. Le commentaire dans le code doit l'expliciter : "Setup: this PUT is not the test — TestPut_LargeFile_Streaming covers the PUT path."

</specifics>

<deferred>
## Deferred Ideas

- **Benchmarks `BenchmarkPut_LargeFile` / `BenchmarkGet_LargeFile` séparés** — si en v1.3 on veut faire du tracking perf CI. Pour v1.2, le log MB/s suffit.
- **Env var `COZY_WEBDAV_LARGE_SIZE` pour override local** — si les devs se plaignent du temps d'itération. Pour l'instant, `-short` suffit comme échappatoire.
- **Seed dérivé de `t.Name()`** — hérité de Phase 4 deferred. Seed fixe reste OK.
- **Helper `sampleHeapCurve()`** exposant tous les samples — hérité de Phase 4 deferred. Si Phase 5 fait émerger un besoin, on le crée alors.
- **Tests multi-GB (2 GB, 5 GB)** — hors scope Phase 5. Si un besoin de stress test apparaît, phase/milestone séparé.

</deferred>

---

*Phase: 05-large-file-streaming-proof*
*Context gathered: 2026-04-14*
