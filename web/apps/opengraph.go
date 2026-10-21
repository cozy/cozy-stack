package apps

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/statik"
)

var openGraphTemplate = template.Must(template.New("opengraph").Parse(`
<meta property="og:type" content="website" />
<meta property="og:title" content="{{.Title}}" />
<meta property="og:description" content="{{.Description}}" />
<meta property="og:image" content="{{.Image}}" />
<meta property="og:image:type" content="image/png" />
<meta property="og:image:width" content="1200" />
<meta property="og:image:height" content="630" />
<meta name="twitter:card" content="summary_large_image" />`,
))

type openGraph struct {
	Title       string
	Description string
	Image       string
}

// A password-protected link must not leak anything about the shared file.
func newOpenGraph(inst *instance.Instance, webapp *app.WebappManifest, perm *permission.Permission) *openGraph {
	if perm.Password != nil {
		return nil
	}

	title := strings.TrimSpace(webapp.NamePrefix() + " " + webapp.NameLocalized(inst.Locale))
	dir, file := getSharedFileOrDir(inst, perm)
	if dir != nil {
		title = dir.DocName
	} else if file != nil {
		title = file.DocName
	}
	imageName := getOpenGraphImageName(dir, file)

	imagePath := statik.AssetPath(inst.ContextualDomain(), "/images/opengraph/"+imageName+".png", inst.ContextName)
	return &openGraph{
		Title:       title,
		Description: inst.Translate("Share by link OpenGraph Description"),
		Image:       inst.Scheme() + ":" + imagePath,
	}
}

func (og *openGraph) HTML() template.HTML {
	if og == nil {
		return ""
	}
	buf := new(bytes.Buffer)
	if err := openGraphTemplate.Execute(buf, og); err != nil {
		logger.WithNamespace("apps").Warnf("Cannot render the OpenGraph tags: %s", err)
		return ""
	}
	return template.HTML(buf.String())
}

func getSharedFileOrDir(inst *instance.Instance, perm *permission.Permission) (*vfs.DirDoc, *vfs.FileDoc) {
	if len(perm.Permissions) != 1 {
		return nil, nil
	}
	rule := perm.Permissions[0]
	if rule.Type != consts.Files || rule.Selector != "" || len(rule.Values) != 1 {
		return nil, nil
	}
	dir, file, err := inst.VFS().DirOrFileByID(rule.Values[0])
	if err != nil {
		return nil, nil
	}
	return dir, file
}

func getOpenGraphImageName(dir *vfs.DirDoc, file *vfs.FileDoc) string {
	if dir != nil {
		return "folder"
	}
	if file == nil {
		return "generic"
	}
	if file.Mime == consts.NoteMimeType {
		return "note"
	}
	switch file.Class {
	case "audio", "code", "image", "pdf", "slide", "spreadsheet", "text", "video", "zip":
		return file.Class
	}
	return "generic"
}
