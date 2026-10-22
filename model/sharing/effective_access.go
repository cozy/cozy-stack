package sharing

import (
	"os"
	"path"
	"slices"
	"sort"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

// SharingScope describes a single sharing scope that applies to a target file
// or folder. It is produced by AccessResolver.scopesMatching and consumed by
// resolve to aggregate an EffectiveAccess. Sharing is the backing sharing
// document and Member the matching member's entry inside it (the same object
// as Sharing.Members[i]): the pair is coherent by construction.
type SharingScope struct {
	Sharing    *Sharing
	Member     *Member
	RootID     string
	RootPath   string
	AccessMode string
}

// EffectiveAccess is the merged result of all applicable additive sharing
// scopes for a target, from the perspective of the current instance. CanRead
// is true if the instance is a member of at least one applicable scope;
// CanWrite is true if at least one such scope grants write (highest right
// wins).
type EffectiveAccess struct {
	CanRead          bool
	CanWrite         bool
	SourceSharingIDs []string
}

// Can reports whether the effective access satisfies the given verb. GET
// requires CanRead; POST, PUT, PATCH and DELETE require CanWrite.
func (ea *EffectiveAccess) Can(verb permission.Verb) bool {
	if ea == nil {
		return false
	}
	switch verb {
	case permission.GET:
		return ea.CanRead
	case permission.POST, permission.PUT, permission.PATCH, permission.DELETE:
		return ea.CanWrite
	}
	return false
}

// AccessResolver computes the effective access for a target file or folder,
// covering the target itself (if shared) plus all shared ancestors on its
// path, filtered to scopes where the current instance is a member.
type AccessResolver struct {
	inst *instance.Instance
}

// NewAccessResolver builds an AccessResolver bound to an instance.
func NewAccessResolver(inst *instance.Instance) *AccessResolver {
	return &AccessResolver{inst: inst}
}

// Resolve returns the effective access for the given file or folder. The
// target's own share (if it is a shared file) and the shares of every
// ancestor directory are all considered. Scopes where the current instance
// is not a member are ignored. The highest permission wins: CanRead if at
// least one applicable scope, CanWrite if at least one non-read-only scope.
func (r *AccessResolver) Resolve(targetID string) (*EffectiveAccess, error) {
	return r.resolve(targetID, func(s *Sharing) *Member {
		return s.MemberFor(r.inst)
	})
}

// ResolveForMember returns the effective access for the given sharing member
// on the target file or folder. The member is matched across sharings by
// email or instance host (see Sharing.MemberMatching), which allows checking
// the access of a specific recipient on the owner's instance.
func (r *AccessResolver) ResolveForMember(targetID string, member *Member) (*EffectiveAccess, error) {
	return r.resolve(targetID, func(s *Sharing) *Member {
		return s.MemberMatching(member)
	})
}

// CrossesDriveBoundary reports whether moving a doc from one directory to
// another enters or leaves a drive, nested ones included, i.e. whether
// different drive sharings apply to the two directories. A drive rooted at
// the moved doc is under neither of them: it moves along with the doc.
func (r *AccessResolver) CrossesDriveBoundary(from, to *vfs.DirDoc) (bool, error) {
	fromIDs, err := r.driveSharingIDs(from)
	if err != nil {
		return false, err
	}
	toIDs, err := r.driveSharingIDs(to)
	if err != nil {
		return false, err
	}
	return !slices.Equal(fromIDs, toIDs), nil
}

// driveSharingIDs returns the sorted IDs of the drive sharings applying to the
// directory, i.e. rooted at it or at one of its ancestors. Only the drives the
// instance is a member of are kept.
func (r *AccessResolver) driveSharingIDs(dir *vfs.DirDoc) ([]string, error) {
	sharings, _, err := r.applicableSharingsForDoc(dir, nil)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(sharings))
	for _, s := range sharings {
		if s.Drive && s.MemberFor(r.inst) != nil {
			ids = append(ids, s.SID)
		}
	}
	return ids, nil
}

