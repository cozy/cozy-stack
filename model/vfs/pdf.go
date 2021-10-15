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
	"github.com/cozy/cozy-stack/pkg/previewfs"
)

// ServePDFIcon will send the icon image for a PDF.
func ServePDFIcon(w http.ResponseWriter, req *http.Request, fs VFS, doc *FileDoc) error {
	name := fmt.Sprintf("%s-icon.jpg", doc.ID())
	modtime := doc.UpdatedAt
	if doc.CozyMetadata != nil && doc.CozyMetadata.UploadedAt != nil {
		modtime = *doc.CozyMetadata.UploadedAt
	}
	buf, err := icon(fs, doc)
	if err != nil {
		return err
	}
	http.ServeContent(w, req, name, modtime, bytes.NewReader(buf.Bytes()))
	return nil
}

func icon(fs VFS, doc *FileDoc) (*bytes.Buffer, error) {
	cache := previewfs.SystemCache()
	if buf, err := cache.GetIcon(doc.MD5Sum); err == nil {
		return buf, nil
	}

	buf, err := generateIcon(fs, doc)
	if err != nil {
		return nil, err
	}
	_ = cache.SetIcon(doc.MD5Sum, buf)
	return buf, nil
}

func generateIcon(fs VFS, doc *FileDoc) (*bytes.Buffer, error) {
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
		"-limit", "Memory", "1GB",
		"-limit", "Map", "1GB",
		"-[0]",           // Takes the input from stdin
		"-quality", "99", // At small resolution, we want a very good quality
		"-interlace", "none", // Don't use progressive JPEGs, they are heavier
		"-thumbnail", "96x96", // Makes a thumbnail that fits inside the given format
		"-background", "white", // Use white for the background
		"-alpha", "remove", // JPEGs don't have an alpha channel
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
		logger.WithNamespace("pdf_icon").
			WithField("stderr", msg).
			WithField("file_id", doc.ID()).
			Errorf("imagemagick failed: %s", err)
		return nil, err
	}
	return &stdout, nil
}

// ServePDFPreview will send the preview image for a PDF.
func ServePDFPreview(w http.ResponseWriter, req *http.Request, fs VFS, doc *FileDoc) error {
	name := fmt.Sprintf("%s-preview.jpg", doc.ID())
	modtime := doc.UpdatedAt
	if doc.CozyMetadata != nil && doc.CozyMetadata.UploadedAt != nil {
		modtime = *doc.CozyMetadata.UploadedAt
	}
	buf, err := preview(fs, doc)
	if err != nil {
		return err
	}
	http.ServeContent(w, req, name, modtime, bytes.NewReader(buf.Bytes()))
	return nil
}

func preview(fs VFS, doc *FileDoc) (*bytes.Buffer, error) {
	cache := previewfs.SystemCache()
	if buf, err := cache.GetPreview(doc.MD5Sum); err == nil {
		return buf, nil
	}

	buf, err := generatePreview(fs, doc)
	if err != nil {
		return nil, err
	}
	_ = cache.SetPreview(doc.MD5Sum, buf)
	return buf, nil
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
		"-density", "300", // We want a high resolution for PDFs
		"-[0]",           // Takes the input from stdin
		"-quality", "82", // A good compromise between file size and quality
		"-interlace", "none", // Don't use progressive JPEGs, they are heavier
		"-thumbnail", "1080x1920>", // Makes a thumbnail that fits inside the given format
		"-background", "white", // Use white for the background
		"-alpha", "remove", // JPEGs don't have an alpha channel
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
