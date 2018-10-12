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

// Package contains a program that generates code to register
// a directory and its contents as zip data for statik file system.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	humanize "github.com/dustin/go-humanize"
)

const (
	namePackage    = "statik"
	nameSourceFile = "statik.go"
)

var (
	flagSrc       = flag.String("src", path.Join(".", "public"), "The path of the source directory.")
	flagDest      = flag.String("dest", ".", "The destination path of the generated package.")
	flagExternals = flag.String("externals", "", "File containing a description of externals assets to download.")
	flagForce     = flag.Bool("f", false, "Overwrite destination file if it already exists.")
)

var (
	errExternalsMalformed = errors.New("assets externals file malformed")
)

type asset struct {
	name   string
	size   int64
	url    string
	data   []byte
	sha256 []byte
}

func main() {
	flag.Parse()

	destDir := path.Join(*flagDest, namePackage)
	destFilename := path.Join(destDir, nameSourceFile)

	file, noChange, err := generateSource(destFilename, *flagSrc, *flagExternals)
	if err != nil {
		exitWithError(err)
	}

	if !noChange {
		err = os.MkdirAll(destDir, 0755)
		if err != nil {
			exitWithError(err)
		}

		src := file.Name()

		hSrc, err := shasum(src)
		if err != nil {
			exitWithError(err)
		}
		hDest, err := shasum(destFilename)
		if err != nil {
			exitWithError(err)
		}

		if !bytes.Equal(hSrc, hDest) {
			err = rename(src, destFilename)
			if err != nil {
				exitWithError(err)
			}
			fmt.Println("asset file updated successfully")
		} else {
			fmt.Println("asset file left unchanged")
		}
	} else {
		fmt.Println("asset file left unchanged")
	}
}

func shasum(file string) ([]byte, error) {
	h := sha256.New()
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// rename tries to os.Rename, but fall backs to copying from src
// to dest and unlink the source if os.Rename fails.
func rename(src, dest string) error {
	// Try to rename generated source.
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	// If the rename failed (might do so due to temporary file residing on a
	// different device), try to copy byte by byte.
	rc, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		rc.Close()
		os.Remove(src) // ignore the error, source is in tmp.
	}()

	if _, err = os.Stat(dest); !os.IsNotExist(err) {
		if *flagForce {
			if err = os.Remove(dest); err != nil {
				return fmt.Errorf("file %q could not be deleted", dest)
			}
		} else {
			return fmt.Errorf("file %q already exists; use -f to overwrite", dest)
		}
	}

	wc, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer wc.Close()

	if _, err = io.Copy(wc, rc); err != nil {
		// Delete remains of failed copy attempt.
		os.Remove(dest)
	}
	return err
}

func loadAsset(name, srcPath string) (*asset, error) {
	data := new(bytes.Buffer)

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	r := io.TeeReader(f, h)
	size, err := io.Copy(data, r)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(srcPath, name)
	if err != nil {
		return nil, err
	}

	return &asset{
		name:   path.Join("/", filepath.ToSlash(relPath)),
		size:   size,
		sha256: h.Sum(nil),
		data:   data.Bytes(),
	}, nil
}

// Walks on the source path and generates source code
// that contains source directory's contents as zip contents.
// Generates source registers generated zip contents data to
// be read by the statik/fs HTTP file system.
func generateSource(destFilename, srcPath, externalsFile string) (f *os.File, noChange bool, err error) {
	var assets []*asset

	currentAssets, err := readCurrentAssets(destFilename)
	if err != nil {
		return
	}

	doneCh := make(chan error)
	filesCh := make(chan string)
	assetsCh := make(chan *asset)

	go func() {
		defer close(filesCh)
		err = filepath.Walk(srcPath, func(name string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// Ignore directories and hidden assets.
			// No entry is needed for directories in a zip file.
			// Each file is represented with a path, no directory
			// entities are required to build the hierarchy.
			if !fi.IsDir() && !strings.HasPrefix(fi.Name(), ".") {
				filesCh <- name
			}
			return nil
		})
		if err != nil {
			doneCh <- err
		}
	}()

	for i := 0; i < 16; i++ {
		go func() {
			for name := range filesCh {
				asset, err := loadAsset(name, srcPath)
				if err != nil {
					doneCh <- err
					return
				}
				assetsCh <- asset
			}
			doneCh <- nil
		}()
	}

	go func() {
		defer close(assetsCh)
		for i := 0; i < 16; i++ {
			if err = <-doneCh; err != nil {
				return
			}
		}
	}()

	for a := range assetsCh {
		assets = append(assets, a)
	}
	if err != nil {
		return
	}

	if externalsFile != "" {
		var exts []*asset
		exts, err = downloadExternals(externalsFile, currentAssets)
		if err != nil {
			return
		}
		assets = append(assets, exts...)
	}

	sort.Slice(assets, func(i, j int) bool {
		return assets[i].name < assets[j].name
	})

	if len(assets) == len(currentAssets) {
		noChange = true
		for i, a := range assets {
			old := currentAssets[i]
			if old.name != a.name || !bytes.Equal(old.sha256, a.sha256) {
				noChange = false
				break
			}
		}
	}
	if noChange {
		return
	}

	f, err = ioutil.TempFile("", namePackage)
	if err != nil {
		return
	}

	_, err = fmt.Fprintf(f, `// Code generated by statik. DO NOT EDIT.

package %s

import (
	"github.com/cozy/cozy-stack/pkg/statik/fs"
)

func init() {
	data := `, namePackage)
	if err != nil {
		return
	}

	_, err = fmt.Fprint(f, "`")
	if err != nil {
		return
	}

	err = printZipData(f, assets)
	if err != nil {
		return
	}

	_, err = fmt.Fprint(f, "`")
	if err != nil {
		return
	}
	_, err = fmt.Fprint(f, `
	fs.Register(data)
}
`)
	if err != nil {
		return
	}

	return
}

