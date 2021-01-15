package vfs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// ServePDFPreview will send the preview image for a PDF.
func ServePDFPreview(w http.ResponseWriter, req *http.Request, fs VFS, doc *FileDoc) error {
	name := fmt.Sprintf("%s-preview.jpg", doc.ID())
	modtime := doc.UpdatedAt
	if doc.CozyMetadata != nil && doc.CozyMetadata.UploadedAt != nil {
		modtime = *doc.CozyMetadata.UploadedAt
	}
	buf, err := generatePreview(fs, doc)
	if err != nil {
		return err
	}
	http.ServeContent(w, req, name, modtime, bytes.NewReader(buf.Bytes()))
	return nil
}

func generatePreview(fs VFS, doc *FileDoc) (*bytes.Buffer, error) {
	f, err := fs.OpenFile(doc)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tempDir, err := ioutil.TempDir("", "magick")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	envTempDir := fmt.Sprintf("MAGICK_TEMPORARY_PATH=%s", tempDir)
	env := []string{envTempDir}

	convertCmd := config.GetConfig().Jobs.ImageMagickConvertCmd
	if convertCmd == "" {
		convertCmd = "convert"
	}
	args := []string{
		"-limit", "Memory", "2GB",
		"-limit", "Map", "3GB",
		"-[0]",           // Takes the input from stdin
		"-quality", "82", // A good compromise between file size and quality
		"-interlace", "none", // Don't use progressive JPEGs, they are heavier
		"-thumbnail", "720x1280>", // Makes a thumbnail that fits inside the given format
		"-colorspace", "sRGB", // Use the colorspace recommended for web, sRGB
		"jpg:-", // Send the output on stdout, in JPEG format
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(convertCmd, args...)
	cmd.Env = env
	cmd.Stdin = f
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Truncate very long messages
		msg := stderr.String()
		if len(msg) > 4000 {
			msg = msg[:4000]
		}
		logger.WithNamespace("pdf_preview").
			WithField("stderr", msg).
			WithField("file_id", doc.ID()).
			Errorf("imagemagick failed: %s", err)
		return nil, err
	}
	return &stdout, nil
}
