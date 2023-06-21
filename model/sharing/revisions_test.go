package sharing_test

import (
	"fmt"
	"testing"
	"unicode"

	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/revision"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevsTreeGeneration(t *testing.T) {
	tree := &sharing.RevsTree{Rev: "1-aaa"}
	assert.Equal(t, 1, tree.Generation())
	twoA := sharing.RevsTree{Rev: "2-baa"}
	twoB := sharing.RevsTree{Rev: "2-bbb"}
	twoB.Branches = []sharing.RevsTree{{Rev: "3-ccc"}}
	tree.Branches = []sharing.RevsTree{twoA, twoB}
	assert.Equal(t, 3, tree.Generation())
}

func TestRevsTreeFind(t *testing.T) {
	tree := &sharing.RevsTree{Rev: "1-aaa"}
	twoA := sharing.RevsTree{Rev: "2-baa"}
	twoB := sharing.RevsTree{Rev: "2-bbb"}
	three := sharing.RevsTree{Rev: "3-ccc"}
	twoB.Branches = []sharing.RevsTree{three}
	tree.Branches = []sharing.RevsTree{twoA, twoB}
	actual, depth := tree.Find("1-aaa")
	assert.Equal(t, tree, actual)
	assert.Equal(t, 1, depth)
	actual, depth = tree.Find("2-baa")
	assert.Equal(t, &twoA, actual)
	assert.Equal(t, 2, depth)
	actual, depth = tree.Find("2-bbb")
	assert.Equal(t, &twoB, actual)
	assert.Equal(t, 2, depth)
	actual, depth = tree.Find("3-ccc")
	assert.Equal(t, &three, actual)
	assert.Equal(t, 3, depth)
	actual, _ = tree.Find("4-ddd")
	assert.Equal(t, (*sharing.RevsTree)(nil), actual)
}

func TestRevsTreeAdd(t *testing.T) {
	tree := &sharing.RevsTree{Rev: "1-aaa"}
	twoA := sharing.RevsTree{Rev: "2-baa"}
	twoB := sharing.RevsTree{Rev: "2-bbb"}
	three := sharing.RevsTree{Rev: "3-ccc"}
	twoB.Branches = []sharing.RevsTree{three}
	tree.Branches = []sharing.RevsTree{twoA, twoB}
	ret := tree.Add("3-caa")
	assert.Equal(t, "3-caa", ret.Rev)
	ret = tree.Add("4-daa")
	assert.Equal(t, "4-daa", ret.Rev)
	ret = tree.Add("5-eaa")
	assert.Equal(t, "5-eaa", ret.Rev)
	assert.Equal(t, tree.Rev, "1-aaa")
	assert.Len(t, tree.Branches, 2)
	sub := tree.Branches[0]
	assert.Equal(t, sub.Rev, "2-baa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "3-caa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "4-daa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "5-eaa")
	assert.Len(t, sub.Branches, 0)
	sub = tree.Branches[1]
	assert.Equal(t, sub.Rev, "2-bbb")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "3-ccc")
	assert.Len(t, sub.Branches, 0)

	tree = &sharing.RevsTree{Rev: "2-bbb"}
	ret = tree.Add("3-ccc")
	assert.Equal(t, "3-ccc", ret.Rev)
	ret = tree.Add("1-aaa")
	assert.Equal(t, "1-aaa", ret.Rev)
	assert.Equal(t, "1-aaa", tree.Rev)
	require.Len(t, tree.Branches, 1)
	sub = tree.Branches[0]
	assert.Equal(t, "2-bbb", sub.Rev)
	require.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, "3-ccc", sub.Rev)
	require.Len(t, sub.Branches, 0)
}

