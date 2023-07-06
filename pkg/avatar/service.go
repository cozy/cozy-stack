package avatar

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cozy/cozy-stack/pkg/cache"
)

// Options can be used to give options for the generated image
type Options int

const (
	cacheTTL    = 30 * 24 * time.Hour // 1 month
	contentType = "image/png"

	// GreyBackground is an option to force a grey background
	GreyBackground Options = 1 + iota
)

// Initials is able to generate initial avatar.
type Initials interface {
	// Generate will create a new avatar with the given initials and color.
	Generate(initials, color string) ([]byte, error)
	ContentType() string
}

// Service handle all the interactions with the initials images.
type Service struct {
	cache    cache.Cache
	initials Initials
}

// NewService instantiate a new [Service].
func NewService(cache cache.Cache, cmd string) *Service {
	initials := NewPNGInitials(cmd)
	return &Service{cache, initials}
}

// GenerateInitials an image with the initials for the given name (and the
// content-type to use for the HTTP response).
func (s *Service) GenerateInitials(publicName string, opts ...Options) ([]byte, string, error) {
	name := strings.TrimSpace(publicName)
	info := extractInfo(name)
	for _, opt := range opts {
		if opt == GreyBackground {
			info.color = charcoalGrey
		}
	}

	key := "initials:" + info.initials + info.color
	if bytes, ok := s.cache.Get(key); ok {
		return bytes, contentType, nil
	}

	bytes, err := s.initials.Generate(info.initials, info.color)
	if err != nil {
		return nil, "", err
	}
	s.cache.Set(key, bytes, cacheTTL)
	return bytes, s.initials.ContentType(), nil
}

// See https://github.com/cozy/cozy-ui/blob/master/react/Avatar/index.jsx#L9-L26
// and https://docs.cozy.io/cozy-ui/styleguide/section-settings.html#kssref-settings-colors
var colors = []string{
	"#1FA8F1",
	"#FD7461",
	"#FC6D00",
	"#F52D2D",
	"#FF962F",
	"#FF7F1B",
	"#6984CE",
	"#7F6BEE",
	"#B449E7",
	"#40DE8E",
	"#0DCBCF",
	"#35CE68",
	"#3DA67E",
	"#C2ADF4",
	"#FFC644",
	"#FC4C83",
}

var charcoalGrey = "#32363F"

type info struct {
	initials string
	color    string
}

func extractInfo(name string) info {
	initials := strings.ToUpper(getInitials(name))
	color := getColor(name)
	return info{initials: initials, color: color}
}

func getInitials(name string) string {
	parts := strings.Split(name, " ")
	initials := make([]rune, 0, len(parts))
	for _, part := range parts {
		r, size := utf8.DecodeRuneInString(part)
		if size > 0 && unicode.IsLetter(r) {
			initials = append(initials, r)
		}
	}
	switch len(initials) {
	case 0:
		return "?"
	case 1:
		return string(initials)
	default:
		return string(initials[0]) + string(initials[len(initials)-1])
	}
}

func getColor(name string) string {
	sum := 0
	for i := 0; i < len(name); i++ {
		sum += int(name[i])
	}
	return colors[sum%len(colors)]
}
