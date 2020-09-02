package initials

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// Image returns an image with the initials for the given name (and the
// content-type to use for the HTTP response).
// TODO add cache
func Image(publicName string) ([]byte, string, error) {
	name := strings.TrimSpace(publicName)
	info := extractInfo(name)
	bytes, err := draw(info)
	if err != nil {
		return nil, "", err
	}
	return bytes, "image/png", nil
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

type info struct {
	initials string
	color    string
}

func extractInfo(name string) info {
	initials := getInitials(name)
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

func draw(info info) ([]byte, error) {
	var env []string
	{
		tempDir, err := ioutil.TempDir("", "magick")
		if err == nil {
			defer os.RemoveAll(tempDir)
			envTempDir := fmt.Sprintf("MAGICK_TEMPORARY_PATH=%s", tempDir)
			env = []string{envTempDir}
		}
	}

	convertCmd := config.GetConfig().Jobs.ImageMagickConvertCmd
	if convertCmd == "" {
		convertCmd = "convert"
	}

	// convert -size 128x128 null: -fill blue -draw 'circle 64,64 0,64' -fill white -font Lato-Regular
	// -pointsize 64 -gravity center -annotate "+0,+0" "AM" foo.png
	args := []string{
		"-limit", "Memory", "1GB",
		"-limit", "Map", "1GB",
		// Use a transparent background
		"-size", "128x128",
		"null:",
		// Add a cicle of color
		"-fill", info.color,
		"-draw", "circle 64,64 0,64",
		// Add the initials
		"-fill", "white",
		"-font", "Lato-Regular",
		"-pointsize", "64",
		"-gravity", "center",
		"-annotate", "+0,+0",
		info.initials,
		// Use the colorspace recommended for web, sRGB
		"-colorspace", "sRGB",
		// Send the output on stdout, in PNG format
		"png:-",
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(context.Background(), convertCmd, args...)
	cmd.Env = env
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logger.WithNamespace("initials").
			WithField("stderr", stderr.String()).
			WithField("initials", info.initials).
			WithField("color", info.color).
			Errorf("imagemagick failed: %s", err)
		return nil, err
	}
	return stdout.Bytes(), nil
}