func TestRevsTreeInsertAfter(t *testing.T) {
	tree := &sharing.RevsTree{Rev: "1-aaa"}
	twoA := sharing.RevsTree{Rev: "2-baa"}
	twoB := sharing.RevsTree{Rev: "2-bbb"}
	three := sharing.RevsTree{Rev: "3-ccc"}
	twoB.Branches = []sharing.RevsTree{three}
	tree.Branches = []sharing.RevsTree{twoA, twoB}
	tree.InsertAfter("4-ddd", "3-ccc")
	tree.InsertAfter("4-daa", "3-caa")
	tree.InsertAfter("3-caa", "2-baa")
	tree.InsertAfter("5-eaa", "4-daa")
	assert.Equal(t, tree.Rev, "1-aaa")
	assert.Len(t, tree.Branches, 2)
	sub := tree.Branches[0]
	assert.Equal(t, sub.Rev, "2-baa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "3-caa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "4-daa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "5-eaa")
	assert.Len(t, sub.Branches, 0)
	sub = tree.Branches[1]
	assert.Equal(t, sub.Rev, "2-bbb")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "3-ccc")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "4-ddd")
	assert.Len(t, sub.Branches, 0)
}

func TestRevsTreeInsertAfterMaxDepth(t *testing.T) {
	tree := &sharing.RevsTree{Rev: "1-aaa"}
	parent := tree.Rev
	for i := 2; i < 2*sharing.MaxDepth; i++ {
		next := fmt.Sprintf("%d-bbb", i)
		tree.InsertAfter(next, parent)
		parent = next
	}
	_, depth := tree.Find(parent)
	assert.Equal(t, depth, sharing.MaxDepth)
}

func TestRevsTreeInsertChain(t *testing.T) {
	tree := &sharing.RevsTree{Rev: "1-aaa"}
	twoA := sharing.RevsTree{Rev: "2-baa"}
	twoB := sharing.RevsTree{Rev: "2-bbb"}
	three := sharing.RevsTree{Rev: "3-ccc"}
	twoB.Branches = []sharing.RevsTree{three}
	tree.Branches = []sharing.RevsTree{twoA, twoB}
	tree.InsertChain([]string{"2-baa", "3-caa", "4-daa"})
	tree.InsertChain([]string{"5-eaa"})
	tree.InsertChain([]string{"2-bbb", "3-ccc", "4-ddd"})
	assert.Equal(t, tree.Rev, "1-aaa")
	assert.Len(t, tree.Branches, 2)
	sub := tree.Branches[0]
	assert.Equal(t, sub.Rev, "2-baa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "3-caa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "4-daa")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "5-eaa")
	assert.Len(t, sub.Branches, 0)
	sub = tree.Branches[1]
	assert.Equal(t, sub.Rev, "2-bbb")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "3-ccc")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "4-ddd")
	assert.Len(t, sub.Branches, 0)
}

func TestRevsTreeInsertChainStartingBefore(t *testing.T) {
	tree := &sharing.RevsTree{Rev: "2-bbb"}
	three := sharing.RevsTree{Rev: "3-ccc"}
	tree.Branches = []sharing.RevsTree{three}
	tree.InsertChain([]string{"1-aaa", "2-bbb", "3-ccc", "4-ddd"})
	assert.Equal(t, tree.Rev, "2-bbb")
	assert.Len(t, tree.Branches, 1)
	sub := tree.Branches[0]
	assert.Equal(t, sub.Rev, "3-ccc")
	assert.Len(t, sub.Branches, 1)
	sub = sub.Branches[0]
	assert.Equal(t, sub.Rev, "4-ddd")
	assert.Len(t, sub.Branches, 0)
}

func TestRevsStructToChain(t *testing.T) {
	input := sharing.RevsStruct{
		Start: 3,
		IDs:   []string{"ccc", "bbb", "aaa"},
	}
	chain := sharing.RevsStructToChain(input)
	expected := []string{"1-aaa", "2-bbb", "3-ccc"}
	assert.Equal(t, expected, chain)
}

func TestRevsChainToStruct(t *testing.T) {
	slice := []string{"2-aaa", "3-bbb", "4-ccc"}
	revs := sharing.RevsChainToStruct(slice)
	assert.Equal(t, 4, revs.Start)
	assert.Equal(t, []string{"ccc", "bbb", "aaa"}, revs.IDs)
}

