// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package fs contains an HTTP file system that works with zip contents.
package fs

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/filetype"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/swift"
	multierror "github.com/hashicorp/go-multierror"
)

var assetsClient = &http.Client{
	Timeout: 30 * time.Second,
}

var globalAssets sync.Map // {context:path -> *Asset}

const sumLen = 10

// DynamicAssetsContainerName is the Swift container name for dynamic assets
const DynamicAssetsContainerName = "__dyn-assets__"

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

	zippedData   []byte
	zippedSize   string
	unzippedData []byte
	unzippedSize string
}

// Size returns the size in bytes of the asset (no compression).
func (f *Asset) Size() string {
	return f.unzippedSize
}

// Reader returns a bytes.Reader for the asset content (no compression).
func (f *Asset) Reader() *bytes.Reader {
	return bytes.NewReader(f.unzippedData)
}

// GzipSize returns the size of the gzipped version of the asset.
func (f *Asset) GzipSize() string {
	return f.zippedSize
}

// GzipReader returns a bytes.Reader for the gzipped content of the asset.
func (f *Asset) GzipReader() *bytes.Reader {
	return bytes.NewReader(f.zippedData)
}

// Register registers zip contents data, later used to
// initialize the statik file system.
func Register(zipData string) {
	if zipData == "" {
		panic("statik/fs: no zip data registered")
	}
	if err := unzip([]byte(zipData)); err != nil {
		panic(fmt.Errorf("statik/fs: error unzipping data: %s", err))
	}
}

// RegisterCustomExternals ensures that the assets are in the Swift, and load
// them from their source if they are not yet available.
func RegisterCustomExternals(opts []AssetOption, maxTryCount int) error {
	if len(opts) == 0 {
		return nil
	}

	assetsCh := make(chan AssetOption)
	doneCh := make(chan error)

	for i := 0; i < len(opts); i++ {
		go func() {
			var err error
			sleepDuration := 500 * time.Millisecond
			opt := <-assetsCh

			for tryCount := 0; tryCount < maxTryCount+1; tryCount++ {
				err = registerCustomExternal(opt)
				if err == nil {
					break
				}
				logger.WithNamespace("statik").
					Errorf("Could not load asset from %q, retrying in %s", opt.URL, sleepDuration)
				time.Sleep(sleepDuration)
				sleepDuration *= 4
			}

			doneCh <- err
		}()
	}

	for _, opt := range opts {
		assetsCh <- opt
	}
	close(assetsCh)

	var errm error
	for i := 0; i < len(opts); i++ {
		if err := <-doneCh; err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func registerCustomExternal(opt AssetOption) error {
	if opt.Context == "" {
		logger.WithNamespace("custom assets").
			Warningf("Could not load asset %s with empty context", opt.URL)
		return nil
	}

	opt.IsCustom = true

	assetURL := opt.URL

	var body io.Reader

	u, err := url.Parse(assetURL)
	if err != nil {
		return err
	}

	switch u.Scheme {
	case "http", "https":
		req, err := http.NewRequest(http.MethodGet, assetURL, nil)
		if err != nil {
			return err
		}
		res, err := assetsClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("could not load external asset on %s: status code %d", assetURL, res.StatusCode)
		}
		body = res.Body
	case "file":
		f, err := os.Open(u.Path)
		if err != nil {
			return err
		}
		defer f.Close()
		body = f
	default:
		return fmt.Errorf("does not support externals assets with scheme %q", u.Scheme)
	}

	h := sha256.New()
	zippedDataBuf := new(bytes.Buffer)
	gw := gzip.NewWriter(zippedDataBuf)

	teeReader := io.TeeReader(body, io.MultiWriter(h, gw))
	unzippedData, err := ioutil.ReadAll(teeReader)
	if err != nil {
		return err
	}
	if errc := gw.Close(); errc != nil {
		return errc
	}

	sum := h.Sum(nil)

	if opt.Shasum == "" {
		opt.Shasum = hex.EncodeToString(sum)
		log := logger.WithNamespace("custom_external")
		log.Warnf("shasum was not provided for file %s, inserting unsafe content %s: %s",
			opt.Name, opt.URL, opt.Shasum)
	}

	if hex.EncodeToString(sum) != opt.Shasum {
		return fmt.Errorf("external content checksum do not match: expected %s got %x on url %s",
			opt.Shasum, sum, assetURL)
	}

	asset := newAsset(opt, zippedDataBuf.Bytes(), unzippedData)

	objectName := path.Join(asset.Context, asset.Name)
	swiftConn := config.GetSwiftConnection()

	f, err := swiftConn.ObjectCreate(DynamicAssetsContainerName, objectName, true, "", "", nil)
	if err != nil {
		return err
	}
	defer f.Close()

	// Writing the asset content to Swift
	_, err = f.Write(asset.unzippedData)
	if err != nil {
		return err
	}

	return nil
}

func unzip(data []byte) (err error) {
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		var zippedData, unzippedData []byte
		zippedData = block.Bytes
		var gr *gzip.Reader
		gr, err = gzip.NewReader(bytes.NewReader(block.Bytes))
		if err != nil {
			return
		}
		h := sha256.New()
		r := io.TeeReader(gr, h)
		unzippedData, err = ioutil.ReadAll(r)
		if err != nil {
			return
		}
		if err = gr.Close(); err != nil {
			return
		}

		name := block.Headers["Name"]
		opt := AssetOption{
			Name:    name,
			Context: config.DefaultInstanceContext,
			Shasum:  hex.EncodeToString(h.Sum(nil)),
		}
		asset := newAsset(opt, zippedData, unzippedData)
		storeAsset(asset)
		data = rest
	}
	return
}

