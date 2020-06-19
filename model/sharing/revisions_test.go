package sharing

import (
	"fmt"
	"testing"
	"unicode"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/assert"
)

func TestRevsTreeGeneration(t *testing.T) {
	tree := &RevsTree{Rev: "1-aaa"}
	assert.Equal(t, 1, tree.Generation())
	twoA := RevsTree{Rev: "2-baa"}
	twoB := RevsTree{Rev: "2-bbb"}
	twoB.Branches = []RevsTree{{Rev: "3-ccc"}}
	tree.Branches = []RevsTree{twoA, twoB}
	assert.Equal(t, 3, tree.Generation())
}

func TestRevsTreeFind(t *testing.T) {
	tree := &RevsTree{Rev: "1-aaa"}
	twoA := RevsTree{Rev: "2-baa"}
	twoB := RevsTree{Rev: "2-bbb"}
	three := RevsTree{Rev: "3-ccc"}
	twoB.Branches = []RevsTree{three}
	tree.Branches = []RevsTree{twoA, twoB}
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
	assert.Equal(t, (*RevsTree)(nil), actual)
}

func TestRevsTreeAdd(t *testing.T) {
	tree := &RevsTree{Rev: "1-aaa"}
	twoA := RevsTree{Rev: "2-baa"}
	twoB := RevsTree{Rev: "2-bbb"}
	three := RevsTree{Rev: "3-ccc"}
	twoB.Branches = []RevsTree{three}
	tree.Branches = []RevsTree{twoA, twoB}
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
}

func TestRevsTreeInsertAfter(t *testing.T) {
	tree := &RevsTree{Rev: "1-aaa"}
	twoA := RevsTree{Rev: "2-baa"}
	twoB := RevsTree{Rev: "2-bbb"}
	three := RevsTree{Rev: "3-ccc"}
	twoB.Branches = []RevsTree{three}
	tree.Branches = []RevsTree{twoA, twoB}
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
	tree := &RevsTree{Rev: "1-aaa"}
	parent := tree.Rev
	for i := 2; i < 2*MaxDepth; i++ {
		next := fmt.Sprintf("%d-bbb", i)
		tree.InsertAfter(next, parent)
		parent = next
	}
	_, depth := tree.Find(parent)
	assert.Equal(t, depth, MaxDepth)
}

