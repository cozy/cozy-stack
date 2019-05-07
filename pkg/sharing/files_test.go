package sharing

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/assert"
)

func TestMakeXorKey(t *testing.T) {
	key := MakeXorKey()
	assert.Len(t, key, 16)
	for _, k := range key {
		assert.True(t, k < 16)
	}
}

func TestXorID(t *testing.T) {
	id := "12345678-abcd-90ef-1337-cafebee54321"
	assert.Equal(t, id, XorID(id, []byte{0}))
	assert.Equal(t, id, XorID(id, []byte{0, 0, 0, 0}))

	key := MakeXorKey()
	xored := XorID(id, key)
	assert.NotEqual(t, id, xored)
	assert.Equal(t, id, XorID(xored, key))
	for _, c := range xored {
		switch {
		case '0' <= c && c <= '9':
		case 'a' <= c && c <= 'f':
		default:
			assert.Equal(t, '-', c)
		}
	}

	expected := "03254769-badc-81fe-0226-dbefaff45230"
	assert.Equal(t, expected, XorID(id, []byte{1}))

	expected = "133b5777-bb3d-9fee-e327-cbf1bfea422e"
	assert.Equal(t, expected, XorID(id, []byte{0, 1, 0, 15}))
}

func TestSortFilesToSent(t *testing.T) {
	s := &Sharing{}
	foo := map[string]interface{}{"type": "directory", "name": "foo", "path": "/foo"}
	foobar := map[string]interface{}{"type": "directory", "name": "bar", "path": "/foo/bar"}
	foobarbaz := map[string]interface{}{"type": "directory", "name": "baz", "path": "/foo/bar/baz"}
	dela := map[string]interface{}{"_deleted": true, "name": "dela"}
	delb := map[string]interface{}{"_deleted": true, "name": "delb"}
	filea := map[string]interface{}{"type": "file", "name": "filea"}
	fileb := map[string]interface{}{"type": "file", "name": "fileb"}
	filec := map[string]interface{}{"type": "file", "name": "filec"}
	files := []map[string]interface{}{filea, foobar, foobarbaz, dela, delb, fileb, filec, foo}
	s.SortFilesToSent(files)
	expected := []map[string]interface{}{delb, dela, foo, foobar, foobarbaz, filea, fileb, filec}
	assert.Equal(t, expected, files)
}

