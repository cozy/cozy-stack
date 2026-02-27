# ADR: Universal Drive Domain Architecture

## Status

**Proposed/Draft**

## Context

### Problem

When UserA shares a file in a shared drive with UserB, the URL sharing workflow is broken:

1. UserA copies URL from browser: `https://usera.org.com/drive/#/file/abc123`
2. UserA sends this URL to UserB via chat/email
3. UserB clicks the link but cannot access the file

**Root Cause:** The current architecture uses user-centric domains where each user has their own subdomain (`usera.org.com`, `userb.org.com`). The domain determines which CouchDB database to query, making URLs inherently tied to a specific user's data context.

### Current Architecture

```
User A: https://usera.twake.example.com/drive/
        └── CouchDB: usera.twake.example.com/files

User B: https://userb.twake.example.com/drive/
        └── CouchDB: userb.twake.example.com/files
```

When UserB opens UserA's URL:
- The server looks up `usera.twake.example.com` instance
- UserB has no session/authentication on UserA's instance
- Access is denied even though UserB may have sharing permissions

### Constraints

- No "Copy Link" button - users copy directly from browser address bar
- Users may be from different organizations with separate identity providers
- Solution should minimize user interaction (no manual URL/email entry)
- Must support cross-organization sharing (secondary priority)

## Proposal

### Organization-Centric Universal Domain

Replace user-specific domains with a single **organization-wide Drive domain**:

```
Current:  usera.org.com/drive  ← UserA's drive
          userb.org.com/drive  ← UserB's drive

Proposed: drive.org.com        ← Universal Drive for all users in org
```

### Architecture

The Universal Drive functionality is **integrated directly into cozy-stack** rather than deployed as a separate service. This provides better performance, code reuse, and simpler deployment.

```
┌──────────────────────────────────────────────────────────────────┐
│                        User's Browser                             │
│  URL: https://drive.org.com/files/{file-id}                      │
│       https://drive.org.com/shares/{sharing-id}/files/{file-id}  │
│       https://drive.org.com/public/{encoded-sharecode}           │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│                         Cozy-Stack                                │
│  (Universal Drive Mode - org-centric domain handling)            │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Universal Drive Router (new)                              │  │
│  │  - Detects org-centric domain (drive.org.com)              │  │
│  │  - OIDC client for Registration App                        │  │
│  │  - Routes to appropriate instance based on session         │  │
│  │  - Handles public links via encoded sharecodes             │  │
│  └────────────────────────────────────────────────────────────┘  │
│                              │                                    │
│              ┌───────────────┼───────────────┐                    │
│              ▼               ▼               ▼                    │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐     │
│  │   Instance A    │ │   Instance B    │ │   Instance C    │     │
│  │   (UserA)       │ │   (UserB)       │ │   (UserC)       │     │
│  │   CouchDB: A    │ │   CouchDB: B    │ │   CouchDB: C    │     │
│  └─────────────────┘ └─────────────────┘ └─────────────────┘     │
└──────────────────────────────────────────────────────────────────┘
                                │
                                │ OIDC Auth
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│                      Registration App                             │
│                      (OIDC Provider)                              │
│                                                                   │
│  - User authentication via organization's IdP                    │
│  - LDAP user lookup                                              │
│  - Returns id_token with workplaceFQDN claim                     │
└──────────────────────────────────────────────────────────────────┘
```

**Benefits of integrated architecture:**
- Single codebase to maintain
- No additional network hop (internal routing vs HTTP proxy)
- Full access to existing VFS, CouchDB, auth, and permission infrastructure
- Simpler deployment and operations
- Shared test infrastructure

### URL Design

| URL Pattern | Description | Routing | Auth Required |
|-------------|-------------|---------|---------------|
| `/files/{file-id}` | Personal files | Route to authenticated user's instance | Yes |
| `/shares/{sharing-id}/files/{file-id}` | Shared drive files | Query user's instance for sharing info | Yes |
| `/public/{encoded-sharecode}` | Public link files | Decode sharecode to find instance | No |

**Note:** For public links, `encoded-sharecode` contains instance information as `base64(instance-slug:original-sharecode)`. For authenticated shared drive access, the user's instance is queried to resolve the sharing.

#### Shared Drive Links (Member-to-Member)

For authenticated shared drive access, we need to resolve `sharing-id` → `owner-instance`. There are three approaches:

