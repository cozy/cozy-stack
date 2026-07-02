package rag

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/stretchr/testify/assert"
)

func TestApplyRAGStatus(t *testing.T) {
	t1 := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	t.Run("success sets Indexed=true and LastSuccessDate", func(t *testing.T) {
		rag := &vfs.RAGMetadata{}
		applyRAGStatus(rag, RAGStatusSuccess, t1)
		assert.True(t, rag.Indexed)
		assert.Equal(t, RAGStatusSuccess, rag.Status)
		assert.Equal(t, t1, *rag.LastSuccessDate)
		assert.Nil(t, rag.LastErrorDate)
	})

	t.Run("error without prior success keeps Indexed=false", func(t *testing.T) {
		rag := &vfs.RAGMetadata{}
		applyRAGStatus(rag, RAGStatusError, t1)
		assert.False(t, rag.Indexed)
		assert.Equal(t, RAGStatusError, rag.Status)
		assert.Equal(t, t1, *rag.LastErrorDate)
		assert.Nil(t, rag.LastSuccessDate)
	})

	t.Run("error after success preserves Indexed=true", func(t *testing.T) {
		rag := &vfs.RAGMetadata{Indexed: true}
		applyRAGStatus(rag, RAGStatusError, t1)
		assert.True(t, rag.Indexed)
		assert.Equal(t, RAGStatusError, rag.Status)
		assert.Equal(t, t1, *rag.LastErrorDate)
	})

	t.Run("notsupported does not touch Indexed or dates", func(t *testing.T) {
		t2 := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
		rag := &vfs.RAGMetadata{
			Indexed:         true,
			LastSuccessDate: &t2,
			LastErrorDate:   &t2,
		}
		applyRAGStatus(rag, RAGStatusNotSupported, t1)
		assert.True(t, rag.Indexed)
		assert.Equal(t, RAGStatusNotSupported, rag.Status)
		assert.Equal(t, t2, *rag.LastSuccessDate)
		assert.Equal(t, t2, *rag.LastErrorDate)
	})

	t.Run("success overwrites a previous error status", func(t *testing.T) {
		t2 := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
		rag := &vfs.RAGMetadata{Status: RAGStatusError, LastErrorDate: &t2}
		applyRAGStatus(rag, RAGStatusSuccess, t1)
		assert.True(t, rag.Indexed)
		assert.Equal(t, RAGStatusSuccess, rag.Status)
		assert.Equal(t, t1, *rag.LastSuccessDate)
	})
}