func downloadExternals(filename string, currentAssets []*asset) (newAssets []*asset, err error) {
	externalAssets, err := parseExternalsFile(filename)
	if err != nil {
		return
	}

	currentAssetsMap := make(map[string]*asset)
	for _, a := range currentAssets {
		currentAssetsMap[a.name] = a
	}

	for _, externalAsset := range externalAssets {
		var newAsset *asset
		if a, ok := currentAssetsMap[externalAsset.name]; ok && bytes.Equal(a.sha256, externalAsset.sha256) {
			newAsset = a
		} else {
			fmt.Printf("downloading %q... ", externalAsset.name)
			newAsset, err = downloadExternal(externalAsset)
			if err != nil {
				return
			}
			fmt.Printf("ok (%s)\n", humanize.Bytes(uint64(newAsset.size)))
		}
		newAssets = append(newAssets, newAsset)
	}

	return
}

func parseExternalsFile(filename string) (assets []*asset, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer func() {
		if errc := f.Close(); errc != nil && err == nil {
			err = errc
		}
	}()

	var a *asset
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 && line[0] == '#' {
			continue
		}
		fields := strings.Fields(line)
		switch len(fields) {
		case 0:
			if a != nil {
				return nil, errExternalsMalformed
			}
		case 2:
			if a == nil {
				a = new(asset)
			}
			k, v := fields[0], fields[1]
			switch strings.ToLower(k) {
			case "name":
				a.name = path.Join("/", v)
			case "url":
				a.url = v
			case "sha256":
				a.sha256, err = hex.DecodeString(v)
				if err != nil {
					return nil, errExternalsMalformed
				}
			}
		default:
			return nil, errExternalsMalformed
		}
		if a != nil && a.name != "" && a.url != "" && len(a.sha256) > 0 {
			assets = append(assets, a)
			a = nil
		}
	}

	return
}

func downloadExternal(ext *asset) (f *asset, err error) {
	res, err := http.Get(ext.url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not fetch external assets %q: received status \"%d %s\"",
			ext.url, res.StatusCode, res.Status)
	}

	h := sha256.New()
	r := io.TeeReader(res.Body, h)

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("could not fetch external asset: %s", err)
	}

	if sum := h.Sum(nil); !bytes.Equal(sum, ext.sha256) {
		return nil, fmt.Errorf("shasum does not match: expected %x got %x",
			ext.sha256, sum)
	}

	return &asset{
		data:   data,
		name:   ext.name,
		size:   int64(len(data)),
		sha256: ext.sha256,
	}, nil
}

func readCurrentAssets(filename string) (assets []*asset, err error) {
	statikFile, err := ioutil.ReadFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var zippedData []byte
	if len(statikFile) > 0 {
		i := bytes.Index(statikFile, []byte("`"))
		if i >= 0 {
			j := bytes.Index(statikFile[i+1:], []byte("`"))
			if i >= 0 && j > i {
				zippedData = statikFile[i+1 : i+j]
			}
		}
	}

	for {
		block, rest := pem.Decode(zippedData)
		if block == nil {
			break
		}
		var size int64
		size, err = strconv.ParseInt(block.Headers["Size"], 10, 64)
		if err != nil {
			return
		}
		var gr *gzip.Reader
		gr, err = gzip.NewReader(bytes.NewReader(block.Bytes))
		if err != nil {
			return
		}
		var data []byte
		h := sha256.New()
		r := io.TeeReader(gr, h)
		data, err = ioutil.ReadAll(r)
		if err != nil {
			return
		}
		if err = gr.Close(); err != nil {
			return
		}
		name := block.Headers["Name"]
		assets = append(assets, &asset{
			name:   name,
			size:   size,
			data:   data,
			sha256: h.Sum(nil),
		})
		zippedData = rest
	}
	return
}

// printZipData converts zip binary contents to a string literal.
func printZipData(dest io.Writer, assets []*asset) error {
	for _, f := range assets {
		b := new(bytes.Buffer)
		gw, err := gzip.NewWriterLevel(b, gzip.BestCompression)
		if err != nil {
			return err
		}
		_, err = io.Copy(gw, bytes.NewReader(f.data))
		if err != nil {
			return err
		}
		err = gw.Close()
		if err != nil {
			return err
		}
		err = pem.Encode(dest, &pem.Block{
			Type:  "COZY ASSET",
			Bytes: b.Bytes(),
			Headers: map[string]string{
				"Name": f.name,
				"Size": strconv.FormatInt(f.size, 10),
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Prints out the error message and exists with a non-success signal.
func exitWithError(err error) {
	fmt.Println(err)
	os.Exit(1)
}
