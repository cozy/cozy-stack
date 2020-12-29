package vfs

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/stretchr/testify/assert"
)

func TestImageMetadataExtractor(t *testing.T) {
	doc := &FileDoc{Mime: "image/png"}
	extractor := NewMetaExtractor(doc)
	assert.NotNil(t, extractor)
	f, err := os.Open("../../assets/icon-192.png")
	assert.NoError(t, err)
	defer f.Close()
	_, err = io.Copy(*extractor, f)
	assert.True(t, err == nil || err == io.ErrClosedPipe)
	err = (*extractor).Close()
	assert.NoError(t, err)
	meta := (*extractor).Result()
	version, ok := meta["extractor_version"].(int)
	assert.True(t, ok, "extractor_version is present")
	assert.Equal(t, MetadataExtractorVersion, version)
	w, ok := meta["width"].(int)
	assert.True(t, ok, "width is present")
	assert.Equal(t, 192, w)
	h, ok := meta["height"].(int)
	assert.True(t, ok, "height is present")
	assert.Equal(t, 192, h)
}

func TestExifMetadataExtractor(t *testing.T) {
	doc := &FileDoc{Mime: "image/jpeg"}
	extractor := NewMetaExtractor(doc)
	assert.NotNil(t, extractor)
	f, err := os.Open("../../tests/fixtures/wet-cozy_20160910__M4Dz.jpg")
	assert.NoError(t, err)
	defer f.Close()
	_, err = io.Copy(*extractor, f)
	(*extractor).Close()
	assert.NoError(t, err)
	meta := (*extractor).Result()
	version, ok := meta["extractor_version"].(int)
	assert.True(t, ok, "extractor_version is present")
	assert.Equal(t, MetadataExtractorVersion, version)
	dt, ok := meta["datetime"].(time.Time)
	assert.True(t, ok, "datetime is present")
	year, month, day := dt.Date()
	assert.Equal(t, 2016, year)
	assert.Equal(t, time.September, month)
	assert.Equal(t, 10, day)
	w, ok := meta["width"].(int)
	assert.True(t, ok, "width is present")
	assert.Equal(t, 440, w)
	h, ok := meta["height"].(int)
	assert.True(t, ok, "height is present")
	assert.Equal(t, 294, h)
}

func TestShortcutMetadataExtractor(t *testing.T) {
	doc := &FileDoc{
		Mime: consts.ShortcutMimeType,
		CozyMetadata: &FilesCozyMetadata{
			CreatedOn: "http://cozy.tools:8080/",
		},
	}
	extractor := NewMetaExtractor(doc)
	assert.NotNil(t, extractor)
	f, err := os.Open("../../tests/fixtures/shortcut.url")
	assert.NoError(t, err)
	defer f.Close()
	_, err = io.Copy(*extractor, f)
	(*extractor).Close()
	assert.NoError(t, err)
	meta := (*extractor).Result()
	version, ok := meta["extractor_version"].(int)
	assert.True(t, ok, "extractor_version is present")
	assert.Equal(t, MetadataExtractorVersion, version)
	target, ok := meta["target"].(map[string]interface{})
	assert.True(t, ok, "target is present in metadata")
	cm, ok := target["cozyMetadata"].(map[string]interface{})
	assert.True(t, ok, "target.cozyMetadata is present")
	cozy, ok := cm["instance"].(string)
	assert.True(t, ok, "target.cozyMetadata.instance is present")
	assert.Equal(t, "cozy.tools:8080", cozy)
	app, ok := target["app"]
	assert.True(t, ok, "target.app is present")
	assert.Equal(t, "drive", app)
}
