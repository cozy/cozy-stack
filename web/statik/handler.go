package statik

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	web_utils "github.com/cozy/cozy-stack/web/utils"
	"github.com/cozy/echo"
)

var (
	templatesList = []string{
		"authorize.html",
		"authorize_app.html",
		"authorize_sharing.html",
		"compat.html",
		"error.html",
		"login.html",
		"need_onboarding.html",
		"passphrase_reset.html",
		"passphrase_renew.html",
		"passphrase_onboarding.html",
		"sharing_discovery.html",
		"instance_blocked.html",
	}
)

const (
	assetsPrefix    = "/assets"
	assetsExtPrefix = "/assets/ext"
)

// FuncsMap is a the helper functions used in templates
var FuncsMap template.FuncMap

// AssetRenderer is an interface for both a template renderer and an asset HTTP
// handler.
type AssetRenderer interface {
	echo.Renderer
	http.Handler
}

type dir string

func (d dir) Open(name string) (http.File, error) {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) {
		return nil, errors.New("http: invalid character in file path")
	}
	dir := string(d)
	if dir == "" {
		dir = "."
	}
	name, _ = ExtractAssetID(name)
	fullName := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+name)))
	f, err := os.Open(fullName)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// NewDirRenderer returns a renderer with assets opened from a specified local
// directory.
func NewDirRenderer(assetsPath string) (AssetRenderer, error) {
	list := make([]string, len(templatesList))
	for i, name := range templatesList {
		list[i] = filepath.Join(assetsPath, "templates", name)
	}

	t := template.New("stub")
	h := http.StripPrefix(assetsPrefix, http.FileServer(dir(assetsPath)))
	FuncsMap = template.FuncMap{
		"t":     fmt.Sprintf,
		"split": strings.Split,
		"asset": assetPath,
	}

	var err error
	t, err = t.Funcs(FuncsMap).ParseFiles(list...)
	if err != nil {
		return nil, fmt.Errorf("Can't load the assets from %q: %s", assetsPath, err)
	}

	return &renderer{t: t, Handler: h}, nil
}

// NewRenderer return a renderer with assets loaded form their packed
// representation into the binary.
func NewRenderer() (AssetRenderer, error) {
	t := template.New("stub")

	FuncsMap = template.FuncMap{
		"t":     fmt.Sprintf,
		"split": strings.Split,
		"asset": AssetPath,
	}

	for _, name := range templatesList {
		tmpl := t.New(name).Funcs(FuncsMap)
		f, err := fs.Open("/templates/" + name)
		if err != nil {
			return nil, fmt.Errorf("Can't load asset %q: %s", name, err)
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}
		if _, err = tmpl.Parse(string(b)); err != nil {
			return nil, err
		}
	}

	return &renderer{t: t, Handler: NewHandler()}, nil
}

type renderer struct {
	http.Handler
	t *template.Template
}

func (r *renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	var funcMap template.FuncMap
	i, ok := middlewares.GetInstanceSafe(c)
	if ok {
		funcMap = template.FuncMap{"t": i.Translate}
	} else {
		lang := GetLanguageFromHeader(c.Request().Header)
		funcMap = template.FuncMap{"t": i18n.Translator(lang)}
	}
	t, err := r.t.Clone()
	if err != nil {
		return err
	}
	return t.Funcs(funcMap).ExecuteTemplate(w, name, data)
}

// AssetPath return the fullpath with unique identifier for a given asset file.
func AssetPath(domain, name string, context ...string) string {
	f, ok := fs.Get(name, context...)
	if !ok && len(context) > 0 && context[0] != "" {
		// fallback on default context if asset is not found in the given one.
		f, ok = fs.Get(name)
		if ok {
			context = nil
		}
	}
	if ok {
		name = f.NameWithSum
	}
	return assetPath(domain, name, context...)
}

func assetPath(domain, name string, context ...string) string {
	if len(context) > 0 && context[0] != "" {
		name = path.Join(assetsExtPrefix, url.PathEscape(context[0]), name)
	} else {
		name = path.Join(assetsPrefix, name)
	}
	if domain != "" {
		return "//" + domain + name
	}
	return name
}