**Encode Instance in Sharing ID**

Similar to public links, encode the owner's instance slug into the sharing ID:

```
Encoded sharing-id = base64(owner-instance-slug + ":" + original-sharing-id)

Example:
  Original: sharing-id = "abc123def" owned by usera.org.com
  Encoded:  sharing-id = "dXNlcmE6YWJjMTIzZGVm" (base64 of "usera:abc123def")

URL: /shares/dXNlcmE6YWJjMTIzZGVm/files/{file-id}
```

- **Pros:** Self-contained, no external lookup needed
- **Cons:** Breaks backward compatibility with existing sharing IDs, requires migration

#### Public Links (Share-by-Link)

For public links where there is no authenticated user, the instance information is encoded into the sharecode:

```
Encoded sharecode = base64(instance-slug + ":" + original-sharecode)

Example:
  Original: ?sharecode=abc123 on usera.org.com
  Encoded:  /public/dXNlcmE6YWJjMTIz (base64 of "usera:abc123")
```

This approach:
- Keeps URLs clean (no visible instance information)
- Works with existing sharecode/permission infrastructure
- Universal Drive decodes to find target instance, then validates sharecode

### Key Components

1. **Universal Drive Router** (new module in cozy-stack)
   - Detects org-centric domain requests (`drive.org.com`)
   - OIDC client for Registration App authentication
   - Session management with `workplaceFQDN` from id_token
   - Internal routing to appropriate instance (no HTTP proxy needed)
   - Public link handling with encoded sharecodes

2. **Registration App as OIDC Provider**
   - Existing Twake Workplace Registration application
   - Already stores user → instance mapping (`workspaceUrl` in LDAP)
   - Extended to act as OIDC Identity Provider
   - Returns `workplaceFQDN` claim in id_token after authentication

3. **Cozy-Stack Core Changes**
   - Domain detection for org-centric vs instance-centric mode
   - Cross-instance request handling within same process
   - WebSocket realtime domain injection for direct connections

### Authentication Flow

```
┌─────────────┐                    ┌─────────────────┐                    ┌─────────────────┐
│   Browser   │                    │   Cozy-Stack    │                    │ Registration App│
│             │                    │ (Universal Mode)│                    │ (OIDC Provider) │
└─────────────┘                    └─────────────────┘                    └─────────────────┘
       │                                   │                                      │
       │  1. GET drive.org.com/files/123   │                                      │
       │──────────────────────────────────►│                                      │
       │                                   │                                      │
       │  2. 302 Redirect to Registration  │                                      │
       │     /auth?client_id=drive&        │                                      │
       │     redirect_uri=drive.org/cb     │                                      │
       │◄──────────────────────────────────│                                      │
       │                                   │                                      │
       │  3. User authenticates            │                                      │
       │──────────────────────────────────────────────────────────────────────────►
       │                                   │                                      │
       │  4. 302 Redirect back with code   │                                      │
       │◄──────────────────────────────────────────────────────────────────────────
       │                                   │                                      │
       │  5. GET drive.org/cb?code=xxx     │                                      │
       │──────────────────────────────────►│                                      │
       │                                   │  6. Exchange code for tokens         │
       │                                   │─────────────────────────────────────►│
       │                                   │                                      │
       │                                   │  7. Return id_token + access_token   │
       │                                   │◄─────────────────────────────────────│
       │                                   │                                      │
       │  8. Session created, serve file   │  (internal routing to user instance) │
       │◄──────────────────────────────────│                                      │
```

### id_token Claims

The Registration App returns an id_token containing the user's instance URL:

```json
{
  "iss": "https://registration.org.com",
  "sub": "user-uuid-123",
  "aud": "universal-drive",
  "email": "bob@acme.com",
  "name": "Bob Smith",
  "workplaceFQDN": "https://bob.twake.acme.com",
  "iat": 1704700000,
  "exp": 1704703600
}
```

The `workplaceFQDN` claim is the key - it tells Universal Drive which Cozy-Stack instance to proxy requests to.

### Request Routing

1. User navigates to `drive.org.com`
2. Cozy-stack detects org-centric domain, enters Universal Drive mode
3. If no session → redirect to Registration App for OIDC authentication
4. After authentication → extract `workplaceFQDN` from id_token, create session
5. For personal files → internal routing to user's instance context
6. For shared files → look up sharing owner's instance, internal routing
7. For public links → decode sharecode to find instance, validate permissions

