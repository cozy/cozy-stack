# Plan 03-06: Litmus copymove Suite — First-Run Inventory

**Date:** 2026-04-12T13:30 UTC
**cozy-stack commit:** b49515ab9 (docs(03-07): complete litmus props suite plan — SUMMARY, STATE, ROADMAP)
**litmus version:** 0.13
**Branch:** feat/webdav

---

## Route 1: /dav/files/

**Result:** 12 passed, 1 failed (92.3%)

### Pass/Fail Per Test

| # | Test name        | Result |
|---|-----------------|--------|
| 0 | init            | PASS   |
| 1 | begin           | PASS   |
| 2 | copy_init       | PASS   |
| 3 | copy_simple     | PASS   |
| 4 | copy_overwrite  | FAIL   |
| 5 | copy_nodestcoll | PASS   |
| 6 | copy_cleanup    | PASS   |
| 7 | copy_coll       | PASS   |
| 8 | copy_shallow    | PASS   |
| 9 | move            | PASS   |
|10 | move_coll       | PASS   |
|11 | move_cleanup    | PASS   |
|12 | finish          | PASS   |

### Failure Detail

**copy_overwrite** — `COPY overwrites collection: 405 Method Not Allowed`

---

## Route 2: /remote.php/webdav/

**Result:** 12 passed, 1 failed (92.3%)

### Pass/Fail Per Test

| # | Test name        | Result |
|---|-----------------|--------|
| 0 | init            | PASS   |
| 1 | begin           | PASS   |
| 2 | copy_init       | PASS   |
| 3 | copy_simple     | PASS   |
| 4 | copy_overwrite  | FAIL   |
| 5 | copy_nodestcoll | PASS   |
| 6 | copy_cleanup    | PASS   |
| 7 | copy_coll       | PASS   |
| 8 | copy_shallow    | PASS   |
| 9 | move            | PASS   |
|10 | move_coll       | PASS   |
|11 | move_cleanup    | PASS   |
|12 | finish          | PASS   |

### Failure Detail

**copy_overwrite** — `COPY overwrites collection: 405 Method Not Allowed`

---

## Failure Analysis

### copy_overwrite

**What the test does:**
The litmus `copy_overwrite` test sends a COPY request from a source **file** to a destination path that already contains a **collection** (directory). With `Overwrite: T` (or absent header), RFC 4918 §9.8.4 requires the server to replace the destination regardless of its type (file or collection) and return 204.

**Contract mapping:** COPY-02 (overwrite semantics, RFC 4918 §10.6)

**Root-cause (copy.go):**

`handleCopy` (file branch, step 9) only checks `_, dstFile, err := fs.DirOrFileByPath(dstPath)` and ignores the `dstDir` return value. When the destination is a collection:
- `dstFile` is `nil`
- `dstDir` is non-nil
- The overwrite/trash logic is skipped entirely
- `fs.CopyFile(srcFile, newdoc)` is called with the directory still present at the same path
- VFS returns `os.ErrExist`
- `mapVFSWriteError` maps `os.ErrExist` → 405 Method Not Allowed

**Fix location:** `web/webdav/copy.go`, `handleCopy`, step 9 (lines ~105-119)

**Fix:** Extend the overwrite check to also handle the case where `dstDir != nil`:
```go
dstDir, dstFile, err := fs.DirOrFileByPath(dstPath)
if err != nil && !errors.Is(err, os.ErrNotExist) {
    return mapVFSWriteError(c, err)
}
if dstFile != nil || dstDir != nil {
    if !overwrite {
        return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
    }
    destExisted = true
    if dstFile != nil {
        if _, err = vfs.TrashFile(fs, dstFile); err != nil {
            return mapVFSWriteError(c, err)
        }
    } else {
        if _, err = vfs.TrashDir(fs, dstDir); err != nil {
            return mapVFSWriteError(c, err)
        }
    }
}
```

---

## Final State

| Failure | Status |
|---------|--------|
| copy_overwrite (both routes) | FIXED — see plan 03-06 RED+GREEN commits |