// Handler implements http.handler for a subpart of the available assets on a
// specified prefix.
type Handler struct{}

// NewHandler returns a new handler
func NewHandler() Handler {
	return Handler{}
}

// ServeHTTP implements the http.Handler interface.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// The URL path should be formed in one on those forms:
	// /assets/:file...
	// /assets/ext/(:context-name)/:file...

	var id, name, context string

	if strings.HasPrefix(r.URL.Path, assetsExtPrefix+"/") {
		nameWithContext := strings.TrimPrefix(r.URL.Path, assetsExtPrefix+"/")
		nameWithContextSplit := strings.SplitN(nameWithContext, "/", 2)
		if len(nameWithContextSplit) != 2 {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		context = nameWithContextSplit[0]
		name = nameWithContextSplit[1]
	} else {
		name = strings.TrimPrefix(r.URL.Path, assetsPrefix)
	}

	name, id = ExtractAssetID(name)
	if len(name) > 0 && name[0] != '/' {
		name = "/" + name
	}

	f, ok := fs.Get(name, context)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	checkETag := id == ""
	h.ServeFile(w, r, f, checkETag)
}

// ServeFile can be used to respond with an asset file to an HTTP request
func (h *Handler) ServeFile(w http.ResponseWriter, r *http.Request, f *fs.Asset, checkETag bool) {
	if checkETag && web_utils.CheckPreconditions(w, r, f.Etag) {
		return
	}

	headers := w.Header()
	headers.Set("Content-Type", f.Mime)
	headers.Set("Content-Length", f.Size())
	headers.Add("Vary", "Accept-Encoding")

	acceptsGZIP := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	if acceptsGZIP {
		headers.Set("Content-Encoding", "gzip")
		headers.Set("Content-Length", f.GzipSize())
	} else {
		headers.Set("Content-Length", f.Size())
	}

	if checkETag {
		headers.Set("Etag", f.Etag)
		headers.Set("Cache-Control", "no-cache, public")
	} else {
		headers.Set("Cache-Control", "max-age=31536000, public, immutable")
	}

	if r.Method == http.MethodGet {
		if acceptsGZIP {
			io.Copy(w, f.GzipReader())
		} else {
			io.Copy(w, f.Reader())
		}
	}
}

// GetLanguageFromHeader return the language tag given the Accept-Language
// header.
func GetLanguageFromHeader(header http.Header) (lang string) {
	// TODO: improve language detection with a package like
	// "golang.org/x/text/language"
	lang = i18n.DefaultLocale
	acceptHeader := header.Get("Accept-Language")
	if acceptHeader == "" {
		return
	}
	acceptLanguages := utils.SplitTrimString(acceptHeader, ",")
	for _, tag := range acceptLanguages {
		// tag may contain a ';q=' for a quality factor that we do not take into
		// account.
		if i := strings.Index(tag, ";q="); i >= 0 {
			tag = tag[:i]
		}
		// tag may contain a '-' to introduce a country variante, that we do not
		// take into account.
		if i := strings.IndexByte(tag, '-'); i >= 0 {
			tag = tag[:i]
		}
		if utils.IsInArray(tag, i18n.SupportedLocales) {
			lang = tag
			return
		}
	}
	return
}

// ExtractAssetID checks if a long hexadecimal string is contained in given
// file path and returns the original file name and ID (if any). For instance
// <foo.badbeedbadbeef.min.js> = <foo.min.js, badbeefbadbeef>
func ExtractAssetID(file string) (string, string) {
	var id string
	base := path.Base(file)
	off1 := strings.IndexByte(base, '.') + 1
	if off1 < len(base) {
		off2 := off1 + strings.IndexByte(base[off1:], '.')
		if off2 > off1 {
			if s := base[off1:off2]; isLongHexString(s) || s == "immutable" {
				dir := path.Dir(file)
				id = s
				file = base[:off1-1] + base[off2:]
				if dir != "." {
					file = path.Join(dir, file)
				}
			}
		}
	}
	return file, id
}

func isLongHexString(s string) bool {
	if len(s) < 10 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
