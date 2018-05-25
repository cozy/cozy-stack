package sharing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRevGeneration(t *testing.T) {
	assert.Equal(t, 1, RevGeneration("1-aaa"))
	assert.Equal(t, 3, RevGeneration("3-123"))
	assert.Equal(t, 10, RevGeneration("10-1f2"))
}

func TestRevsStructToChain(t *testing.T) {
	input := map[string]interface{}{
		"start": float64(3),
		"ids":   []interface{}{"ccc", "bbb", "aaa"},
	}
	chain := revsStructToChain(input)
	expected := []string{"1-aaa", "2-bbb", "3-ccc"}
	assert.Equal(t, expected, chain)
}

func TestRevsChainToStruct(t *testing.T) {
	slice := []string{"2-aaa", "3-bbb", "4-ccc"}
	revs := revsChainToStruct(slice)
	assert.Equal(t, 4, revs.Start)
	assert.Equal(t, []string{"ccc", "bbb", "aaa"}, revs.Ids)
}

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
	assert.Equal(t, tree, tree.Find("1-aaa"))
	assert.Equal(t, &twoA, tree.Find("2-baa"))
	assert.Equal(t, &twoB, tree.Find("2-bbb"))
	assert.Equal(t, &three, tree.Find("3-ccc"))
	assert.Equal(t, (*RevsTree)(nil), tree.Find("4-ddd"))
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
