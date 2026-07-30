package preprocess_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kalke/personal-document-extractor/internal/preprocess"
)

func TestValidateUploadAcceptsPDFMagic(t *testing.T) {
	data := []byte("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	kind, mime, err := preprocess.ValidateUpload("doc.pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	if kind != preprocess.KindPDF || mime != "application/pdf" {
		t.Fatalf("got kind=%s mime=%s", kind, mime)
	}
}

func TestValidateUploadRejectsZIP(t *testing.T) {
	data := []byte("PK\x03\x04trash")
	_, _, err := preprocess.ValidateUpload("doc.pdf", data)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, preprocess.ErrUnsupportedMedia) && !strings.Contains(err.Error(), "unsupported media type") {
		t.Fatalf("expected unsupported media, got %v", err)
	}
}

func TestValidateUploadRejectsExtensionMismatch(t *testing.T) {
	data := []byte("\xff\xd8\xff\xe0\x00\x10JFIF")
	_, _, err := preprocess.ValidateUpload("photo.pdf", data)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !errors.Is(err, preprocess.ErrMIMEMismatch) && !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatch, got %v", err)
	}
}

func TestValidateUploadRejectsEmpty(t *testing.T) {
	_, _, err := preprocess.ValidateUpload("a.pdf", nil)
	if !errors.Is(err, preprocess.ErrEmptyUpload) {
		t.Fatalf("got %v", err)
	}
}
