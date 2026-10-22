package driveactivity

import (
	"context"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useRedis(t *testing.T) store {
	t.Helper()
	if testing.Short() {
		t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
	}
	opt, err := redis.ParseURL("redis://localhost:6379/15")
	require.NoError(t, err)
	cli := redis.NewClient(opt)
	require.NoError(t, cli.Ping(context.Background()).Err())
	previous := config.GetConfig().SessionStorage
	config.GetConfig().SessionStorage = cli
	t.Cleanup(func() { config.GetConfig().SessionStorage = previous })
	st, _ := getStore()
	return st
}

func TestStore(t *testing.T) {
	config.UseTestFile(t)
	st := useRedis(t)
	drive := "test/" + utils.RandomString(10)
	add := func(c change, start string) (string, bool) {
		t.Helper()
		w, opened, err := st.add(drive, c, start, time.Second)
		require.NoError(t, err)
		return w, opened
	}

	w, opened := add(change{"alice@example.net", "dir1", Created, "a.txt"}, "w1")
	assert.Equal(t, "w1", w)
	assert.True(t, opened)

	w, opened = add(change{"alice@example.net", "dir1", Created, "a.txt"}, "w2")
	assert.Equal(t, "w1", w)
	assert.False(t, opened)
	for _, name := range []string{"b.txt", "c.txt", "d.txt", "e.txt", "f.txt"} {
		add(change{"alice@example.net", "dir1", Updated, name}, "w2")
	}
	add(change{"bob@example.net", "dir1", Deleted, "z.txt"}, "w2")

	counts, err := st.take(drive, "w1")
	require.NoError(t, err)
	require.Len(t, counts, 2)
	alice := counts[key{"alice@example.net", "dir1"}]
	assert.Equal(t, 2, alice.created)
	assert.Equal(t, 5, alice.updated)
	assert.Len(t, alice.samples, maxSamples)
	assert.Equal(t, &tally{deleted: 1, samples: []string{"z.txt"}}, counts[key{"bob@example.net", "dir1"}])

	again, err := st.take(drive, "w1")
	require.NoError(t, err)
	assert.Equal(t, len(counts), len(again), "a flush that runs again gets the same counts")

	time.Sleep(1100 * time.Millisecond)
	w, opened = add(change{"alice@example.net", "dir2", Created, "g.txt"}, "w3")
	assert.Equal(t, "w3", w, "a change after the window opens the next one")
	assert.True(t, opened)

	require.NoError(t, st.drop(drive, "w1"))
	dropped, err := st.take(drive, "w1")
	require.NoError(t, err)
	assert.Empty(t, dropped)

	next, err := st.take(drive, "w3")
	require.NoError(t, err)
	assert.Equal(t, map[key]*tally{
		{"alice@example.net", "dir2"}: {created: 1, samples: []string{"g.txt"}},
	}, next)
}
