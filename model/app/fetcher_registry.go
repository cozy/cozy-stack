package app

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/url"
	"regexp"
	"strings"

	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/logger"
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

func (f *registryFetcher) Fetch(src *url.URL, fs appfs.Copier, man Manifest) error {
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
	man.SetChecksum(v.Sha256)
	return fetchHTTP(u, shasum, fs, man, v.TarPrefix)
}

func getRegistryChannel(src *url.URL) (string, string) {
	var channel, version string
	channel = "stable"
	splittedPath := strings.Split(src.String(), "/")
	log := logger.WithNamespace("fetcher_registry")
	switch len(splittedPath) {
	case 4: // Either channel or version
		channelOrVersion := splittedPath[3]

		// If it starts with a number, it is the version
		versionRegex := "^\\d"
		matched, err := regexp.MatchString(versionRegex, channelOrVersion)
		if err != nil {
			log.Errorf("fetcher_registry: Bad format for %s", src.String())
			return "", ""
		}
		if matched {
			version = channelOrVersion
		} else {
			channel = channelOrVersion
		}
	case 5: // Channel and version
		channel = splittedPath[3]
		version = splittedPath[4]
	}

	if version == "latest" {
		version = ""
	}

	return channel, version
}
