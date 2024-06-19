// Package pdf is for manipulating PDF files.
package pdf

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"

	"github.com/cozy/cozy-stack/pkg/logger"
)

// Service provides methods for manipulating PDF files.
type Service struct {
	ghostscriptCmd string
}

// NewService instantiate a new [Service].
func NewService(ghostscriptCmd string) *Service {
	return &Service{ghostscriptCmd}
}

// ExtractPage extract a page from a PDF.
func (s *Service) ExtractPage(stdin io.Reader, page int) (*bytes.Buffer, error) {
	args := []string{
		"-q",
		"-sDEVICE=pdfwrite",
		"-dNOPAUSE",
		"-dBATCH",
		"-dSAFER",
		fmt.Sprintf("-dFirstPage=%d", page),
		fmt.Sprintf("-dLastPage=%d", page),
		"-sOutputFile=-",
		"-", // Use stdin for input
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(s.ghostscriptCmd, args...)
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.WithNamespace("pdf").
			WithField("stderr", stderr.String()).
			Errorf("ghostscript failed: %s", err)
		return nil, fmt.Errorf("failed to run the cmd %q: %w", s.ghostscriptCmd, err)
	}
	return &stdout, nil
}
