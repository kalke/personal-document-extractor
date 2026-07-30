package preprocess

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
)

const MaxUploadBytes = 32 << 20

var (
	ErrUnsupportedMedia = errors.New("unsupported media type")
	ErrEmptyUpload      = errors.New("uploaded file is empty")
	ErrUploadTooLarge   = errors.New("uploaded file exceeds size limit")
	ErrMIMEMismatch     = errors.New("filename extension does not match file contents")
)

var allowedMIME = map[string]Kind{
	"application/pdf": KindPDF,
	"image/jpeg":      KindImage,
	"image/png":       KindImage,
	"image/webp":      KindImage,
}

var extensionKind = map[string]Kind{
	".pdf":  KindPDF,
	".jpg":  KindImage,
	".jpeg": KindImage,
	".png":  KindImage,
	".webp": KindImage,
}

func ValidateUpload(filename string, data []byte) (Kind, string, error) {
	if len(data) == 0 {
		return KindOther, "", ErrEmptyUpload
	}
	if len(data) > MaxUploadBytes {
		return KindOther, "", fmt.Errorf("%w (%d bytes)", ErrUploadTooLarge, len(data))
	}

	sniffed := canonicalizeMIME(http.DetectContentType(data))
	if sniffed == "application/octet-stream" {
		sniffed = sniffImageMIME(data)
	}
	kind, ok := allowedMIME[sniffed]
	if !ok {
		slog.Warn("upload rejected",
			"filename", filename,
			"sniffed_mime", sniffed,
			"bytes", len(data),
		)
		return KindOther, sniffed, fmt.Errorf("%w: %s", ErrUnsupportedMedia, sniffed)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != "" {
		if expected, known := extensionKind[ext]; known && expected != kind {
			slog.Warn("upload rejected: extension mismatch",
				"filename", filename,
				"extension", ext,
				"sniffed_mime", sniffed,
			)
			return KindOther, sniffed, fmt.Errorf("%w: %s vs %s", ErrMIMEMismatch, ext, sniffed)
		}
	}

	return kind, sniffed, nil
}

func sniffImageMIME(data []byte) string {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return "application/octet-stream"
}

func canonicalizeMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	switch mime {
	case "image/jpg":
		return "image/jpeg"
	default:
		return mime
	}
}