func (r *AccessResolver) resolve(targetID string, memberOf func(*Sharing) *Member) (*EffectiveAccess, error) {
	scopes, err := r.scopesMatching(targetID, memberOf)
	if err != nil {
		return nil, err
	}
	ea := &EffectiveAccess{SourceSharingIDs: make([]string, 0, len(scopes))}
	for _, sc := range scopes {
		ea.SourceSharingIDs = append(ea.SourceSharingIDs, sc.Sharing.SID)
		ea.CanRead = true
		if !sc.Member.ReadOnly {
			ea.CanWrite = true
		}
	}
	return ea, nil
}

// rootInfo describes a shared root applying to a target: the directory (or
// file) that is the root of a sharing scope.
type rootInfo struct {
	RootID   string
	RootPath string
	RootName string
}

// scopesMatching loads the target, builds ancestor paths, finds shared roots
// on the path, adds the target's own file share if any, bulk-loads sharings,
// filters to active additive ones, and returns the scopes where memberOf
// resolves a member (nil member = scope does not apply).
func (r *AccessResolver) scopesMatching(targetID string, memberOf func(*Sharing) *Member) ([]SharingScope, error) {
	sharings, rootBySharing, err := r.applicableSharings(targetID)
	if err != nil {
		return nil, err
	}

	scopes := make([]SharingScope, 0, len(sharings))
	for _, s := range sharings {
		info, ok := rootBySharing[s.SID]
		if !ok {
			continue
		}
		member := memberOf(s)
		if member == nil {
			continue
		}
		scopes = append(scopes, SharingScope{
			Sharing:    s,
			Member:     member,
			RootID:     info.RootID,
			RootPath:   info.RootPath,
			AccessMode: s.EffectiveAccessMode(),
		})
	}
	return scopes, nil
}

// applicableSharings resolves the active additive sharings applying to the
// target: its own share (if it is a shared file) plus the shares of every
// ancestor directory, without any membership filtering. It also returns the
// root info of each sharing, keyed by sharing ID.
func (r *AccessResolver) applicableSharings(targetID string) ([]*Sharing, map[string]rootInfo, error) {
	dir, file, err := r.inst.VFS().DirOrFileByID(targetID)
	if err != nil {
		return nil, nil, err
	}
	if dir == nil && file == nil {
		return nil, nil, os.ErrNotExist
	}
	return r.applicableSharingsForDoc(dir, file)
}

// applicableSharingsForDoc is applicableSharings with the target doc already
// loaded by the caller (exactly one of dir/file must be non-nil).
func (r *AccessResolver) applicableSharingsForDoc(dir *vfs.DirDoc, file *vfs.FileDoc) ([]*Sharing, map[string]rootInfo, error) {
	fs := r.inst.VFS()

	var targetPath string
	var err error
	if dir != nil {
		targetPath = dir.Fullpath
	} else {
		targetPath, err = file.Path(fs)
		if err != nil {
			return nil, nil, err
		}
	}

	ancestorPaths := r.ancestorPaths(targetPath, dir != nil)

	// XXX: single find via dir-by-path + Go-side referenced_by filter, rather
	// than a dedicated shared-folder-roots-by-path view — one fewer view to
	// maintain while N ancestors is small.
	paths := make([]any, len(ancestorPaths))
	for i, p := range ancestorPaths {
		paths[i] = p
	}
	type sharedRootDoc struct {
		ID           string                 `json:"_id"`
		Path         string                 `json:"path"`
		Name         string                 `json:"name"`
		ReferencedBy []couchdb.DocReference `json:"referenced_by"`
	}
	var roots []sharedRootDoc
	if len(paths) > 0 {
		req := &couchdb.FindRequest{
			UseIndex: "dir-by-path",
			Selector: mango.And(
				mango.In("path", paths),
				mango.Equal("type", consts.DirType),
				mango.Exists(couchdb.SelectorReferencedBy),
			),
			Fields: []string{"_id", "path", "name", "referenced_by"},
			Limit:  len(paths),
		}
		if err := couchdb.FindDocs(r.inst, consts.Files, req, &roots); err != nil {
			return nil, nil, err
		}
	}

	// Collect (sharingID -> rootInfo) from ancestor dir roots.
	rootBySharing := make(map[string]rootInfo)
	for _, root := range roots {
		for _, ref := range root.ReferencedBy {
			if ref.Type == consts.Sharings {
				if _, ok := rootBySharing[ref.ID]; !ok {
					rootBySharing[ref.ID] = rootInfo{RootID: root.ID, RootPath: root.Path, RootName: root.Name}
				}
			}
		}
	}

	// XXX: DirType selector excludes the file from the Mango find; its
	// own scopes are added by hand so file shares stay additive at the target
	// level.
	if file != nil {
		for _, ref := range file.ReferencedBy {
			if ref.Type == consts.Sharings {
				if _, ok := rootBySharing[ref.ID]; !ok {
					rootBySharing[ref.ID] = rootInfo{RootID: file.DocID, RootPath: targetPath, RootName: file.DocName}
				}
			}
		}
	}

	if len(rootBySharing) == 0 {
		return nil, nil, nil
	}

	sharingIDs := make([]string, 0, len(rootBySharing))
	for id := range rootBySharing {
		sharingIDs = append(sharingIDs, id)
	}
	// Stable order: the recipient list derived from these sharings must not
	// change between identical calls.
	sort.Strings(sharingIDs)

	sharings, err := r.loadSharings(sharingIDs)
	if err != nil {
		return nil, nil, err
	}
	return sharings, rootBySharing, nil
}

