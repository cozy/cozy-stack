package avatar

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/cozy/cozy-stack/pkg/logger"
)

// PNGInitials create PNG avatars with initials in it.
//
// This implementation is based on the `convert` binary.
type PNGInitials struct {
	cmd string
}

// NewPNGInitials instantiate a new [PNGInitials].
func NewPNGInitials(cmd string) *PNGInitials {
	if cmd == "" {
		cmd = "convert"
	}
	return &PNGInitials{cmd}
}

// ContentType return the generated avatar content-type.
func (a *PNGInitials) ContentType() string {
	return "image/png"
}

// Generate will create a new avatar with the given initials and color.
func (a *PNGInitials) Generate(initials, color string) ([]byte, error) {
	tempDir, err := os.MkdirTemp("", "magick")
	if err != nil {
		return nil, fmt.Errorf("failed to create the tempdir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	envTempDir := fmt.Sprintf("MAGICK_TEMPORARY_PATH=%s", tempDir)
	env := []string{envTempDir}

	// convert -size 128x128 null: -fill blue -draw 'circle 64,64 0,64' -fill white -font Lato-Regular
	// -pointsize 64 -gravity center -annotate "+0,+0" "AM" foo.png
	args := []string{
		"-limit", "Memory", "1GB",
		"-limit", "Map", "1GB",
		// Use a transparent background
		"-size", "128x128",
		"null:",
		// Add a cicle of color
		"-fill", color,
		"-draw", "circle 64,64 0,64",
		// Add the initials
		"-fill", "white",
		"-font", "Lato-Regular",
		"-pointsize", "64",
		"-gravity", "center",
		"-annotate", "+0,+0",
		initials,
		// Use the colorspace recommended for web, sRGB
		"-colorspace", "sRGB",
		// Send the output on stdout, in PNG format
		"png:-",
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(a.cmd, args...)
	cmd.Env = env
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logger.WithNamespace("initials").
			WithField("stderr", stderr.String()).
			WithField("initials", initials).
			WithField("color", color).
			Errorf("imagemagick failed: %s", err)
		return nil, fmt.Errorf("failed to run the cmd %q: %w", a.cmd, err)
	}
	return stdout.Bytes(), nil
}
