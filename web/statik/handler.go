package statik

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	web_utils "github.com/cozy/cozy-stack/web/utils"
	"github.com/cozy/echo"
	"github.com/cozy/statik/fs"
)

var (
	templatesList = []string{
		"authorize.html",
		"authorize_app.html",
		"authorize_sharing.html",
		"error.html",
		"login.html",
		"need_onboarding.html",
		"passphrase_reset.html",
		"passphrase_renew.html",
		"sharing_discovery.html",
	}

	privateAssets = []string{
		"/templates/",
		"/locales/",
		"/placeholders/",
	}
)

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
	h := http.StripPrefix("/assets", http.FileServer(dir(assetsPath)))
	funcsMap := template.FuncMap{
		"t":     fmt.Sprintf,
		"split": strings.Split,
		"asset": func(domain, file string) string {
			return AssetResolver(domain, path.Join("/assets", file))
		},
	}

	var err error
	t, err = t.Funcs(funcsMap).ParseFiles(list...)
	if err != nil {
		return nil, fmt.Errorf("Can't load the assets from %q: %s", assetsPath, err)
	}

	return &renderer{t: t, Handler: h}, nil
}

// NewRenderer return a renderer with assets loaded form their packed
// representation into the binary.
func NewRenderer() (AssetRenderer, error) {
	t := template.New("stub")
	h := NewHandler(Options{
		Prefix:   "/assets",
		Privates: privateAssets,
	})
	funcsMap := template.FuncMap{
		"t":     fmt.Sprintf,
		"split": strings.Split,
		"asset": func(domain, file string) string {
			return AssetResolver(domain, h.AssetPath(file))
		},
	}

	for _, name := range templatesList {
		tmpl := t.New(name).Funcs(funcsMap)
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

	return &renderer{t: t, Handler: h}, nil
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

// AssetResolver is a template helper returning a complete URL, with domain
// name, for a given asset path.
func AssetResolver(domain, file string) string {
	if domain != "" {
		return "//" + domain + file
	}
	return file
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

// Handler implements http.Handler for a subpart of the available assets on a
// specified prefix.
type Handler struct {
	prefix string
	files  map[string]*fs.Asset
}

// Options contains the different options to create an asset handler.
type Options struct {
	Prefix   string
	Privates []string
}

// NewHandler returns a new handler
func NewHandler(opts Options) *Handler {
	files := make(map[string]*fs.Asset)
	fs.Foreach(func(name string, f *fs.Asset) {
		isPrivate := false
		for _, p := range opts.Privates {
			if strings.HasPrefix(name, p) {
				isPrivate = true
				break
			}
		}
		if !isPrivate {
			files[name] = f
		}
	})
	return &Handler{
		prefix: opts.Prefix,
		files:  files,
	}
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

// AssetPath return the fullpath with unique identifier for a given asset file.
func (h *Handler) AssetPath(file string) string {
	f, ok := h.files[file]
	if !ok {
		return h.prefix + file
	}
	return h.prefix + f.Name()
}

// ServeHTTP implements the http.Handler interface.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var id string
	file := strings.TrimPrefix(r.URL.Path, h.prefix)
	file, id = ExtractAssetID(file)
	if len(file) > 0 && file[0] != '/' {
		file = "/" + file
	}
	f, ok := h.files[file]
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	checkETag := id == ""
	if checkETag && web_utils.CheckPreconditions(w, r, f.Etag()) {
		return
	}

	headers := w.Header()
	headers.Set("Content-Type", f.Mime())
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
		headers.Set("Etag", f.Etag())
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
