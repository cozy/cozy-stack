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

	"github.com/cozy/cozy-stack/pkg/magic"
)

var assetsClient = &http.Client{
	Timeout: 30 * time.Second,
}

var globalAssets map[string]*sync.Map // context -> sync.Map{path -> *Asset}

const sumLen = 10
const defaultContext = "default"

// Asset holds unzipped read-only file contents and file metadata.
type Asset struct {
	zippedData     []byte
	zippedSize     string
	unzippedData   []byte
	unzippedSize   string
	unzippedShasum []byte
	Etag           string `json:"etag"`
	Name           string `json:"name"`
	NameWithSum    string `json:"nameWithSum"`
	Mime           string `json:"mime"`
	Context        string `json:"context"`
}

func (f *Asset) Size() string {
	return f.unzippedSize
}
func (f *Asset) Reader() *bytes.Reader {
	return bytes.NewReader(f.unzippedData)
}

func (f *Asset) GzipSize() string {
	return f.zippedSize
}
func (f *Asset) GzipReader() *bytes.Reader {
	return bytes.NewReader(f.zippedData)
}

// Register registers zip contents data, later used to
// initialize the statik file system.
func Register(zipData string) {
	if zipData == "" {
		panic("statik/fs: no zip data registered")
	}
	if globalAssets == nil {
		globalAssets = make(map[string]*sync.Map)
	}
	if err := unzip([]byte(zipData)); err != nil {
		panic(fmt.Errorf("statik/fs: error unzipping data: %s", err))
	}
}

type AssetOption struct {
	Name    string `json:"name"`
	Context string `json:"context"`
	URL     string `json:"url"`
	Shasum  string `json:"shasum"`
}

func RegisterCustomExternals(opts []AssetOption) error {
	if globalAssets == nil {
		globalAssets = make(map[string]*sync.Map)
	}

	var loadedAssets []*Asset
	for _, opt := range opts {
		asset, err := registerCustomExternal(opt.Name, opt.Context, opt.URL, opt.Shasum)
		if err != nil {
			return err
		}
		loadedAssets = append(loadedAssets, asset)
	}
	for _, asset := range loadedAssets {
		storeAsset(asset)
	}
	return nil
}

func registerCustomExternal(name, context, assetURL, shasum string) (*Asset, error) {
	hexShasum, _ := hex.DecodeString(shasum)
	if currentAsset, ok := Get(name, context); ok {
		if bytes.Equal(currentAsset.unzippedShasum, []byte(hexShasum)) {
			return currentAsset, nil
		}
	}

	u, err := url.Parse(assetURL)
	if err != nil {
		return nil, err
	}

	var body io.Reader

	if u.Scheme == "http" || u.Scheme == "https" {
		req, err := http.NewRequest(http.MethodGet, assetURL, nil)
		if err != nil {
			return nil, err
		}
		res, err := assetsClient.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("could not load external asset on %s: status code %d", assetURL, assetURL, res.StatusCode)
		}
		defer res.Body.Close()
		body = res.Body
	} else if u.Scheme == "file" {
		f, err := os.Open(u.Path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		body = f
	} else {
		return nil, fmt.Errorf("does not support externals assets with scheme %q", u.Scheme)
	}

	h := sha256.New()

	zippedDataBuf := new(bytes.Buffer)
	gw := gzip.NewWriter(zippedDataBuf)

	teeReader := io.TeeReader(body, io.MultiWriter(h, gw))
	unzippedData, err := ioutil.ReadAll(teeReader)
	if err != nil {
		return nil, err
	}

	if errc := gw.Close(); errc != nil {
		return nil, err
	}

	sum := h.Sum(nil)
	if !bytes.Equal(sum, []byte(hexShasum)) {
		return nil, fmt.Errorf("external content checksum do not match: expected %x got %x on url %s",
			hexShasum, sum, assetURL)
	}

	return newAsset(name, context, sum, zippedDataBuf.Bytes(), unzippedData), nil
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
		asset := newAsset(name, defaultContext, h.Sum(nil), zippedData, unzippedData)
		storeAsset(asset)
		data = rest
	}
	return
}

func newAsset(name, context string, unzippedSum, zippedData, unzippedData []byte) *Asset {
	mime := magic.MIMETypeByExtension(path.Ext(name))
	if mime == "" {
		mime = magic.MIMEType(unzippedData)
	}
	if mime == "" {
		mime = "application/octet-stream"
	}

	sumx := hex.EncodeToString(unzippedSum)
	etag := fmt.Sprintf(`"%s"`, sumx[:sumLen])
	nameWithSum := name
	if off := strings.IndexByte(name, '.'); off >= 0 {
		nameWithSum = name[:off] + "." + sumx[:sumLen] + name[off:]
	}

	return &Asset{
		zippedData: zippedData,
		zippedSize: strconv.Itoa(len(zippedData)),

		unzippedData:   unzippedData,
		unzippedSize:   strconv.Itoa(len(unzippedData)),
		unzippedShasum: unzippedSum,

		Etag:        etag,
		Name:        name,
		NameWithSum: nameWithSum,
		Mime:        mime,
		Context:     context,
	}
}

// threadsafe
func storeAsset(asset *Asset) {
	context := asset.Context
	if context == "" {
		context = defaultContext
	}
	// NOTE(pierre): warning: to be completely threadsafe, this implies that
	// contextual maps are not added concurrently.
	assetsForContext, ok := globalAssets[context]
	if !ok {
		assetsForContext = new(sync.Map)
		globalAssets[context] = assetsForContext
	}
	assetsForContext.Store(asset.Name, asset)
}

func Get(name string, context ...string) (*Asset, bool) {
	if globalAssets == nil {
		panic("statik/fs: not registered")
	}
	var ctx string
	if len(context) > 0 && context[0] != "" {
		ctx = context[0]
	} else {
		ctx = defaultContext
	}
	if assetsForContext, ok := globalAssets[ctx]; ok {
		if v, ok := assetsForContext.Load(name); ok {
			return v.(*Asset), true
		}
	}
	// If we didn't find the asset in the specified context, with the given name,
	// we seek into the default context.
	if ctx != defaultContext {
		if assetsForDefaultContext, ok := globalAssets[defaultContext]; ok {
			if v, ok := assetsForDefaultContext.Load(name); ok {
				return v.(*Asset), true
			}
		}
	}
	return nil, false
}

func Open(name string, context ...string) (*bytes.Reader, error) {
	f, ok := Get(name, context...)
	if ok {
		return f.Reader(), nil
	}
	return nil, os.ErrNotExist
}

func Foreach(predicate func(name, context string, f *Asset)) {
	for context, assetsForContext := range globalAssets {
		assetsForContext.Range(func(name interface{}, v interface{}) bool {
			predicate(name.(string), context, v.(*Asset))
			return true
		})
	}
}
