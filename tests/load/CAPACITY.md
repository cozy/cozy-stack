# Cozy Stack Capacity Model

This document defines the operational capacity limits and pass criteria for Cozy Stack as part of [Epic #4864 (Load Testing Suite)](https://github.com/linagora/cozy-stack/issues/4864), [Issue #4866 (Organization / B2B scenarios)](https://github.com/linagora/cozy-stack/issues/4866), [Issue #4867 (Load-test shared drives)](https://github.com/linagora/cozy-stack/issues/4867), and [Issue #4863 (Indexing sharing groups and members)](https://github.com/linagora/cozy-stack/issues/4863).

It serves as the specification for automated capacity exploration in `tests/load` and defines how the test harness measures system boundaries under stress.

---

## Capacity Dimensions Overview

| Dimension / Limit | Unit | Primary Metric Axis | Test Step Sizes | Pass Criteria Summary |
| :--- | :--- | :--- | :--- | :--- |
| **1. Members per Organization** | `members` | Organization directory & group membership | 1k, 10k, 25k, 50k | P95 < 200ms, Error < 0.1%, RabbitMQ drain < 60s |
| **2. Shares per Instance** | `shares` | Shared drive discovery & permission resolution | 100, 1k, 5k, 10k, 25k | P95 < 250ms, Error < 0.1%, Index refresh < 5s |
| **3. Files per Instance** | `files` | Directory traversal & CouchDB Mango / view indexing | 10k, 100k, 500k, 1M | P95 < 150ms browse, Error < 0.05%, Index lag < 10s |
| **4. Concurrent Requests** | `concurrent requests` / `VUs` | Sustained throughput & connection concurrency | 10, 25, 50, 100, 250, 500 | P95 < 300ms read / 500ms write, Error < 0.1%, Drain < 5s |

---

## 1. Members per Organization

### Question Answered
*What is the maximum number of member accounts a single organization can contain before group resolution, directory lookups, and event-driven membership synchronization degrade below acceptable service thresholds?*

- **Unit of result:** `members` (active user accounts registered within the organization).

### Dimensions
- **Varying dimension:**
  - Total organization member count: **1,000**, **10,000**, **25,000**, **50,000** members (as identified in #4866).
- **Fixed dimensions:**
  - 1 organization instance.
  - Group density: 50 organizational groups, with each member belonging to an average of 3 groups.
  - Background API traffic: steady light query rate (10 req/s reading member profiles and directory listings).
  - Resource allocation: fixed container CPU and RAM limits on cozy-stack and CouchDB.

### Pass Criteria
- **P95 Latency:**
  - Directory query (`GET /organizations/:id/members` or member search): **P95 < 200 ms**.
  - Group membership resolution (`GET /groups/:id/members`): **P95 < 250 ms**.
- **Error Rate:** **< 0.1%** non-2xx responses under concurrent queries.
- **Queue Drain Time:**
  - RabbitMQ user/group synchronization backlog must drain at **>= 500 events/second**.
  - Total queue drain time for a batch of 1,000 membership mutation events must complete in **< 60 seconds** without unbounded queue accumulation.

---

## 2. Shares per Instance

### Question Answered
*What is the maximum number of active shared drives and permission references an instance can host before permission resolution, changes feeds, and drive listings exceed latency limits?*

- **Unit of result:** `shares` (number of active shared drives or share records on the instance).

### Dimensions
- **Varying dimension:**
  - Active shares on instance: **100**, **1,000**, **5,000**, **10,000**, and **25,000** shares.
- **Fixed dimensions:**
  - 1 Cozy instance.
  - File density per share: 100 files per drive.
  - Member recipients per share: 5 recipients (mix of direct users and group references).
  - Workload: 10 concurrent virtual users querying drive listings, checking file permissions, and polling changes feeds.
  - Indexing: Mango / CouchDB views enabled as targeted by #4863.

### Pass Criteria
- **P95 Latency:**
  - Drive discovery (`GET /sharings/`): **P95 < 250 ms**.
  - File access permission check within shared folder: **P95 < 50 ms**.
  - Changes feed query (`GET /changes` with filter): **P95 < 300 ms**.
- **Error Rate:** **< 0.1%** failures or HTTP 5xx responses.
- **Queue Drain & Index Time:**
  - CouchDB view / index regeneration upon adding new share references must settle within **< 5 seconds**.

---

## 3. Files per Instance

### Question Answered
*What is the maximum number of file documents an instance database can hold before directory listing, pagination, and tree updates degrade CouchDB view performance?*

- **Unit of result:** `files` (total file and folder metadata documents in the `io.cozy.files` doctype).

### Dimensions
- **Varying dimension:**
  - Total file count: **10,000**, **100,000**, **500,000**, **1,000,000** files.
- **Fixed dimensions:**
  - Directory structure: realistic hierarchical folder tree (depth 3–5 levels, average 50–200 files per folder).
  - Workload: 20 concurrent readers navigating directories, requesting pagination, and querying file metadata by ID.
  - Storage backend: CouchDB metadata store with local/Swift blob storage.

### Pass Criteria
- **P95 Latency:**
  - Directory browsing (`GET /files/:folder_id/browse` for 100 items): **P95 < 150 ms**.
  - File metadata fetch by ID (`GET /files/:id`): **P95 < 50 ms**.
  - File creation metadata write (`POST /files` excluding blob upload): **P95 < 300 ms**.
- **Error Rate:** **< 0.05%** failed requests.
- **Queue Drain & Index Time:**
  - Incremental CouchDB index catch-up lag following a 1,000-file batch insertion: **< 10 seconds**.

---
## 4. Concurrent Requests per Instance

### Question Answered
*What is the maximum number of concurrent requests (or virtual users) a single Cozy instance can serve under mixed read/write traffic before saturating Go worker pools, socket buffers, or database connections?*

- **Unit of result:** `concurrent requests` (simultaneous virtual users `VUs` or sustained `req/s`).

### Dimensions
- **Varying dimension:**
  - Load concurrency: **10**, **25**, **50**, **100**, **250**, **500** concurrent VUs.
- **Fixed dimensions:**
  - Pre-seeded data state: realistic baseline (10,000 files, 50 shared drives).
  - Traffic profile: mixed real-world activity:
    - 70% read operations (directory browse, stat, changes feed).
    - 20% write operations (small/medium file upload, metadata touch).
    - 10% WebSocket realtime connections.
  - Network round-trip latency: LAN / local baseline (< 5 ms RTT).

### Pass Criteria
- **P95 Latency:**
  - Read requests: **P95 < 300 ms**.
  - Write requests: **P95 < 500 ms** (excluding payload transfer time).
  - WebSocket connection handshake: **P95 < 100 ms**.
- **Error Rate:** **< 0.1%** non-2xx responses or connection timeouts.
- **Queue Drain & Stabilization:**
  - Request queue and in-flight backpressure must fully drain within **< 5 seconds** after load step completion (no goroutine accumulation, memory stabilizes at quiescent plateau).

---

## Architectural Impact on Load-Test Harness

This capacity model directly dictates two core components of the load-test harness:

1. **Generalized Capacity Engine (`src/lib/capacity/`)**:
   - Replaces the upload-specific binary search in `find-upload-capacity.ts` with a modular engine capable of varying arbitrary dimensions (`VUs`, `members`, `shares`, `files`).
   - Evaluates multidimensional pass criteria: composite P95 latency thresholds, max error rate, and queue drain time.

2. **Pre-Seeded Data Generator (`src/scripts/seed-scale-data.ts`)**:
   - Provides fast, automated provisioning of the test sizes (1k → 50k members, 100 → 25k shares, 10k → 1M files).
   - Measures provision duration as a baseline benchmark.
   - Supports clean teardown per test run to prevent CouchDB state corruption.