// ancestorPaths returns the directory paths to query for shared roots. When
// the target is a directory, its own path is included (a dir can be a shared
// root). When the target is a file, only its parent directories are included.
func (r *AccessResolver) ancestorPaths(targetPath string, targetIsDir bool) []string {
	var paths []string
	current := path.Clean(targetPath)
	if !targetIsDir {
		current = path.Dir(current)
	}
	for {
		if current == "" || current == "." {
			break
		}
		paths = append(paths, current)
		if current == "/" {
			break
		}
		parent := path.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return paths
}

// loadSharings bulk-loads sharings by ID and keeps only the active additive
// ones. limited_access sharings are dropped in v1. Membership filtering is
// done by the caller (scopesMatching) so this helper stays reusable.
func (r *AccessResolver) loadSharings(ids []string) ([]*Sharing, error) {
	sharings, err := FindSharings(r.inst, ids)
	if err != nil {
		return nil, err
	}
	kept := make([]*Sharing, 0, len(sharings))
	for _, s := range sharings {
		if s == nil {
			continue
		}
		if !s.Active {
			continue
		}
		if s.EffectiveAccessMode() != AccessModeAdditive {
			continue
		}
		kept = append(kept, s)
	}
	return kept, nil
}

// NearestWritableSharing returns the sharing scope granting write access to
// the member that is nearest to the target: the shared root enclosing the
// target with the deepest path. Every applicable scope covers the target by
// construction, so the nearest one is the narrowest: a sharecode minted from
// it (see FileOpener.GetSharecode) lets the member act on the target without
// granting anything beyond their effective write scope. Returns nil when no
// applicable scope grants the member write.
func (r *AccessResolver) NearestWritableSharing(targetID string, member *Member) (*SharingScope, error) {
	scopes, err := r.scopesMatching(targetID, func(s *Sharing) *Member {
		return s.MemberMatching(member)
	})
	if err != nil {
		return nil, err
	}
	var nearest *SharingScope
	for i := range scopes {
		sc := &scopes[i]
		if sc.Member.ReadOnly {
			continue
		}
		if nearest == nil || len(sc.RootPath) > len(nearest.RootPath) {
			nearest = sc
		}
	}
	return nearest, nil
}

// NearestRestrictiveBoundary returns the closest limited_access boundary
// applying to the target. v1 has no limited_access mode, so it always
// returns nil.
//
// XXX: reserved for limited_access). v1 returns no boundary; upgrade path is
// to walk ancestors and stop at the first limited_access scope.
func (r *AccessResolver) NearestRestrictiveBoundary(targetID string) (*SharingScope, error) {
	return nil, nil
}

// ChildSharedRootsUnder returns shared roots nested under the given folder.
// v1 has no caller for it yet, so it returns nil.
//
// XXX: reserved for recursive move/copy/delete.
func (r *AccessResolver) ChildSharedRootsUnder(folderID string) ([]SharingScope, error) {
	return nil, nil
}
