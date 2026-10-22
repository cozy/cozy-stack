package driveactivity

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/redis/go-redis/v9"
)

// The kinds of change counted.
const (
	Created = "created"
	Updated = "updated"
	Deleted = "deleted"
)

// maxSamples is the number of file names kept per member and folder.
const maxSamples = 5

// keepCounts bounds how long counts that no flush took stay in Redis.
const keepCounts = 24 * time.Hour

type change struct {
	member string
	folder string
	kind   string
	name   string
}

// key is a member and the folder holding the files they changed.
type key struct {
	member string
	folder string
}

type tally struct {
	created int
	updated int
	deleted int
	samples []string
}

func (t *tally) count(kind string, n int) {
	switch kind {
	case Created:
		t.created += n
	case Updated:
		t.updated += n
	case Deleted:
		t.deleted += n
	}
}

func (t *tally) sample(name string) {
	if len(t.samples) < maxSamples && !slices.Contains(t.samples, name) {
		t.samples = append(t.samples, name)
	}
}

// store holds the counts of each window in a Redis hash, whose fields are
// the member, the folder and what is counted, separated by newlines. The
// open window of a drive is a key that expires with the window, so no command
// has to close it.
type store struct {
	c   redis.UniversalClient
	ctx context.Context
}

// getStore returns false when the Stack has no Redis: the digest is off.
func getStore() (store, bool) {
	cli := config.GetConfig().SessionStorage
	return store{cli, context.Background()}, cli != nil
}

func windowKey(drive string) string {
	return "drive-activity:" + drive + ":window"
}

func countsKey(drive, window string) string {
	return "drive-activity:" + drive + ":" + window
}

// add counts a change in the open window of the drive, or opens one at start
// when there is none. It returns the window and whether this change opened
// it: the caller then schedules its flush.
func (s store) add(drive string, c change, start string, window time.Duration) (string, bool, error) {
	opened, err := s.c.SetNX(s.ctx, windowKey(drive), start, window).Result()
	if err != nil {
		return "", false, err
	}
	w := start
	if !opened {
		if w, err = s.c.Get(s.ctx, windowKey(drive)).Result(); err != nil {
			return "", false, err
		}
	}

	counts := countsKey(drive, w)
	field := c.member + "\n" + c.folder + "\n"
	pipe := s.c.Pipeline()
	n := pipe.HIncrBy(s.ctx, counts, field+c.kind, 1)
	pipe.Expire(s.ctx, counts, keepCounts)
	if _, err := pipe.Exec(s.ctx); err != nil {
		return "", false, err
	}
	if n.Val() <= maxSamples {
		err = s.c.HSet(s.ctx, counts, field+"sample\n"+c.name, 1).Err()
	}
	return w, opened, err
}

// take returns the counts of the window. Taking it again returns the same
// counts, until drop.
func (s store) take(drive, window string) (map[key]*tally, error) {
	fields, err := s.c.HGetAll(s.ctx, countsKey(drive, window)).Result()
	if err != nil {
		return nil, err
	}
	return parseCounts(fields), nil
}

func (s store) drop(drive, window string) error {
	return s.c.Del(s.ctx, countsKey(drive, window)).Err()
}

func parseCounts(fields map[string]string) map[key]*tally {
	counts := make(map[key]*tally)
	for field, value := range fields {
		parts := strings.SplitN(field, "\n", 3)
		if len(parts) != 3 {
			continue
		}
		k := key{parts[0], parts[1]}
		t := counts[k]
		if t == nil {
			t = &tally{}
			counts[k] = t
		}
		if name, ok := strings.CutPrefix(parts[2], "sample\n"); ok {
			t.sample(name)
		} else if n, err := strconv.Atoi(value); err == nil {
			t.count(parts[2], n)
		}
	}
	return counts
}