func NormalizeAssetName(name string) string {
	return path.Join("/", name)
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

func newAsset(opt AssetOption, zippedData, unzippedData []byte) *Asset {
	mime := filetype.ByExtension(path.Ext(opt.Name))
	if mime == "" {
		mime = filetype.Match(unzippedData)
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
		zippedData:  zippedData,
		zippedSize:  strconv.Itoa(len(zippedData)),

		unzippedData: unzippedData,
		unzippedSize: strconv.Itoa(len(unzippedData)),
	}
}

// threadsafe
// Used to store statik assets
func storeAsset(asset *Asset) {
	context := asset.Context
	if context == "" {
		context = config.DefaultInstanceContext
	}
	contextKey := marshalContextKey(context, asset.Name)
	globalAssets.Store(contextKey, asset)
}

// DeleteAsset removes a dynamic asset.
func DeleteAsset(asset *Asset) {
	context := asset.Context
	if context == "" {
		context = config.DefaultInstanceContext
	}
	contextKey := marshalContextKey(context, asset.Name)
	globalAssets.Delete(contextKey)
}

// GetDynamicAsset retrieves a raw asset from Swit and build a fs.Asset
func GetDynamicAsset(context, name string) (*Asset, error) {
	swiftConn := config.GetSwiftConnection()
	objectName := path.Join(context, name)

	assetContent := new(bytes.Buffer)

	_, err := swiftConn.ObjectGet(DynamicAssetsContainerName, objectName, assetContent, true, nil)
	if err != nil && err == swift.ObjectNotFound {
		return nil, err
	}

	// Re-constructing the asset struct from the Swift content
	content := assetContent.Bytes()

	h := sha256.New()
	_, err = h.Write(content)
	if err != nil {
		return nil, err
	}
	suma := h.Sum(nil)
	sumx := hex.EncodeToString(suma)

	zippedDataBuf := new(bytes.Buffer)
	gw := gzip.NewWriter(zippedDataBuf)
	_, err = gw.Write(content)
	if err != nil {
		return nil, err
	}
	zippedContent := zippedDataBuf.Bytes()

	asset := newAsset(AssetOption{
		Shasum:   sumx,
		Name:     name,
		Context:  context,
		IsCustom: true,
	}, zippedContent, content)

	return asset, nil
}

// Get returns an asset for the given context, or the default context if
// no context is given.
func Get(name string, context ...string) (*Asset, bool) {
	var ctx string

	if len(context) > 0 && context[0] != "" {
		ctx = context[0]
	} else {
		ctx = config.DefaultInstanceContext
	}

	// Dynamic assets are stored in Swift
	if config.FsURL().Scheme == config.SchemeSwift ||
		config.FsURL().Scheme == config.SchemeSwiftSecure {
		dynAsset, err := GetDynamicAsset(ctx, name)
		if err == nil {
			return dynAsset, true
		}
	}

	// If asset was not found, try to retrieve it from static assets
	asset, ok := globalAssets.Load(marshalContextKey(ctx, name))
	if !ok {
		return nil, false
	}
	return asset.(*Asset), true

}

// Open returns a bytes.Reader for an asset in the given context, or the
// default context if no context is given.
func Open(name string, context ...string) (*bytes.Reader, error) {
	f, ok := Get(name, context...)
	if ok {
		return f.Reader(), nil
	}
	return nil, os.ErrNotExist
}

// Foreach iterates on the dynamic assets.
func Foreach(predicate func(name, context string, f *Asset)) {
	globalAssets.Range(func(contextKey interface{}, v interface{}) bool {
		context, name, _ := unMarshalContextKey(contextKey.(string))
		predicate(name, context, v.(*Asset))
		return true
	})
}

func marshalContextKey(context, name string) (marshaledKey string) {
	return context + ":" + name
}

func unMarshalContextKey(contextKey string) (context string, name string, err error) {
	unmarshaled := strings.SplitN(contextKey, ":", 2)
	if len(unmarshaled) != 2 {
		panic("statik/fs: the contextKey is malformed")
	}
	return unmarshaled[0], unmarshaled[1], nil
}
