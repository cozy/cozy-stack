package vfs

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExifMetadataExtractor(t *testing.T) {
	doc := &FileDoc{Mime: "image/jpg"}
	extractor := NewMetaExtractor(doc)
	assert.NotNil(t, extractor)
	f, err := os.Open("../../tests/fixtures/wet-cozy_20160910__©M4Dz.jpg")
	assert.NoError(t, err)
	defer f.Close()
	_, err = io.Copy(*extractor, f)
	(*extractor).Close()
	assert.NoError(t, err)
	meta := (*extractor).Result()
	dt, ok := meta["datetime"].(time.Time)
	assert.True(t, ok, "datetime is present")
	year, month, day := dt.Date()
	assert.Equal(t, 2016, year)
	assert.Equal(t, time.September, month)
	assert.Equal(t, 10, day)
}
