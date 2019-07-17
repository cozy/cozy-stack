package vfs

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
)

func uuidv4() string {
	id, _ := uuid.NewV4()
	return id.String()
}

func TestDetectVersionsToClean(t *testing.T) {
	fileID := uuidv4()
	now := time.Now()
	genVersion := func(timeAgo time.Duration) Version {
		v := Version{
			DocID: fileID + "/" + utils.RandomString(16),
		}
		v.CozyMetadata.CreatedAt = now.Add(-1 * timeAgo)
		return v
	}

	v0 := genVersion(120 * time.Minute)
	v1 := genVersion(100 * time.Minute)
	v2 := genVersion(80 * time.Minute)
	v3 := genVersion(60 * time.Minute)
	v4 := genVersion(40 * time.Minute)
	v5 := genVersion(20 * time.Minute)
	v6 := genVersion(2 * time.Minute)
	candidate := genVersion(0 * time.Minute)

	olds := []*Version{&v0, &v1, &v2}
	cleanCandidate, toClean := detectVersionsToClean(&candidate, olds, 20, 1*time.Minute)
	assert.False(t, cleanCandidate)
	assert.Len(t, toClean, 0)

	olds = []*Version{&v0, &v1, &v2, &v3, &v4, &v5, &v6}
	cleanCandidate, toClean = detectVersionsToClean(&candidate, olds, 20, 15*time.Minute)
	assert.True(t, cleanCandidate)
	assert.Len(t, toClean, 0)

	olds = []*Version{&v1, &v2, &v3, &v4, &v5}
	cleanCandidate, toClean = detectVersionsToClean(&candidate, olds, 5, 30*time.Minute)
	assert.True(t, cleanCandidate)
	assert.Len(t, toClean, 1)
	assert.Equal(t, &v1, toClean[0])

	olds = []*Version{&v1, &v2, &v3, &v4}
	cleanCandidate, toClean = detectVersionsToClean(&candidate, olds, 5, 15*time.Minute)
	assert.False(t, cleanCandidate)
	assert.Len(t, toClean, 1)
	assert.Equal(t, &v1, toClean[0])

	olds = []*Version{&v3, &v6, &v2, &v0, &v5, &v4, &v1}
	cleanCandidate, toClean = detectVersionsToClean(&candidate, olds, 5, 1*time.Minute)
	assert.False(t, cleanCandidate)
	assert.Len(t, toClean, 4)
	assert.Equal(t, &v0, toClean[0])
	assert.Equal(t, &v1, toClean[1])
	assert.Equal(t, &v2, toClean[2])
	assert.Equal(t, &v3, toClean[3])

	olds = []*Version{&v3, &v6, &v2, &v0, &v5, &v4, &v1}
	cleanCandidate, toClean = detectVersionsToClean(&candidate, olds, 5, 10*time.Minute)
	assert.True(t, cleanCandidate)
	assert.Len(t, toClean, 3)
	assert.Equal(t, &v0, toClean[0])
	assert.Equal(t, &v1, toClean[1])
	assert.Equal(t, &v2, toClean[2])

	v0.Tags = []string{"foo"}
	v2.Tags = []string{"bar", "baz"}
	candidate.Tags = []string{"qux"}

	olds = []*Version{&v3, &v6, &v2, &v0, &v5, &v4, &v1}
	cleanCandidate, toClean = detectVersionsToClean(&candidate, olds, 5, 10*time.Minute)
	assert.False(t, cleanCandidate)
	assert.Len(t, toClean, 4)
	assert.Equal(t, &v1, toClean[0])
	assert.Equal(t, &v3, toClean[1])
	assert.Equal(t, &v4, toClean[2])
	assert.Equal(t, &v5, toClean[3])
}
