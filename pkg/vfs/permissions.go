package vfs

import (
	"errors"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// Validable extends on permissions.validable with hierarchy functions
type Validable interface {
	permissions.Validable
	parentID() string
	Path(fs VFS) (string, error)
	Parent(fs VFS) (*DirDoc, error)
}

// FileDoc & DirDoc are vfs.Validable
var _ Validable = (*FileDoc)(nil)
var _ Validable = (*DirDoc)(nil)

// Allows check if a permSet allows verb on given file
// µoptim : we can probably make this function iterate less on pset.Rules, but
//  it will lower readability ...
func Allows(fs VFS, pset permissions.Set, v permissions.Verb, fd Validable) error {
	allowedIDs := []string{}
	otherRules := []permissions.Rule{}

	// First pass, we iterate over the rules, check if we have an easy match
	// keep a short list of useful rules and allowed IDs.
	for _, r := range pset {
		if r.Type != consts.Files || !r.Verbs.Contains(v) {
			continue
		}

		// permission on whole io.cozy.files doctype
		if len(r.Values) == 0 {
			return nil
		}

		// permission by ID directly on self, parent or root
		if r.Selector == "" {
			for _, v := range r.Values {
				if v == fd.ID() || v == fd.parentID() || v == consts.RootDirID {
					return nil
				}
				allowedIDs = append(allowedIDs, v)
			}
		}

		// permission by attributes values (tags, mime ...) on self
		var valid = func(value string) bool { return fd.Valid(r.Selector, value) }
		if r.SomeValue(valid) {
			return nil
		}

		// store rules that could apply to an ancestor
		if r.Selector != "mime" && r.Selector != "class" {
			otherRules = append(otherRules, r)
		}
	}

	// We have some rules on IDs, let's fetch their paths and check if they are
	// ancestors of current object
	if len(allowedIDs) > 0 {
		var selfPath, err = fd.Path(fs)
		if err != nil {
			return err
		}

		for _, id := range allowedIDs {
			allowedPath, err := pathFromID(fs, id)
			// tested is children of allowed
			// err is ignored, it most probably means a permissions on a
			// deleted directory. @TODO We will want to clean this up.
			if err == nil && strings.HasPrefix(selfPath, allowedPath+"/") {
				return nil
			}
		}

	}

	// We have some rules on attributes, let's iterate over the current object
	// ancestors and check if any match the rules
	if len(otherRules) > 0 {
		cur, err := fd.Parent(fs)
		if err != nil {
			return err
		}
		for cur.ID() != consts.RootDirID {
			for _, rule := range otherRules {
				if rule.ValuesValid(cur) {
					return nil
				}
				cur, err = cur.Parent(fs)
				if err != nil {
					return err
				}
			}
		}
	}

	// no match : game over !
	return errors.New("no permission")
}

func pathFromID(fs VFS, id string) (string, error) {
	if id == consts.RootDirID {
		return "", nil
	}

	if id == consts.TrashDirID {
		return TrashDirName, nil
	}

	dir, err := fs.DirByID(id)
	if err != nil {
		return "", err
	}

	return dir.Path(fs)
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

func (f *FileDoc) parentID() string { return f.DirID }
func (d *DirDoc) parentID() string  { return d.DirID }

// Valid implements permissions.Validable on FileDoc
func (f *FileDoc) Valid(field, expected string) bool {
	switch field {
	case "type":
		return f.Type == expected
	case "name":
		return f.DocName == expected
	case "mime":
		return f.Mime == expected
	case "class":
		return f.Class == expected
	case "tags":
		return contains(f.Tags, expected)
	default:
		return false
	}
}

// Valid implements permissions.Validable on DirDOc
func (d *DirDoc) Valid(field, expected string) bool {
	switch field {
	case "type":
		return d.Type == expected
	case "name":
		return d.DocName == expected
	case "tags":
		return contains(d.Tags, expected)
	default:
		return false
	}
}
