package sharing

import (
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/tests/testutils"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

var inst *instance.Instance

const testDoctype = "io.cozy.sharing.tests"

func TestRevGeneration(t *testing.T) {
	assert.Equal(t, 1, RevGeneration("1-aaa"))
	assert.Equal(t, 3, RevGeneration("3-123"))
	assert.Equal(t, 10, RevGeneration("10-1f2"))
}

func TestComputePossibleAncestors(t *testing.T) {
	wants := []string{"2-b"}
	haves := []string{"1-a", "2-a", "3-a"}
	pas := computePossibleAncestors(wants, haves)
	expected := []string{"1-a"}
	assert.Equal(t, expected, pas)

	wants = []string{"2-b", "2-c", "4-b"}
	haves = []string{"1-a", "2-a", "3-a", "4-a"}
	pas = computePossibleAncestors(wants, haves)
	expected = []string{"1-a", "3-a"}
	assert.Equal(t, expected, pas)

	wants = []string{"5-b"}
	haves = []string{"1-a", "2-a", "3-a"}
	pas = computePossibleAncestors(wants, haves)
	expected = []string{"3-a"}
	assert.Equal(t, expected, pas)
}

func uuidv4() string {
	id, _ := uuid.NewV4()
	return id.String()
}

func createSharedRef(t *testing.T) {
	ref := SharedRef{
		SID:       testDoctype + "/" + uuidv4(),
		Revisions: []string{"1-aaa"},
	}
	err := couchdb.CreateNamedDocWithDB(inst, &ref)
	assert.NoError(t, err)
}

func TestSequenceNumber(t *testing.T) {
	nb := 5
	for i := 0; i < nb; i++ {
		createSharedRef(t)
	}
	s := &Sharing{SID: uuidv4(), Members: []Member{
		{Status: MemberStatusOwner, Name: "Alice"},
		{Status: MemberStatusReady, Name: "Bob"},
	}}
	m := &s.Members[1]

	rid, err := s.replicationID(m)
	assert.NoError(t, err)
	assert.Equal(t, "sharing-"+s.SID+"-1", rid)

	seq, err := s.getLastSeqNumber(inst, m)
	assert.NoError(t, err)
	assert.Empty(t, seq)
	_, seq, err = s.callChangesFeed(inst, seq)
	assert.NoError(t, err)
	assert.NotEmpty(t, seq)
	assert.Equal(t, nb, RevGeneration(seq))
	err = s.UpdateLastSequenceNumber(inst, m, seq)
	assert.NoError(t, err)
	seq2, err := s.getLastSeqNumber(inst, m)
	assert.NoError(t, err)
	assert.Equal(t, seq, seq2)

	err = s.UpdateLastSequenceNumber(inst, m, "2-abc")
	assert.NoError(t, err)
	seq3, err := s.getLastSeqNumber(inst, m)
	assert.NoError(t, err)
	assert.Equal(t, seq, seq3)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "sharing_test_repl")
	inst = setup.GetTestInstance()
	os.Exit(setup.Run())
}