func TestDetectConflicts(t *testing.T) {
	chain := []string{"1-aaa", "2-bbb", "3-ccc"}
	assert.Equal(t, sharing.NoConflict, sharing.DetectConflict("1-aaa", chain))
	assert.Equal(t, sharing.NoConflict, sharing.DetectConflict("2-bbb", chain))
	assert.Equal(t, sharing.NoConflict, sharing.DetectConflict("3-ccc", chain))
	assert.Equal(t, sharing.WonConflict, sharing.DetectConflict("2-ddd", chain))
	assert.Equal(t, sharing.WonConflict, sharing.DetectConflict("3-abc", chain))
	assert.Equal(t, sharing.LostConflict, sharing.DetectConflict("4-eee", chain))
	assert.Equal(t, sharing.LostConflict, sharing.DetectConflict("3-def", chain))
}

func TestMixupChainToResolveConflict(t *testing.T) {
	chain := []string{"1-aaa", "2-bbb", "3-ccc", "4-ddd", "5-eee"}
	altered := sharing.MixupChainToResolveConflict("3-abc", chain)
	expected := []string{"3-abc", "4-ddd", "5-eee"}
	assert.Equal(t, expected, altered)
}

func TestAddMissingRevsToChain(t *testing.T) {
	if testing.Short() {
		t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()

	doc := &couchdb.JSONDoc{
		Type: "io.cozy.test",
		M: map[string]interface{}{
			"test": "1",
		},
	}
	assert.NoError(t, couchdb.CreateDoc(inst, doc))
	rev1 := doc.Rev()
	assert.NoError(t, couchdb.UpdateDoc(inst, doc))
	rev2 := doc.Rev()

	tree := &sharing.RevsTree{Rev: rev1}
	ref := &sharing.SharedRef{
		SID:       "io.cozy.test/" + doc.ID(),
		Revisions: tree,
	}

	chain := []string{"3-ccc", "4-ddd"}
	newChain, err := sharing.AddMissingRevsToChain(inst, ref, chain)
	assert.NoError(t, err)
	expectedChain := []string{rev2, chain[0], chain[1]}
	assert.Equal(t, expectedChain, newChain)
}

func TestIndexerIncrementRevisions(t *testing.T) {
	indexer := &sharing.SharingIndexer{
		BulkRevs: &sharing.BulkRevs{
			Rev: "3-bf26bb2d42b0abf6a715ccf949d8e5f4",
			Revisions: sharing.RevsStruct{
				Start: 3,
				IDs: []string{
					"bf26bb2d42b0abf6a715ccf949d8e5f4",
					"031e47856210360b44db86669ee83cd1",
				},
			},
		},
	}
	indexer.IncrementRevision()
	assert.Equal(t, 4, indexer.BulkRevs.Revisions.Start)
	gen := revision.Generation(indexer.BulkRevs.Rev)
	assert.Equal(t, 4, gen)
	assert.Len(t, indexer.BulkRevs.Revisions.IDs, 3)
	rev := fmt.Sprintf("%d-%s", gen, indexer.BulkRevs.Revisions.IDs[0])
	assert.Equal(t, indexer.BulkRevs.Rev, rev)
}

func TestIndexerStashRevision(t *testing.T) {
	indexer := &sharing.SharingIndexer{
		BulkRevs: &sharing.BulkRevs{
			Rev: "4-9a8d25e7fc9834dc85a252ca8c11723d",
			Revisions: sharing.RevsStruct{
				Start: 4,
				IDs: []string{
					"9a8d25e7fc9834dc85a252ca8c11723d",
					"ac12db6cd9bd8190f98b2bfed6522d1f",
					"9822dfe81c0e30da3d7b4213f0dcca2a",
				},
			},
		},
	}

	stash := indexer.StashRevision(false)
	assert.Equal(t, "9a8d25e7fc9834dc85a252ca8c11723d", stash)
	assert.Equal(t, "3-ac12db6cd9bd8190f98b2bfed6522d1f", indexer.BulkRevs.Rev)
	assert.Equal(t, 3, indexer.BulkRevs.Revisions.Start)
	assert.Len(t, indexer.BulkRevs.Revisions.IDs, 2)
	assert.Equal(t, "ac12db6cd9bd8190f98b2bfed6522d1f", indexer.BulkRevs.Revisions.IDs[0])
	assert.Equal(t, "9822dfe81c0e30da3d7b4213f0dcca2a", indexer.BulkRevs.Revisions.IDs[1])

	indexer.UnstashRevision(stash)
	assert.Equal(t, "4-9a8d25e7fc9834dc85a252ca8c11723d", indexer.BulkRevs.Rev)
	assert.Equal(t, 4, indexer.BulkRevs.Revisions.Start)
	assert.Len(t, indexer.BulkRevs.Revisions.IDs, 3)
	assert.Equal(t, "9a8d25e7fc9834dc85a252ca8c11723d", indexer.BulkRevs.Revisions.IDs[0])
	assert.Equal(t, "ac12db6cd9bd8190f98b2bfed6522d1f", indexer.BulkRevs.Revisions.IDs[1])
	assert.Equal(t, "9822dfe81c0e30da3d7b4213f0dcca2a", indexer.BulkRevs.Revisions.IDs[2])

	indexer.BulkRevs = &sharing.BulkRevs{
		Rev: "2-a61b005843648f5822cc44e1e586c29c",
		Revisions: sharing.RevsStruct{
			Start: 2,
			IDs: []string{
				"a61b005843648f5822cc44e1e586c29c",
				"7f0065d977dcfd49bbcd77f8630f185b",
			},
		},
	}
	stash = indexer.StashRevision(false)
	assert.Empty(t, stash)
	assert.Nil(t, indexer.BulkRevs)
	indexer.UnstashRevision(stash)
	assert.Nil(t, indexer.BulkRevs)

	indexer.BulkRevs = &sharing.BulkRevs{
		Rev: "2-a61b005843648f5822cc44e1e586c29c",
		Revisions: sharing.RevsStruct{
			Start: 2,
			IDs: []string{
				"a61b005843648f5822cc44e1e586c29c",
			},
		},
	}
	stash = indexer.StashRevision(true)
	assert.Empty(t, stash)
	assert.Nil(t, indexer.BulkRevs)
	indexer.UnstashRevision(stash)
	assert.Nil(t, indexer.BulkRevs)
}

func TestIndexerCreateBogusPrevRev(t *testing.T) {
	indexer := &sharing.SharingIndexer{
		BulkRevs: &sharing.BulkRevs{
			Rev: "3-bf26bb2d42b0abf6a715ccf949d8e5f4",
			Revisions: sharing.RevsStruct{
				Start: 3,
				IDs: []string{
					"bf26bb2d42b0abf6a715ccf949d8e5f4",
				},
			},
		},
	}
	indexer.CreateBogusPrevRev()
	assert.Equal(t, 3, indexer.BulkRevs.Revisions.Start)
	gen := revision.Generation(indexer.BulkRevs.Rev)
	assert.Equal(t, 3, gen)
	assert.Len(t, indexer.BulkRevs.Revisions.IDs, 2)
	rev := fmt.Sprintf("%d-%s", gen, indexer.BulkRevs.Revisions.IDs[0])
	assert.Equal(t, indexer.BulkRevs.Rev, rev)
}

func TestConflictID(t *testing.T) {
	id := "d9dfd293577eea9f6d29d140259fa71d"
	rev := "3-bf26bb2d42b0abf6a715ccf949d8e5f4"
	xored := sharing.ConflictID(id, rev)
	for _, c := range xored {
		assert.True(t, unicode.IsDigit(c) || unicode.IsLetter(c))
	}
}
