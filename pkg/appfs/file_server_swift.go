package appfs

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/cozy/cozy-stack/pkg/consts"
	web_utils "github.com/cozy/cozy-stack/pkg/utils"
	"github.com/labstack/echo/v4"
	"github.com/ncw/swift/v2"
)

// SwiftServer is a [FileServer] implementation based on the Swift protocol.
type SwiftServer struct {
	c         *swift.Connection
	container string
	ctx       context.Context
}

// NewSwiftFileServer instantiate a new [SwiftServer] with the giver conn as backend.
func NewSwiftFileServer(conn *swift.Connection, appsType consts.AppType) *SwiftServer {
	return &SwiftServer{
		c:         conn,
		container: containerName(appsType),
		ctx:       context.Background(),
	}
}

func (s *SwiftServer) Open(slug, version, shasum, file string) (io.ReadCloser, error) {
	objName := s.makeObjectName(slug, version, shasum, file)
	f, h, err := s.c.ObjectOpen(s.ctx, s.container, objName, false, nil)
	if err != nil {
		return nil, wrapSwiftErr(err)
	}
	o := h.ObjectMetadata()
	contentEncoding := o["content-encoding"]
	if contentEncoding == "br" {
		return newBrotliReadCloser(f)
	} else if contentEncoding == "gzip" {
		return newGzipReadCloser(f)
	}
	return f, nil
}

func (s *SwiftServer) ServeFileContent(w http.ResponseWriter, req *http.Request, slug, version, shasum, file string) error {
	objName := s.makeObjectName(slug, version, shasum, file)
	f, h, err := s.c.ObjectOpen(s.ctx, s.container, objName, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	if checkETag := req.Header.Get("Cache-Control") == ""; checkETag {
		etag := fmt.Sprintf(`"%s"`, h["Etag"][:10])
		if web_utils.CheckPreconditions(w, req, etag) {
			return nil
		}
		w.Header().Set("Etag", etag)
	}

	var r io.Reader = f
	contentLength := h["Content-Length"]
	contentType := h["Content-Type"]
	o := h.ObjectMetadata()
	contentEncoding := o["content-encoding"]
	if contentEncoding == "br" {
		if acceptBrotliEncoding(req) {
			w.Header().Set(echo.HeaderContentEncoding, "br")
		} else {
			contentLength = o["original-content-length"]
			r = brotli.NewReader(f)
		}
	} else if contentEncoding == "gzip" {
		if acceptGzipEncoding(req) {
			w.Header().Set(echo.HeaderContentEncoding, "gzip")
		} else {
			contentLength = o["original-content-length"]
			var gr *gzip.Reader
			gr, err = gzip.NewReader(f)
			if err != nil {
				return err
			}
			defer gr.Close()
			r = gr
		}
	}

	ext := path.Ext(file)
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}
	if contentType == "text/xml" && ext == ".svg" {
		// override for files with text/xml content because of leading <?xml tag
		contentType = "image/svg+xml"
	}

	size, _ := strconv.ParseInt(contentLength, 10, 64)

	return serveContent(w, req, contentType, size, r)
}

func (s *SwiftServer) ServeCodeTarball(w http.ResponseWriter, req *http.Request, slug, version, shasum string) error {
	objName := path.Join(slug, version)
	if shasum != "" {
		objName += "-" + shasum
	}
	objName += ".tgz"

	f, h, err := s.c.ObjectOpen(s.ctx, s.container, objName, false, nil)
	if err == nil {
		defer f.Close()
		contentLength := h["Content-Length"]
		contentType := h["Content-Type"]
		size, _ := strconv.ParseInt(contentLength, 10, 64)

		return serveContent(w, req, contentType, size, f)
	}

	buf, err := prepareTarball(s, slug, version, shasum)
	if err != nil {
		return err
	}
	contentType := mime.TypeByExtension(".gz")

	file, err := s.c.ObjectCreate(s.ctx, s.container, objName, true, "", contentType, nil)
	if err == nil {
		_, _ = io.Copy(file, buf)
		_ = file.Close()
	}

	return serveContent(w, req, contentType, int64(buf.Len()), buf)
}

func (s *SwiftServer) makeObjectName(slug, version, shasum, file string) string {
	basepath := path.Join(slug, version)
	if shasum != "" {
		basepath += "-" + shasum
	}
	return path.Join(basepath, file)
}

func (s *SwiftServer) FilesList(slug, version, shasum string) ([]string, error) {
	prefix := s.makeObjectName(slug, version, shasum, "") + "/"
	names, err := s.c.ObjectNamesAll(s.ctx, s.container, &swift.ObjectsOpts{
		Prefix: prefix,
	})
	if err != nil {
		return nil, err
	}
	filtered := names[:0]
	for _, n := range names {
		n = strings.TrimPrefix(n, prefix)
		if n != "" {
			filtered = append(filtered, n)
		}
	}
	return filtered, nil
}
