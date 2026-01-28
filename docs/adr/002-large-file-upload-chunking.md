# ADR: Large File Upload Support with Storage-Level Chunking

## Status

Proposed

## Context

Twake Workplace (Cozy-Stack) allows users to upload files via a single HTTP request that are stored in OpenStack Swift or local filesystem (Afero). 
Currently, there are practical limitations:

1. Swift has a ~5GB single object limit
2. Very large file uploads can stress server resources
3. Long uploads are more prone to network failures
4. We plan to add S3 API support, which has similar chunking needs (multipart uploads)

### Current Architecture

The VFS (Virtual File System) abstraction layer supports multiple backends:
- **VFSSwift** (`model/vfs/vfsswift/impl_v3.go`): OpenStack Swift storage
- **VFSAfero** (`model/vfs/vfsafero/impl.go`): Local filesystem storage

Both implement the same `vfs.VFS` interface, keeping the storage implementation transparent to consumers.

### Problem Statement

Users cannot upload files larger than 5GB to Swift storage. We need a solution that:
- Supports files larger than 5GB
- Works with existing single-request upload API
- Is extensible to local VFS and future S3 support
- Minimizes changes to CouchDB schema and existing code

## Proposal

### Storage-Level Chunking

Implement chunking at the storage backend level, keeping it transparent to the HTTP API layer. Each storage backend handles chunking internally using its native large object support:

**For Swift:** Use Static Large Objects (SLO)
- Swift automatically manages segments and manifests
- Downloads are transparently reassembled by Swift
- No CouchDB schema changes needed

**For Local VFS (Afero):** Already handles large files natively (no chunking needed)

