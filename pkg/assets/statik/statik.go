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
package statik

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

var globalAssets sync.Map // {context:path -> *Asset}

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
		opt := model.AssetOption{
			Name:    name,
			Context: config.DefaultInstanceContext,
			Shasum:  hex.EncodeToString(h.Sum(nil)),
		}
		asset := model.NewAsset(opt, zippedData, unzippedData)
		StoreAsset(asset)
		data = rest
	}
	return
}

// StoreAsset stores in memory a static asset
func StoreAsset(asset *model.Asset) {
	context := asset.Context
	if context == "" {
		context = config.DefaultInstanceContext
	}
	contextKey := marshalContextKey(context, asset.Name)
	globalAssets.Store(contextKey, asset)
}

// UnstoreAsset removes a static asset from the memory list
func UnstoreAsset(asset *model.Asset) {
	context := asset.Context
	if context == "" {
		context = config.DefaultInstanceContext
	}
	contextKey := marshalContextKey(context, asset.Name)
	globalAssets.Delete(contextKey)
}

func GetAsset(context, name string) *model.Asset {
	if context == "" {
		context = config.DefaultInstanceContext
	}
	if v, ok := globalAssets.Load(marshalContextKey(context, name)); ok {
		return v.(*model.Asset)
	}
	return nil
}

// Foreach iterates on the static assets.
func Foreach(predicate func(name, context string, f *model.Asset)) {
	globalAssets.Range(func(contextKey interface{}, v interface{}) bool {
		context, name, _ := unMarshalContextKey(contextKey.(string))
		predicate(name, context, v.(*model.Asset))
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
