package apps

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/sirupsen/logrus"
)

type registryFetcher struct {
	log        *logrus.Entry
	registries []*url.URL
	version    *registry.Version
}

func newRegistryFetcher(registries []*url.URL, log *logrus.Entry) Fetcher {
	return &registryFetcher{log: log, registries: registries}
}

func (f *registryFetcher) FetchManifest(src *url.URL) (io.ReadCloser, error) {
	slug := src.Host
	if slug == "" {
		return nil, ErrManifestNotReachable
	}
	channel := getRegistryChannel(src)
	version, err := registry.GetLatestVersion(slug, channel, f.registries)
	if err != nil {
		f.log.Infof("Could not fetch manifest for %s: %s", src.String(), err.Error())
		return nil, ErrManifestNotReachable
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
	man.SetVersion(v.Version)
	return fetchHTTP(u, shasum, fs, man, v.TarPrefix)
}

func getRegistryChannel(src *url.URL) string {
	channel := src.Path
	if len(channel) > 0 && channel[0] == '/' {
		channel = channel[1:]
	}
	if channel == "" {
		channel = "stable"
	}
	return channel
}