### WebSocket / Realtime Handling

WebSocket connections for realtime events (file changes, notifications) connect **directly to the user's instance domain** rather than through the Universal Drive domain. This avoids the complexity of WebSocket proxying and provides better performance.

**Approach:** Inject a separate `realtimeDomain` in cozyData:

```javascript
window.cozyData = {
  "token": "eyJhbG...",
  "domain": "drive.org.com",           // API calls via Universal Drive
  "realtimeDomain": "usera.org.com",   // WebSocket direct to user's instance
  "app": { "slug": "drive" }
}
```

**Flow:**
```
┌─────────────┐              ┌─────────────────┐              ┌─────────────┐
│   Browser   │              │   Cozy-Stack    │              │ User's      │
│ (cozy-drive)│              │ (Universal Mode)│              │ Instance    │
└─────────────┘              └─────────────────┘              └─────────────┘
       │                             │                               │
       │  HTTP API calls             │                               │
       │  (drive.org.com)            │                               │
       │────────────────────────────►│  internal routing             │
       │                             │──────────────────────────────►│
       │                             │                               │
       │  WebSocket connection       │                               │
       │  (usera.org.com/realtime)   │                               │
       │─────────────────────────────────────────────────────────────►
       │                             │                               │
```

**Required changes:**
1. **cozy-client/cozy-realtime:** Check for `realtimeDomain` in cozyData, use it for WebSocket URL
2. **CORS:** Configure cozy-stack to accept WebSocket connections with Universal Drive origin
3. **Auth:** WebSocket AUTH message works with existing token mechanism

## Reusing Existing cozy-drive Frontend

A key advantage of this architecture is that **the existing cozy-drive frontend can be reused without modifications**.

### How cozy-drive Works Today

The cozy-stack serves cozy-drive with injected configuration:

```html
<!-- index.html served by cozy-stack -->
<script>
  window.cozyData = {
    "token": "eyJhbG...",
    "domain": "usera.example.com",
    "app": { "slug": "drive" }
  }
</script>
```

The frontend uses `cozyData.domain` for all API calls:
```javascript
// cozy-client-js reads domain from cozyData
fetch(`https://${cozyData.domain}/files/${fileId}`, {
  headers: { Authorization: `Bearer ${cozyData.token}` }
})
```

### How Universal Drive Reuses cozy-drive

The frontend doesn't care WHO serves it, as long as:
1. It receives valid `cozyData` with a token
2. API endpoints respond correctly

**Universal Drive mode serves the same cozy-drive app with modified cozyData:**

```html
<!-- index.html served by cozy-stack in Universal Drive mode -->
<script>
  window.cozyData = {
    "token": "eyJhbG...",
    "domain": "drive.org.com",           // ← API calls go to universal domain
    "realtimeDomain": "usera.org.com",   // ← WebSocket connects directly to instance
    "app": { "slug": "drive" }
  }
</script>
```

### Request Flow with Reused Frontend

```
┌─────────────┐              ┌──────────────────────────────────────────────┐
│   Browser   │              │              Cozy-Stack                      │
│ (cozy-drive)│              │  ┌─────────────────┐    ┌─────────────────┐  │
└─────────────┘              │  │ Universal Drive │    │  User Instance  │  │
       │                     │  │     Router      │    │    Context      │  │
       │                     │  └─────────────────┘    └─────────────────┘  │
       │                     └──────────────────────────────────────────────┘
       │                             │                        │
       │  1. GET drive.org.com       │                        │
       │────────────────────────────►│                        │
       │                             │                        │
       │  2. cozy-drive HTML with    │                        │
       │     cozyData.domain =       │                        │
       │     "drive.org.com"         │                        │
       │     cozyData.realtimeDomain │                        │
       │     = "usera.org.com"       │                        │
       │◄────────────────────────────│                        │
       │                             │                        │
       │  3. GET /files/123          │                        │
       │     (to drive.org.com)      │                        │
       │────────────────────────────►│                        │
       │                             │  4. Internal routing   │
       │                             │     to user context    │
       │                             │───────────────────────►│
       │                             │                        │
       │  5. File data               │◄───────────────────────│
       │◄────────────────────────────│                        │
