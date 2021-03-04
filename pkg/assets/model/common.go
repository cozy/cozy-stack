package model

import (
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/pkg/filetype"
)

// sumLen defines the number of characters of the shasum to include in a
// filename
const sumLen = 10

// AssetOption is used to insert a dynamic asset.
type AssetOption struct {
	Name     string `json:"name"`
	Context  string `json:"context"`
	URL      string `json:"url"`
	Shasum   string `json:"shasum"`
	IsCustom bool   `json:"is_custom,omitempty"`
}

// Asset holds unzipped read-only file contents and file metadata.
type Asset struct {
	AssetOption
	Etag        string `json:"etag"`
	NameWithSum string `json:"name_with_sum"`
	Mime        string `json:"mime"`

	brotliData []byte
	brotliSize string
	rawData    []byte
	rawSize    string
}

// GetData returns the raw data as a slice of bytes.
func (f *Asset) GetData() []byte {
	return f.rawData
}

// Size returns the size in bytes of the asset (no compression).
func (f *Asset) Size() string {
	return f.rawSize
}

// Reader returns a bytes.Reader for the asset content (no compression).
func (f *Asset) Reader() *bytes.Reader {
	return bytes.NewReader(f.rawData)
}

// BrotliSize returns the size of the compressed version of the asset.
func (f *Asset) BrotliSize() string {
	return f.brotliSize
}

// BrotliReader returns a bytes.Reader for the compressed content of the asset.
func (f *Asset) BrotliReader() *bytes.Reader {
	return bytes.NewReader(f.brotliData)
}

// NameWithSum returns the filename with its shasum
func NameWithSum(name, sum string) string {
	nameWithSum := name

	nameBase := path.Base(name)
	if off := strings.IndexByte(nameBase, '.'); off >= 0 {
		nameDir := path.Dir(name)
		nameWithSum = path.Join("/", nameDir, nameBase[:off]+"."+sum[:sumLen]+nameBase[off:])
	}

	return nameWithSum
}

// NormalizeAssetName ensures the asset name always start with a "/"
func NormalizeAssetName(name string) string {
	return path.Join("/", name)
}

// NewAsset creates a new asset
func NewAsset(opt AssetOption, rawData, brotliData []byte) *Asset {
	mime := filetype.ByExtension(path.Ext(opt.Name))
	if mime == "" {
		mime = filetype.Match(rawData)
	}

	opt.Name = NormalizeAssetName(opt.Name)

	sumx := opt.Shasum
	etag := fmt.Sprintf(`"%s"`, sumx[:sumLen])
	nameWithSum := NameWithSum(opt.Name, sumx)

	return &Asset{
		AssetOption: opt,
		Etag:        etag,
		NameWithSum: nameWithSum,
		Mime:        mime,
		brotliData:  brotliData,
		brotliSize:  strconv.Itoa(len(brotliData)),

		rawData: rawData,
		rawSize: strconv.Itoa(len(rawData)),
	}
}
