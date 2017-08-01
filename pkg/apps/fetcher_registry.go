package apps

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/apps/registry"
	"github.com/sirupsen/logrus"
)

type registryFetcher struct {
	log     *logrus.Entry
	version *registry.Version
}

func newRegistryFetcher(manFilename string, log *logrus.Entry) Fetcher {
	return &registryFetcher{log: log}
}

func (f *registryFetcher) FetchManifest(src *url.URL) (r io.ReadCloser, err error) {
	slug := src.Host
	version, err := registry.GetAppLatestVersion(slug)
	if err != nil {
		return nil, err
	}
	f.version = version
	return ioutil.NopCloser(bytes.NewBuffer(version.Manifest)), nil
}

func (f *registryFetcher) Fetch(src *url.URL, fs Copier, man Manifest) error {
	v := f.version
	shasum, err := hex.DecodeString(v.Sha256)
	if err != nil {
		return err
	}
	u, err := url.Parse(v.URL)
	if err != nil {
		return err
	}
	return fetchHTTP(u, shasum, fs, man, v.TarPrefix)
}