func TestRevsTreeInsertChain(t *testing.T) {
	tree := &RevsTree{Rev: "1-aaa"}
	twoA := RevsTree{Rev: "2-baa"}
	twoB := RevsTree{Rev: "2-bbb"}
	three := RevsTree{Rev: "3-ccc"}
	twoB.Branches = []RevsTree{three}
	tree.Branches = []RevsTree{twoA, twoB}
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
	tree := &RevsTree{Rev: "2-bbb"}
	three := RevsTree{Rev: "3-ccc"}
	tree.Branches = []RevsTree{three}
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

func TestRevGeneration(t *testing.T) {
	assert.Equal(t, 1, RevGeneration("1-aaa"))
	assert.Equal(t, 3, RevGeneration("3-123"))
	assert.Equal(t, 10, RevGeneration("10-1f2"))
}

func TestRevsStructToChain(t *testing.T) {
	input := RevsStruct{
		Start: 3,
		IDs:   []string{"ccc", "bbb", "aaa"},
	}
	chain := revsStructToChain(input)
	expected := []string{"1-aaa", "2-bbb", "3-ccc"}
	assert.Equal(t, expected, chain)
}

func TestRevsChainToStruct(t *testing.T) {
	slice := []string{"2-aaa", "3-bbb", "4-ccc"}
	revs := revsChainToStruct(slice)
	assert.Equal(t, 4, revs.Start)
	assert.Equal(t, []string{"ccc", "bbb", "aaa"}, revs.IDs)
}

func TestDetectConflicts(t *testing.T) {
	chain := []string{"1-aaa", "2-bbb", "3-ccc"}
	assert.Equal(t, NoConflict, detectConflict("1-aaa", chain))
	assert.Equal(t, NoConflict, detectConflict("2-bbb", chain))
	assert.Equal(t, NoConflict, detectConflict("3-ccc", chain))
	assert.Equal(t, WonConflict, detectConflict("2-ddd", chain))
	assert.Equal(t, WonConflict, detectConflict("3-abc", chain))
	assert.Equal(t, LostConflict, detectConflict("4-eee", chain))
	assert.Equal(t, LostConflict, detectConflict("3-def", chain))
}

func TestMixupChainToResolveConflict(t *testing.T) {
	chain := []string{"1-aaa", "2-bbb", "3-ccc", "4-ddd", "5-eee"}
	altered := MixupChainToResolveConflict("3-abc", chain)
	expected := []string{"3-abc", "4-ddd", "5-eee"}
	assert.Equal(t, expected, altered)
}

func TestAddMissingRevsToChain(t *testing.T) {
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

	tree := &RevsTree{Rev: rev1}
	ref := &SharedRef{
		SID:       "io.cozy.test/" + doc.ID(),
		Revisions: tree,
	}

	chain := []string{"3-ccc", "4-ddd"}
	newChain, err := addMissingRevsToChain(inst, ref, chain)
	assert.NoError(t, err)
	expectedChain := []string{rev2, chain[0], chain[1]}
	assert.Equal(t, expectedChain, newChain)
}

func TestIndexerIncrementRevisions(t *testing.T) {
	indexer := &sharingIndexer{
		bulkRevs: &bulkRevs{
			Rev: "3-bf26bb2d42b0abf6a715ccf949d8e5f4",
			Revisions: RevsStruct{
				Start: 3,
				IDs: []string{
					"bf26bb2d42b0abf6a715ccf949d8e5f4",
					"031e47856210360b44db86669ee83cd1",
				},
			},
		},
	}
	indexer.IncrementRevision()
	assert.Equal(t, 4, indexer.bulkRevs.Revisions.Start)
	gen := RevGeneration(indexer.bulkRevs.Rev)
	assert.Equal(t, 4, gen)
	assert.Len(t, indexer.bulkRevs.Revisions.IDs, 3)
	rev := fmt.Sprintf("%d-%s", gen, indexer.bulkRevs.Revisions.IDs[0])
	assert.Equal(t, indexer.bulkRevs.Rev, rev)
}

func TestIndexerStashRevision(t *testing.T) {
	indexer := &sharingIndexer{
		bulkRevs: &bulkRevs{
			Rev: "4-9a8d25e7fc9834dc85a252ca8c11723d",
			Revisions: RevsStruct{
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
	assert.Equal(t, "3-ac12db6cd9bd8190f98b2bfed6522d1f", indexer.bulkRevs.Rev)
	assert.Equal(t, 3, indexer.bulkRevs.Revisions.Start)
	assert.Len(t, indexer.bulkRevs.Revisions.IDs, 2)
	assert.Equal(t, "ac12db6cd9bd8190f98b2bfed6522d1f", indexer.bulkRevs.Revisions.IDs[0])
	assert.Equal(t, "9822dfe81c0e30da3d7b4213f0dcca2a", indexer.bulkRevs.Revisions.IDs[1])

	indexer.UnstashRevision(stash)
	assert.Equal(t, "4-9a8d25e7fc9834dc85a252ca8c11723d", indexer.bulkRevs.Rev)
	assert.Equal(t, 4, indexer.bulkRevs.Revisions.Start)
	assert.Len(t, indexer.bulkRevs.Revisions.IDs, 3)
	assert.Equal(t, "9a8d25e7fc9834dc85a252ca8c11723d", indexer.bulkRevs.Revisions.IDs[0])
	assert.Equal(t, "ac12db6cd9bd8190f98b2bfed6522d1f", indexer.bulkRevs.Revisions.IDs[1])
	assert.Equal(t, "9822dfe81c0e30da3d7b4213f0dcca2a", indexer.bulkRevs.Revisions.IDs[2])

	indexer.bulkRevs = &bulkRevs{
		Rev: "2-a61b005843648f5822cc44e1e586c29c",
		Revisions: RevsStruct{
			Start: 2,
			IDs: []string{
				"a61b005843648f5822cc44e1e586c29c",
				"7f0065d977dcfd49bbcd77f8630f185b",
			},
		},
	}
	stash = indexer.StashRevision(false)
	assert.Empty(t, stash)
	assert.Nil(t, indexer.bulkRevs)
	indexer.UnstashRevision(stash)
	assert.Nil(t, indexer.bulkRevs)

	indexer.bulkRevs = &bulkRevs{
		Rev: "2-a61b005843648f5822cc44e1e586c29c",
		Revisions: RevsStruct{
			Start: 2,
			IDs: []string{
				"a61b005843648f5822cc44e1e586c29c",
			},
		},
	}
	stash = indexer.StashRevision(true)
	assert.Empty(t, stash)
	assert.Nil(t, indexer.bulkRevs)
	indexer.UnstashRevision(stash)
	assert.Nil(t, indexer.bulkRevs)
}

func TestIndexerCreateBogusPrevRev(t *testing.T) {
	indexer := &sharingIndexer{
		bulkRevs: &bulkRevs{
			Rev: "3-bf26bb2d42b0abf6a715ccf949d8e5f4",
			Revisions: RevsStruct{
				Start: 3,
				IDs: []string{
					"bf26bb2d42b0abf6a715ccf949d8e5f4",
				},
			},
		},
	}
	indexer.CreateBogusPrevRev()
	assert.Equal(t, 3, indexer.bulkRevs.Revisions.Start)
	gen := RevGeneration(indexer.bulkRevs.Rev)
	assert.Equal(t, 3, gen)
	assert.Len(t, indexer.bulkRevs.Revisions.IDs, 2)
	rev := fmt.Sprintf("%d-%s", gen, indexer.bulkRevs.Revisions.IDs[0])
	assert.Equal(t, indexer.bulkRevs.Rev, rev)
}

func TestConflictID(t *testing.T) {
	id := "d9dfd293577eea9f6d29d140259fa71d"
	rev := "3-bf26bb2d42b0abf6a715ccf949d8e5f4"
	xored := conflictID(id, rev)
	for _, c := range xored {
		assert.True(t, unicode.IsDigit(c) || unicode.IsLetter(c))
	}
}
