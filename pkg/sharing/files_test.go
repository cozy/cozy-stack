package sharing

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
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
	assert.NoError(t, s.CreateDirForSharing(inst, &s.Rules[0]))

	dir, err := s.GetSharingDir(inst)
	assert.NoError(t, err)
	if assert.NotNil(t, dir) {
		assert.Equal(t, "Test sharing dir", dir.DocName)
		assert.Equal(t, "/Tree Shared with me/Test sharing dir", dir.Fullpath)
		assert.Len(t, dir.ReferencedBy, 1)
		assert.Equal(t, consts.Sharings, dir.ReferencedBy[0].Type)
		assert.Equal(t, s.SID, dir.ReferencedBy[0].ID)
	}
}
