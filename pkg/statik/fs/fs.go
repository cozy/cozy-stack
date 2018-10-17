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

	"github.com/hashicorp/go-multierror"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/magic"
)

var assetsClient = &http.Client{
	Timeout: 30 * time.Second,
}

var globalAssets sync.Map // {context:path -> *Asset}

const sumLen = 10
const defaultContext = "default"

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
	if err := unzip([]byte(zipData)); err != nil {
		panic(fmt.Errorf("statik/fs: error unzipping data: %s", err))
	}
}

type Cache interface {
	Get(key string) (io.Reader, bool)
	Set(key string, data []byte, expiration time.Duration) bool
}

func RegisterCustomExternals(cache Cache, opts []AssetOption, maxTryCount int) error {
	if len(opts) == 0 {
		return nil
	}

	assetsCh := make(chan AssetOption)
	doneCh := make(chan []error)

	for i := 0; i < 16; i++ {
		go func() {
			var err error
			var errorsResult []error

			for opt := range assetsCh {
				sleepDuration := 500 * time.Millisecond

				for tryCount := 0; tryCount < maxTryCount+1; tryCount++ {
					err = registerCustomExternal(cache, opt)
					if err == nil {
						break
					}
					if tryCount == maxTryCount {
						errorsResult = append(errorsResult, err)
					}
					logger.WithNamespace("statik").
						Errorf("Could not load asset from %q, retrying in %s", opt.URL, sleepDuration)
					time.Sleep(sleepDuration)
					sleepDuration *= 2
				}
			}

			doneCh <- errorsResult
		}()
	}

	for _, opt := range opts {
		assetsCh <- opt
	}
	close(assetsCh)

	var errm error
	for i := 0; i < 16; i++ {
		if errs := <-doneCh; len(errs) > 0 {
			errm = multierror.Append(errm, errs...)
		}
	}
	return errm
}

func registerCustomExternal(cache Cache, opt AssetOption) error {
	name := normalizeAssetName(opt.Name)
	if currentAsset, ok := Get(name, opt.Context); ok {
		if currentAsset.Shasum == opt.Shasum {
			return nil
		}
	}

	opt.IsCustom = true

	assetURL := opt.URL
	key := fmt.Sprintf("assets:%s:%s:%s", opt.Context, name, opt.Shasum)

	var body io.Reader
	var ok, storeInCache bool
	if body, ok = cache.Get(key); !ok {
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
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("could not load external asset on %s: status code %d", assetURL, res.StatusCode)
			}
			defer res.Body.Close()
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

		storeInCache = true
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
		return err
	}

	sum := h.Sum(nil)
	if hex.EncodeToString(sum) != opt.Shasum {
		return fmt.Errorf("external content checksum do not match: expected %s got %x on url %s",
			opt.Shasum, sum, assetURL)
	}

	if storeInCache {
		expiration := 30 * 24 * time.Hour
		cache.Set(key, unzippedData, expiration)
	}

	asset := newAsset(opt, zippedDataBuf.Bytes(), unzippedData)
	storeAsset(asset)
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
			Context: defaultContext,
			Shasum:  hex.EncodeToString(h.Sum(nil)),
		}
		asset := newAsset(opt, zippedData, unzippedData)
		storeAsset(asset)
		data = rest
	}
	return
}

func normalizeAssetName(name string) string {
	return path.Join("/", name)
}

func newAsset(opt AssetOption, zippedData, unzippedData []byte) *Asset {
	mime := magic.MIMETypeByExtension(path.Ext(opt.Name))
	if mime == "" {
		mime = magic.MIMEType(unzippedData)
	}
	if mime == "" {
		mime = "application/octet-stream"
	}

	sumx := opt.Shasum
	etag := fmt.Sprintf(`"%s"`, sumx[:sumLen])

	opt.Name = normalizeAssetName(opt.Name)

	nameWithSum := opt.Name
	nameBase := path.Base(opt.Name)
	if off := strings.IndexByte(nameBase, '.'); off >= 0 {
		nameDir := path.Dir(opt.Name)
		nameWithSum = path.Join("/", nameDir, nameBase[:off]+"."+sumx[:sumLen]+nameBase[off:])
	}

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
func storeAsset(asset *Asset) {
	context := asset.Context
	if context == "" {
		context = defaultContext
	}
	contextKey := marshalContextKey(context, asset.Name)
	globalAssets.Store(contextKey, asset)
}

func Get(name string, context ...string) (*Asset, bool) {
	var ctx string
	if len(context) > 0 && context[0] != "" {
		ctx = context[0]
	} else {
		ctx = defaultContext
	}
	asset, ok := globalAssets.Load(marshalContextKey(ctx, name))
	if !ok {
		return nil, false
	}
	return asset.(*Asset), true
}

func Open(name string, context ...string) (*bytes.Reader, error) {
	f, ok := Get(name, context...)
	if ok {
		return f.Reader(), nil
	}
	return nil, os.ErrNotExist
}

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
