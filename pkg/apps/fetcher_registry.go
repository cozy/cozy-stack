package apps

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/url"
	"strings"

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
	var version *registry.Version
	var err error

	channel, vnumber := getRegistryChannel(src)

	if vnumber != "" {
		slug = strings.Split(slug, ":")[0]
		version, err = registry.GetVersion(slug, vnumber, f.registries)
	} else {
		version, err = registry.GetLatestVersion(slug, channel, f.registries)
	}

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

func getRegistryChannel(src *url.URL) (string, string) {
	var channel, version string

	splittedPath := strings.Split(src.String(), ":")
	rawChan := splittedPath[1]
	if len(rawChan) > 0 && strings.HasPrefix(rawChan, "//") {
		if channelSplitted := strings.Split(rawChan[2:], "/"); len(channelSplitted) == 2 {
			channel = channelSplitted[1]
		} else {
			channel = "stable"
		}
	}
	if len(splittedPath) == 3 && splittedPath[2] != "latest" {
		// Channel and version
		// splittedPath == [registry //freemobile/stable 1.0.2]
		version = splittedPath[2]
	}

	return channel, version
}
