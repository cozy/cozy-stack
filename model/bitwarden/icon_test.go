package bitwarden

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateDomain(t *testing.T) {
	err := validateDomain("")
	assert.Equal(t, err.Error(), "Unauthorized domain")

	err = validateDomain("foo bar baz")
	assert.Equal(t, err.Error(), "Invalid domain")

	err = validateDomain("192.168.0.1")
	assert.Equal(t, err.Error(), "IP address are not authorized")

	err = validateDomain("example.com")
	assert.NoError(t, err)
}

func TestCandidateIcons(t *testing.T) {
	html := `<html><head><title>Foo</title></head><body><h1>Foo</h1></body></html>`
	candidates := getCandidateIcons("example.com", strings.NewReader(html))
	assert.Len(t, candidates, 0)

	html = `<!DOCTYPE html>
<html lang="fr">
<head>
<meta charset="utf-8">
<title>Accueil - LinuxFr.org</title>
<style type="text/css">header#branding h1 { background-image: url(/images/logos/linuxfr2_gouttes.png) }</style>
<link rel="stylesheet" href="/assets/application-e25e004c56c9986c6dc2b7d54b8640425a021edaa68e2948eff1c8dbc668caa0.css" />
<link rel="shortcut icon" type="image/x-icon" href="/favicon.png" />
<link rel="alternate" type="application/atom+xml" title="Flux Atom des dépêches" href="/news.atom" />
</head>
<body class="logged admin" id="home-index">
...
</body>
</html>
`
	candidates = getCandidateIcons("linuxfr.org", strings.NewReader(html))
	assert.Len(t, candidates, 1)
	assert.Equal(t, candidates[0], "https://linuxfr.org/favicon.png")

	html = `<!DOCTYPE html>
	<html>
	<head>
	<link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png">
	<link rel="icon" type="image/png" href="./favicon-32x32.png" sizes="32x32">
	<link rel="icon" type="image/png" href="./favicon-16x16.png" sizes="16x16">
	</head>
	<body>
	...
	</body>
	</html>`
	candidates = getCandidateIcons("example.com", strings.NewReader(html))
	assert.Len(t, candidates, 3)
	assert.Equal(t, candidates[0], "https://example.com/favicon-32x32.png")
	assert.Equal(t, candidates[1], "https://example.com/favicon-16x16.png")
	assert.Equal(t, candidates[2], "https://example.com/apple-touch-icon.png")

	html = `<!DOCTYPE html>
<html>
<head>
<link rel="apple-touch-icon" href="https://static.example.org/apple-touch-icon.png" />
<link rel="icon" type="image/png" href="./images/favicon.png">
</head>
<body>
...
</body>
</html>`
	candidates = getCandidateIcons("example.com", strings.NewReader(html))
	assert.Len(t, candidates, 2)
	assert.Equal(t, candidates[0], "https://example.com/images/favicon.png")
	assert.Equal(t, candidates[1], "https://static.example.org/apple-touch-icon.png")
}

func TestDownloadIcon(t *testing.T) {
	icon, err := downloadFavicon("github.com")
	assert.NoError(t, err)
	assert.Equal(t, icon.Mime, "image/x-icon")

	icon, err = downloadIcon("https://github.githubassets.com/favicon.ico")
	assert.NoError(t, err)
	assert.Equal(t, icon.Mime, "image/vnd.microsoft.icon")

	icon, err = fetchIcon("github.com")
	assert.NoError(t, err)
	assert.Equal(t, icon.Mime, "image/vnd.microsoft.icon")
}