```

### What Changes Where

| Component | Changes Needed |
|-----------|----------------|
| **cozy-drive frontend** | Minor: Support `realtimeDomain` for WebSocket connections |
| **cozy-client/cozy-realtime** | Use `realtimeDomain` from cozyData if present |
| **Cozy-Stack** | New Universal Drive router module, org-centric domain detection, cross-instance routing |
| **Registration App** | Add OIDC provider endpoints, return `workplaceFQDN` in id_token |

### Benefits of Reusing cozy-drive

1. **No frontend rewrite** - Significant development time saved
2. **Feature parity** - All existing Drive features work immediately
3. **Single codebase** - No divergence between "old" and "new" Drive
4. **Proven UI** - Battle-tested interface, no new bugs
5. **Transparent to users** - Same familiar interface

## Alternatives

### Alternative 1: User-Specific URLs with Redirect Endpoint

**Description:** Keep current user-centric domains but add a redirect mechanism.

**How it works:**
1. Frontend detects access failure on wrong instance
2. Redirects to `/sharings/drives/{id}/redirect/{file-id}`
3. Backend identifies user (via discovery page) and redirects to their instance

**Pros:**
- Minimal backend changes
- Works with current architecture

**Cons:**
- Requires user interaction (entering email/Cozy URL)
- Not seamless UX
- Only solves shared files, not personal files

### Alternative 2: Central Gateway for Shared Files Only

**Description:** Central gateway domain only for shared file URLs.

**URL format:** `https://gateway.org.com/shares/{sharing-id}/files/{file-id}`

**Pros:**
- Narrower scope than universal domain
- Less infrastructure change

**Cons:**
- Different URLs for personal vs shared files
- Users don't know when to use which URL format
- Still need mechanism to identify users

### Alternative 3: Cookie-Based Instance Tracking

**Description:** Store user's instance URL in a cookie when they first accept a sharing.

**Pros:**
- No user interaction on subsequent visits

