package preprocess_test

import (
	"errors"
	"testing"

	"github.com/kalke/personal-document-extractor/internal/preprocess"
)

func TestValidateUploadAcceptsPDFMagic(t *testing.T) {
	data := []byte("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	kind, mime, err := preprocess.ValidateUpload("doc.pdf", "application/octet-stream", data)
	if err != nil {
		t.Fatal(err)
	}
	if kind != preprocess.KindPDF || mime != "application/pdf" {
		t.Fatalf("got kind=%s mime=%s", kind, mime)
	}
}

func TestValidateUploadRejectsZIP(t *testing.T) {
	data := []byte("PK\x03\x04trash")
	_, _, err := preprocess.ValidateUpload("doc.pdf", "application/pdf", data)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, preprocess.ErrUnsupportedMedia) && !contains(err.Error(), "unsupported media type") {
		t.Fatalf("expected unsupported media, got %v", err)
	}
}

func TestValidateUploadRejectsExtensionMismatch(t *testing.T) {
	data := []byte("\xff\xd8\xff\xe0\x00\x10JFIF")
	_, _, err := preprocess.ValidateUpload("photo.pdf", "image/jpeg", data)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !errors.Is(err, preprocess.ErrMIMEMismatch) && !contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatch, got %v", err)
	}
}

func TestValidateUploadRejectsEmpty(t *testing.T) {
	_, _, err := preprocess.ValidateUpload("a.pdf", "application/pdf", nil)
	if !errors.Is(err, preprocess.ErrEmptyUpload) {
		t.Fatalf("got %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