**For Future S3:** Use S3 Multipart Upload API
- S3/MinIO provide native multipart upload support similar to Swift SLO (streaming parts directly to disk without buffering entire files)
- Same transparent approach can be used
- Observe S3 protocol limits: a single object maxes out at 5TB, uploads can have at most 10,000 parts, and each part must be between 5MB and 5GB (last part can be smaller)
- Pick part sizes small enough (and configurable) so the 10,000-part limit still covers the largest supported file; anything larger than 5TB must be chunked at the application level because the S3 API itself forbids it
- **Note on checksums:** Like Swift SLO, S3 multipart ETags are not MD5 hashes of the content (they're a hash of part ETags). Application-side MD5 computation will be required for S3 multipart uploads, using the same pattern as Swift SLO

### Implementation Details

#### 1. Configuration (Swift-Specific)

Add Swift-specific configuration under `fs.swift`:

```yaml
fs:
  url: swift://...
  swift:
    # Size of each segment for SLO uploads (default: 4GB)
    segment_size: 4294967296
    # Files larger than this use SLO (default: same as segment_size)
    # Set to 0 to always use SLO
    slo_threshold: 4294967296
```

**Test override**: Tests will be able to set a tiny segment size (e.g., 1KB) via config overrides to exercise the SLO code path without uploading gigabytes.

#### 2. Swift VFS Changes (`model/vfs/vfsswift/impl_v3.go`)

**CreateFile method modifications:**

The `CreateFile` method will be extended to:
1. Read segment size and SLO threshold from configuration
2. Determine whether to use SLO based on file size (files larger than threshold) or streaming mode (unknown size, indicated by negative `ByteSize`)
3. For SLO uploads: use Swift's `StaticLargeObjectCreateFile` API with configured chunk size, letting Swift generate collision-free segment prefixes automatically
4. Return a new `swiftLargeFileCreationV3` struct that wraps the SLO writer and maintains its own MD5 hasher
5. For regular uploads: continue using the existing `ObjectCreate` path unchanged

**Quota enforcement for streaming uploads (unknown size):**

When `ByteSize < 0` (streaming/chunked transfer encoding), the total size is unknown upfront. Quota enforcement will work as follows:
1. Before upload: check that instance has *some* available quota (reject if quota already exceeded)
2. During upload: the `swiftLargeFileCreationV3.Write()` method will track cumulative bytes written
3. On each write: compare cumulative bytes against instance quota; if exceeded, call `Abort()` to clean up segments and return a quota-exceeded error
4. The existing `vfs.CheckAvailableDiskSpace` check runs at file creation time; for streaming uploads, we add a runtime check that aborts mid-stream if the limit is hit
5. This mirrors the existing behavior for regular uploads where Swift/the VFS layer rejects writes that exceed quota

#### 3. MD5/Checksum Handling

**Important**: SLO manifests don't return a single MD5 hash like regular objects. The manifest's ETag is a hash of the segment ETags, not the content.

We will implement application-side MD5 computation:

1. Create a new `swiftLargeFileCreationV3` struct that holds an MD5 hasher alongside the Swift file writer
2. On each `Write()` call, update the MD5 hash before passing data to Swift
3. On `Close()`, finalize the MD5 hash and store it in `newdoc.MD5Sum` (ignoring Swift's manifest ETag)
4. Proceed with the normal close logic (CouchDB update, versioning) using our computed hash

This ensures:
- Antivirus scanning works (relies on MD5Sum)
- File versioning works (compares MD5Sum to detect changes)
- File integrity validation works

#### 4. Failure Handling and Cleanup

**Failure Scenarios:**

1. **Client disconnects mid-upload**: The `swiftLargeFileCreationV3.Close()` is never called; segments remain orphaned
2. **Server crash**: Same as above - partially written segments exist without a manifest
3. **Write error mid-stream**: Error returned from `Write()` or `Close()`, segments may exist

**Cleanup Strategy:**

**Best-effort cleanup on error:**

The `swiftLargeFileCreationV3` struct will implement cleanup behavior:
- On `Close()` error: attempt to delete any written segments using `LargeObjectDelete`, then return the original error
- New `Abort()` method: close the underlying writer and delete any segments that were written; called on context cancellation or explicit abort

**Periodic garbage collection of orphaned segments:**

Orphaned segments can accumulate from crashes or network failures. We will add a worker job (`worker/gc/slo_segments.go`) that:
1. Lists all segment prefixes (objects matching `*_segments/*` pattern)
2. For each segment prefix, checks if the parent manifest exists
3. If the manifest is missing AND segments are older than a configurable threshold (default: 24h), deletes the orphaned segments
4. Logs all deletions for audit trail

**Configuration:**
```yaml
fs:
  swift:
    # ... existing config ...
    # Age threshold for orphan cleanup (default: 24h)
    orphan_segment_max_age: 24h
```

**Triggering cleanup:**
- On upload error: immediate best-effort delete
- On server startup: schedule GC job
- Periodically: run GC worker (configurable interval)
- Manual: `cozy-stack swift gc-segments` CLI command

#### 5. Operations That Need SLO Awareness

The following methods currently use `ObjectDelete` or `ObjectCopy` and need updates:

| Method | Current | Change Needed |
|--------|---------|---------------|
| `destroyFileLocked` | `ObjectDelete` | Use `LargeObjectDelete` with fallback |
| `cleanOldVersion` | `ObjectDelete` | Use `LargeObjectDelete` with fallback |
| `EnsureErased` | `BulkDelete` / `ObjectDelete` | Use `LargeObjectDelete` for each |
| `CopyFile` | `ObjectCopy` | Copy manifest + segments or re-upload |
| `DissociateFile` | `ObjectCopy` + `ObjectDelete` | Handle SLO copy and delete |
| `CopyFileFromOtherFS` | `ObjectCopy` | Handle SLO source objects |

**Deletion pattern:**

We will implement a `deleteObject` helper method that:
1. First attempts `LargeObjectDelete` (which handles both SLO manifests with their segments and regular objects)
2. If Swift returns `NotLargeObject` error, falls back to regular `ObjectDelete`
3. This unified approach ensures both SLO and regular objects are deleted correctly without needing to check the object type first

**Copy pattern** (for SLO objects):

Swift doesn't support copying SLO manifests directly. Available options:
1. **Copy manifest content and update segment references** - Complex: requires parsing manifest JSON, copying each segment individually, updating references
2. **Download and re-upload** - Simple but slow: streams entire file through the server
3. **Copy segments individually then create new manifest** - Medium complexity: server-side segment copy + new manifest creation

**Chosen approach: Segment copy with new manifest (Option 3)**

Rationale:
- Avoids streaming large files through the server (unlike Option 2)
- Keeps data server-side within Swift (efficient for same-region copies)
- Acceptable complexity since copy/dissociate of very large files is rare

Recommendation: For `CopyFile`/`DissociateFile`, detect if source is SLO and handle appropriately. For initial implementation, fall back to download/re-upload for SLO objects (rare case for very large files).

### How Swift SLO Works

```
Upload 10GB file with 4GB segments:
├── {container}/{objName}_segments/1234567890.123456/00000000  (4GB)
├── {container}/{objName}_segments/1234567890.123456/00000001  (4GB)
├── {container}/{objName}_segments/1234567890.123456/00000002  (2GB)
└── {container}/{objName}  (manifest JSON listing segments)

The segment prefix includes a timestamp to avoid collisions.

Download:
Client requests {objName} → Swift reads manifest → Streams segments in order
```

## Alternatives

### Alternative A: UI-Initiated Chunked Uploads

**Description:** Client sends multiple HTTP requests, each containing a chunk of the file. Server assembles chunks after all are received.

**Pros:**
- Client can resume uploads after failure
- Better progress tracking per chunk
- Works with any storage backend uniformly

**Cons:**
- Requires new HTTP API endpoints (`POST /files/chunks/start`, `PUT /files/chunks/{id}`, `POST /files/chunks/{id}/complete`)
- Significant UI/client changes required
- Server must track upload sessions and handle cleanup of incomplete uploads
- Adds complexity to CouchDB (need to track chunk metadata)
- More HTTP round trips
- State management for partial uploads

**Implementation complexity:** High

### Alternative B: Unified Chunking Layer in VFS

**Description:** Add a chunking abstraction in the VFS interface that all backends implement, with chunk metadata stored in CouchDB.

**Pros:**
- Consistent behavior across all storage backends
- Full control over chunk management
- CouchDB knows about file structure

**Cons:**
- CouchDB schema changes required (new `chunks` field in FileDoc)
- Must implement custom chunk assembly for downloads
- Duplicates functionality that Swift/S3 provide natively
- Increases complexity in all VFS operations
- Migration needed for existing files

**Implementation complexity:** High

## Decision

**Recommended approach: Storage-Level Chunking**

Rationale for recommendation:
1. **Minimal changes**: Only affects Swift VFS, no API or CouchDB changes
2. **Uses native features**: Swift SLO is battle-tested and efficient
3. **Transparent**: Existing code (downloads, file operations) works unchanged
4. **Extensible**: Same pattern applies to S3 multipart uploads
5. **No UI changes**: Works with existing single-request upload API
6. **Afero compatibility**: Local VFS already handles large files, no changes needed

## Consequences

### Positive
- Files larger than 5GB can be uploaded to Swift
- Downloads work transparently (Swift handles segment assembly)
- No CouchDB schema changes
- No HTTP API changes
- Easy to extend to S3 when needed
- Minimal code surface area to maintain
- Works with streaming uploads (unknown Content-Length)

### Negative
- Swift-specific implementation (though same pattern works for S3)
- MD5 must be computed application-side for SLO uploads
- Copy operations for SLO files are more complex
- Segments use additional storage namespace (though transparent to users)

### Neutral
- Configuration options are Swift-specific (`fs.swift.segment_size`)
- Deletion is slightly more complex (but library handles it)

### Security Considerations
- Switching to Swift SLO does not introduce new HTTP endpoints or long-lived upload sessions, so the DoS surface stays effectively the same as today's single-request uploads.
- Segment creation remains gated by the existing per-instance quota checks in `vfs.CheckAvailableDiskSpace`, so a malicious client cannot exceed its quota by partially uploading SLO files.
- **Streaming upload quota enforcement:** For uploads with unknown size (`ByteSize < 0`), the `swiftLargeFileCreationV3` writer tracks cumulative bytes and aborts the upload if quota is exceeded mid-stream. This prevents quota bypass via chunked transfer encoding.
- Large uploads remain inherently expensive (bandwidth/CPU). Existing protections—rate limiting (`pkg/limits`), request timeouts, and monitoring for long-running uploads—should continue to be enforced. No additional attack vectors are introduced by SLO itself.

## Testing Considerations

### Unit Tests
- Override `config.Fs.Swift.SegmentSize` to 1KB in tests to exercise SLO code path
- Test files smaller than threshold (regular upload)
- Test files larger than threshold (SLO upload)
- Test streaming upload with unknown size (should use SLO)
- Test MD5 computation matches expected value

### Integration Tests
- Test file deletion (verify segments are cleaned up)
- Test downloads of SLO files
- Test file versioning with SLO files
- Test copy/dissociate operations with SLO files

### Test Configuration Example

Tests will configure tiny segment sizes (e.g., 1KB segments, 512-byte threshold) to exercise the SLO code path with small test files. 
For example, a 2KB test file would use 2 segments, allowing verification of upload, download, and delete operations without requiring gigabytes of test data.