func TestSharingDir(t *testing.T) {
	s := Sharing{
		SID: uuidv4(),
		Rules: []Rule{
			{
				Title:   "Test sharing dir",
				DocType: consts.Files,
				Values:  []string{uuidv4()},
			},
		},
	}
	d1, err := s.CreateDirForSharing(inst, &s.Rules[0])
	assert.NoError(t, err)

	d2, err := s.GetSharingDir(inst)
	assert.NoError(t, err)
	if assert.NotNil(t, d2) {
		assert.Equal(t, d1.DocID, d2.DocID)
		assert.Equal(t, "Test sharing dir", d2.DocName)
		assert.Equal(t, "/Tree Shared with me/Test sharing dir", d2.Fullpath)
		assert.Len(t, d2.ReferencedBy, 1)
		assert.Equal(t, consts.Sharings, d2.ReferencedBy[0].Type)
		assert.Equal(t, s.SID, d2.ReferencedBy[0].ID)
	}

	err = s.RemoveSharingDir(inst)
	assert.NoError(t, err)

	key := []string{consts.Sharings, s.SID}
	end := []string{key[0], key[1], couchdb.MaxString}
	req := &couchdb.ViewRequest{
		StartKey:    key,
		EndKey:      end,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	err = couchdb.ExecView(inst, couchdb.FilesReferencedByView, req, &res)
	assert.NoError(t, err)
	assert.Len(t, res.Rows, 0)
}

func TestCreateDir(t *testing.T) {
	s := Sharing{
		SID: uuidv4(),
		Rules: []Rule{
			{
				Title:   "Test create dir",
				DocType: consts.Files,
				Values:  []string{uuidv4()},
			},
		},
	}

	idFoo := uuidv4()
	target := map[string]interface{}{
		"_id":  idFoo,
		"_rev": "1-6b501ca58928b02b90c430fd730e8b17",
		"_revisions": map[string]interface{}{
			"start": float64(1),
			"ids": []interface{}{
				"6b501ca58928b02b90c430fd730e8b17",
			},
		},
		"name": "Foo",
	}
	assert.NoError(t, s.CreateDir(inst, target))
	dir, err := inst.VFS().DirByID(idFoo)
	assert.NoError(t, err)
	if assert.NotNil(t, dir) {
		assert.Equal(t, idFoo, dir.DocID)
		assert.Equal(t, target["_rev"], dir.DocRev)
		assert.Equal(t, "Foo", dir.DocName)
		assert.Equal(t, "/Tree Shared with me/Test create dir/Foo", dir.Fullpath)
	}

	idBar := uuidv4()
	target = map[string]interface{}{
		"_id":  idBar,
		"_rev": "4-2ee767305024673cfb3f5af037cd2729",
		"_revisions": map[string]interface{}{
			"start": float64(4),
			"ids": []interface{}{
				"2ee767305024673cfb3f5af037cd2729",
				"753875d51501a6b1883a9d62b4d33f91",
			},
		},
		"dir_id":     idFoo,
		"name":       "Bar",
		"created_at": "2018-04-13T15:06:00.012345678+01:00",
		"updated_at": "2018-04-13T15:08:32.581420274+01:00",
		"tags":       []interface{}{"qux", "courge"},
	}
	assert.NoError(t, s.CreateDir(inst, target))
	dir, err = inst.VFS().DirByID(idBar)
	assert.NoError(t, err)
	if assert.NotNil(t, dir) {
		assert.Equal(t, idBar, dir.DocID)
		assert.Equal(t, target["_rev"], dir.DocRev)
		assert.Equal(t, "Bar", dir.DocName)
		assert.Equal(t, "/Tree Shared with me/Test create dir/Foo/Bar", dir.Fullpath)
		assert.Equal(t, "2018-04-13 15:06:00.012345678 +0100 +0100", dir.CreatedAt.String())
		assert.Equal(t, "2018-04-13 15:08:32.581420274 +0100 +0100", dir.UpdatedAt.String())
		assert.Equal(t, []string{"qux", "courge"}, dir.Tags)
	}
}

func TestUpdateDir(t *testing.T) {
	s := Sharing{
		SID: uuidv4(),
		Rules: []Rule{
			{
				Title:   "Test update dir",
				DocType: consts.Files,
				Values:  []string{uuidv4()},
			},
		},
	}

	idFoo := uuidv4()
	target := map[string]interface{}{
		"_id":  idFoo,
		"_rev": "1-4fff5291a41bf1f493460d2070694c5a",
		"_revisions": map[string]interface{}{
			"start": float64(1),
			"ids": []interface{}{
				"4fff5291a41bf1f493460d2070694c5a",
			},
		},
		"name":       "Foo",
		"created_at": "2018-04-13T15:06:00.012345678+01:00",
		"updated_at": "2018-04-13T15:08:32.581420274+01:00",
		"tags":       []interface{}{"qux", "courge"},
	}
	assert.NoError(t, s.CreateDir(inst, target))
	dir, err := inst.VFS().DirByID(idFoo)
	assert.NoError(t, err)
	if assert.NotNil(t, dir) {
		assert.Equal(t, idFoo, dir.DocID)
		assert.Equal(t, target["_rev"], dir.DocRev)
		assert.Equal(t, "Foo", dir.DocName)
		assert.Equal(t, "/Tree Shared with me/Test update dir/Foo", dir.Fullpath)
	}

	target = map[string]interface{}{
		"_id":  idFoo,
		"_rev": "2-96c72d35f3ad802484a61df501b0f1bb",
		"_revisions": map[string]interface{}{
			"start": float64(2),
			"ids": []interface{}{
				"96c72d35f3ad802484a61df501b0f1bb",
				"4fff5291a41bf1f493460d2070694c5a",
			},
		},
		"name":       "Foo",
		"created_at": "2018-04-13T15:06:00.012345678+01:00",
		"updated_at": "2018-04-13T15:10:57.364765745+01:00",
		"tags":       []interface{}{"quux", "courge"},
	}
	var ref SharedRef
	err = couchdb.GetDoc(inst, consts.Shared, consts.Files+"/"+idFoo, &ref)
	assert.NoError(t, err)
	assert.NoError(t, s.UpdateDir(inst, target, dir, &ref))
	dir, err = inst.VFS().DirByID(idFoo)
	assert.NoError(t, err)
	if assert.NotNil(t, dir) {
		assert.Equal(t, idFoo, dir.DocID)
		assert.Equal(t, target["_rev"], dir.DocRev)
		assert.Equal(t, "Foo", dir.DocName)
		assert.Equal(t, "/Tree Shared with me/Test update dir/Foo", dir.Fullpath)
		assert.Equal(t, "2018-04-13 15:06:00.012345678 +0100 +0100", dir.CreatedAt.String())
		assert.Equal(t, "2018-04-13 15:10:57.364765745 +0100 +0100", dir.UpdatedAt.String())
		assert.Equal(t, []string{"quux", "courge"}, dir.Tags)
	}
}
