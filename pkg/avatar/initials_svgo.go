package avatar

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	svg "github.com/ajstarks/svgo"
)

// SVGoInitials create SVG avatars with initials in it.
//
// This implementation is based on the `svgo` library.
type SVGoInitials struct {
}

// NewSVGoInitials instantiate a new [SVGoInitials].
func NewSVGoInitials() *SVGoInitials {
	return &SVGoInitials{}
}

// ContentType return the generated avatar content-type.
func (a *SVGoInitials) ContentType() string {
	return "image/svg+xml"
}

// Generate will create a new avatar with the given initials and color.
func (a *SVGoInitials) Generate(ctx context.Context, initials, color string) ([]byte, error) {
	var buf bytes.Buffer

	rgbBytes, err := hex.DecodeString(strings.TrimPrefix(color, "#"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the color: %w", err)
	}

	fontSize := 64

	width := 128
	height := 128
	canvas := svg.New(&buf)
	canvas.Start(width, height)
	canvas.Circle(width/2, height/2, 64, canvas.RGB(int(rgbBytes[0]), int(rgbBytes[1]), int(rgbBytes[2])))
	canvas.Text(width/2, height/2+fontSize/3, initials, fmt.Sprintf("text-anchor:middle;font-family:Roboto;font-size:%dpx;fill:white", fontSize))
	canvas.End()

	return buf.Bytes(), nil
}