**Cons:**
- Device-specific (cookies don't sync across devices)
- Only works for repeat visits
- Still requires initial discovery

### Alternative 4: Member List Selection

**Description:** On access failure, show list of member organizations for user to select.

**URL flow:**
```
UserB opens: https://usera.example/drive/#/shares/{id}/file/{fid}
Shows page:  "Select your organization"
             - Org A (usera.example.com)
             - Org B (userb.example.com) ← click
Redirect:    https://userb.example/drive/#/shares/{id}/file/{fid}
```

**Pros:**
- One-click instead of typing
- Minimal backend changes

**Cons:**
- Still requires user interaction
- Only solves shared files

## Decision

**Adopt the Universal Drive Domain architecture (Proposal).**

### Rationale

1. **Solves the root cause:** By using a single domain per organization, URL sharing works naturally because all users access the same domain.

2. **Clean architecture:** User identity determined by authentication, not URL domain. This aligns with modern SaaS patterns.

3. **Future-proof:** Easier to add new features (collaboration, real-time editing) when all users share a common entry point.

4. **Better UX:** Users have one domain to remember, consistent experience across personal and shared files.

5. **Security:** Centralized authentication with service-to-service tokens between gateway and instances.

6. **Reuses existing infrastructure:** The Registration App already stores user → instance mappings (`workspaceUrl` in LDAP). Extending it as an OIDC provider eliminates the need for a separate user registry.

### Cross-Organization Sharing

For sharing between different organizations (e.g., UserA at `drive.acme.com` shares with UserB at `drive.globex.com`), additional mechanisms are needed since the Universal Drive domain is per-organization.

#### Potential Approaches

**Option A: Federated OIDC Authentication**

Allow users to authenticate with their home organization's IdP when accessing another organization's Universal Drive.

```
UserB (globex.com) → drive.acme.com → "Select your organization"
                                     → Redirects to globex.com IdP
                                     → Returns with federated identity
                                     → Access granted based on sharing membership
```

- **Pros:** Seamless SSO experience, strong identity verification
- **Cons:** Requires trust relationship between organizations, complex setup

**Option B: Global Gateway with Organization Selection**

A multi-tenant gateway (`drive.twake.app`) that routes to any organization:

```
URL: https://drive.twake.app/org/acme/shares/{sharing-id}/files/{file-id}
```

- **Pros:** Single URL format works across all organizations
- **Cons:** Centralized infrastructure, trust implications

**Option C: Discovery Page Fallback**

When a user from another organization accesses a shared link:
1. Show discovery page: "Enter your email or Cozy URL"
2. System looks up user's organization
3. Redirect to their organization's Universal Drive with sharing context

- **Pros:** Works without federation setup, uses existing discovery flow
- **Cons:** Requires user interaction, not fully seamless

**Option D: Email-Based Magic Links**

When sharing cross-org, send email with personalized link that encodes recipient's organization:

```
Email to UserB: "Click here to access shared folder"
Link: https://drive.globex.com/external-share/{encoded-token}
```

- **Pros:** Direct link to user's own organization domain
- **Cons:** Only works via sharing invitation, not URL copy-paste

#### Recommended Initial Approach

For initial implementation, use **Option C (Discovery Page)** as it:
- Works immediately without additional infrastructure
- Builds on existing sharing invitation flow
- Can be enhanced later with federated auth (Option A)

Cross-organization sharing is a secondary priority and will be fully addressed in a future ADR.

## Consequences

### Positive

1. **URL sharing works seamlessly** within an organization
2. **Simplified user experience** - one domain for all Drive access
3. **Centralized auth** enables better security controls and audit logging
4. **Scalable architecture** - can add more instances without changing URLs
5. **Foundation for future features** - real-time collaboration, unified search
6. **Single codebase** - integrated into cozy-stack, no separate service to maintain
7. **Better performance** - internal routing instead of HTTP proxy hop
8. **Code reuse** - leverages existing VFS, auth, permissions infrastructure

### Negative

1. **Cozy-stack complexity** - adds org-centric logic to instance-centric codebase
2. **Migration complexity** - transitioning from user-centric to org-centric URLs
3. **Cross-org sharing deferred** - not fully solved by this ADR (basic approach outlined)
4. **Frontend changes** - cozy-client needs `realtimeDomain` support for WebSocket

### Risks

1. **Migration risk:** Existing bookmarks, links, and integrations using old URLs will break
   - Mitigation: 301 redirects from old URLs, transition period

2. **Complexity risk:** Adding org-centric mode may affect existing instance-centric behavior
   - Mitigation: Feature flag, thorough testing, gradual rollout

3. **WebSocket CORS risk:** Direct WebSocket connections to instance domain from universal domain
   - Mitigation: Configure CORS headers, validate origin in WebSocket handshake

### Implementation Phases

| Phase | Description |
|-------|-------------|
| 1 | Extend Registration App as OIDC Provider |
| 2 | Add Universal Drive mode to cozy-stack (domain detection, routing) |
| 3 | Implement OIDC client and session management in cozy-stack |
| 4 | Add public link encoding/decoding for sharecodes |
| 5 | Update cozy-client/cozy-realtime for `realtimeDomain` support |
| 6 | DNS configuration and testing |
| 7 | Migration tooling and 301 redirects from old URLs |

### Files to Create/Modify

**Registration App Changes (existing repo: twake-workplace-private/registration):**

| File | Changes |
|------|---------|
| `src/routes/api/oauth/` (new) | OIDC authorization and token endpoints |
| `src/lib/services/oauth/` (new) | OIDC token generation with `workplaceFQDN` claim |
| `src/db/oauth-clients.schema.ts` (new) | OAuth client registration table |

**Cozy-Stack Changes:**

```
cozy-stack/
├── web/
│   ├── universal/                    # New package for Universal Drive mode
│   │   ├── universal.go              # Main router and mode detection
│   │   ├── oidc.go                   # OIDC client for Registration App
│   │   ├── session.go                # Universal session management
│   │   ├── routing.go                # Cross-instance routing logic
│   │   ├── public.go                 # Public link handling
│   │   └── sharecode.go              # Sharecode encoding/decoding
│   ├── routing.go                    # Modified: detect org-centric domain
│   └── server.go                     # Modified: register universal routes
├── pkg/config/
│   └── config.go                     # Add universal drive configuration
└── model/instance/
    └── instance.go                   # Add helpers for universal mode
```

| File | Changes |
|------|---------|
| `web/routing.go` | Detect org-centric domain, route to Universal Drive handler |
| `web/server.go` | Register universal drive routes |
| `pkg/config/config.go` | Universal drive configuration (org domains, OIDC settings) |
| `web/realtime/realtime.go` | CORS support for universal domain origin |

**Frontend Changes (cozy-client/cozy-drive):**

| File | Changes |
|------|---------|
| `cozy-client/src/CozyClient.js` | Support `realtimeDomain` in cozyData |
| `cozy-realtime/src/index.js` | Use `realtimeDomain` for WebSocket URL if present |


## Resolved Design Questions

### 1. URLs for Public Links (No Auth)

**Question:** Do we need to include user_id/slug in URLs for files shared via public link where there is no auth information?

**Decision:** Yes, but encoded into the sharecode rather than visible in the URL path.

**Implementation:**
- Encode instance information into an extended sharecode: `base64(instance-slug + ":" + original-sharecode)`
- URL format: `https://drive.org.com/public/{encoded-sharecode}`
- Universal Drive decodes to find target instance, then validates with original sharecode

**Rationale:** Keeps URLs clean while providing necessary routing information.

### 2. URL Building for Files

**Question:** How do we build URLs for files? Currently we use slug to build the URL.

**Decision:** URLs no longer include user/instance slug. Instance routing is determined server-side based on:
- **Authenticated users:** Session contains `workplaceFQDN` from OIDC id_token
- **Shared files:** Sharing ID maps to owner's instance
- **Public links:** Instance encoded in sharecode

**URL patterns:**
```
Personal files:  https://drive.org.com/files/{file-id}
Shared files:    https://drive.org.com/shares/{sharing-id}/files/{file-id}
Public links:    https://drive.org.com/public/{encoded-sharecode}
```

### 3. Serving cozy-drive App

**Question:** How do we create/serve the cozy app in Universal Drive mode?

**Decision:** Integrated into cozy-stack, serving existing cozy-drive assets with modified cozyData injection.

**Implementation:**
- Cozy-stack detects org-centric domain and enters Universal Drive mode
- Serves same cozy-drive assets from existing app file server
- Injects modified `cozyData` with:
  - `domain`: Universal Drive domain (for API calls)
  - `realtimeDomain`: User's instance domain (for WebSocket)
  - `token`: Generated token for proxied requests

### 4. WebSocket/Realtime Handling

**Question:** How do we proxy WebSocket connections? Should we inject a different domain for WebSocket?

**Decision:** Direct WebSocket connection to user's instance domain (not proxied through Universal Drive).

**Implementation:**
- Inject `realtimeDomain` in cozyData pointing to user's instance
- cozy-client/cozy-realtime uses `realtimeDomain` for WebSocket URL
- Configure CORS to allow Universal Drive origin for WebSocket connections

**Rationale:**
- Avoids complexity of WebSocket proxying
- Better performance (no additional hop)
- Leverages existing proven realtime infrastructure

### 5. Architecture Choice

**Question:** Should Universal Drive be a separate service or integrated into cozy-stack?

**Decision:** Integrated into cozy-stack as a new routing mode.

**Rationale:**
- Single codebase to maintain
- No additional network hop (internal routing)
- Full access to existing VFS, CouchDB, auth infrastructure
- Simpler deployment and operations
- Shared test infrastructure

### 6. Sharing ID to Owner Instance Lookup

**Question:** How do we resolve a sharing ID to the owner's instance without a global index?

**Decision:** Query the authenticated user's instance to get sharing information, which includes the owner's instance.

**Implementation:**
1. User is authenticated → session contains `workplaceFQDN` (user's instance)
2. Query user's instance for sharing: `sharing.FindSharing(userInstance, sharingID)`
3. Sharing document contains owner instance information
4. Route request to owner's instance

**Flow:**
```
GET /shares/abc123def/files/xyz
  ↓
Session: user's instance = userb.org.com
  ↓
Query userb's CouchDB: GET /sharings/abc123def
  → Response: { owner: { instance: "usera.org.com" }, ... }
  ↓
Route to usera.org.com for file access
```

**Rationale:**
- Preserves backward compatibility with existing sharing IDs (no encoding needed)
- No migration required for existing sharings
- Leverages existing sharing replication infrastructure
- Query is internal (same cozy-stack process), minimal latency
- No additional infrastructure or global index needed

**Trade-off:** Requires sharing to be replicated to member's instance before they can access via Universal Drive. This is already the case for normal sharing access.