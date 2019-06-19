package dynamic

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/swift"
	"github.com/hashicorp/go-multierror"
)

// DynamicAssetsContainerName is the Swift container name for dynamic assets
const DynamicAssetsContainerName = "__dyn-assets__"

var assetsClient = &http.Client{
	Timeout: 30 * time.Second,
}

// List dynamic assets
func ListAssets() (map[string][]*model.Asset, error) {
	swiftConn := config.GetSwiftConnection()

	objs := map[string][]*model.Asset{}

	objNames, err := swiftConn.ObjectNamesAll(DynamicAssetsContainerName, nil)
	if err != nil {
		return nil, err
	}

	for _, obj := range objNames {
		splitted := strings.SplitN(obj, "/", 2)
		ctx := splitted[0]
		assetName := model.NormalizeAssetName(splitted[1])

		a, err := GetAsset(ctx, assetName)
		if err != nil {
			return nil, err
		}

		objs[ctx] = append(objs[ctx], a)

	}

	return objs, nil
}

// GetAsset retrieves a raw asset from Swift and build a fs.Asset
func GetAsset(context, name string) (*model.Asset, error) {
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

	asset := model.NewAsset(model.AssetOption{
		Shasum:   sumx,
		Name:     name,
		Context:  context,
		IsCustom: true,
	}, zippedContent, content)

	return asset, nil
}

// RemoveAsset removes a dynamic asset from Swift
func RemoveAsset(context, name string) error {
	swiftConn := config.GetSwiftConnection()
	objectName := path.Join(context, name)

	return swiftConn.ObjectDelete(DynamicAssetsContainerName, objectName)
}

// Initializes the Swift container for dynamic assets
func InitDynamicAssetContainer() error {
	swiftConn := config.GetSwiftConnection()
	return swiftConn.ContainerCreate(DynamicAssetsContainerName, nil)
}

// RegisterCustomExternals ensures that the assets are in the Swift, and load
// them from their source if they are not yet available.
func RegisterCustomExternals(opts []model.AssetOption, maxTryCount int) error {
	if len(opts) == 0 {
		return nil
	}

	assetsCh := make(chan model.AssetOption)
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

func registerCustomExternal(opt model.AssetOption) error {
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

	asset := model.NewAsset(opt, zippedDataBuf.Bytes(), unzippedData)

	objectName := path.Join(asset.Context, asset.Name)
	swiftConn := config.GetSwiftConnection()

	f, err := swiftConn.ObjectCreate(DynamicAssetsContainerName, objectName, true, "", "", nil)
	if err != nil {
		return err
	}
	defer f.Close()

	// Writing the asset content to Swift
	_, err = f.Write(asset.GetUnzippedData())
	if err != nil {
		return err
	}

	return nil
}
