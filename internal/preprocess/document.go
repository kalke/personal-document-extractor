package preprocess

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type PreparedDocument struct {
	Kind     Kind
	MIME     string
	Text     string
	Images   []Image
	Mode     string
	Filename string
}

type Image struct {
	MIME string
	Data []byte
}

const (
	// Prefer text when a PDF already has a usable text layer (typical for digital CVs).
	minPDFTextChars = 200
	// Cap prompt text so prompt+completion stay under free-tier Groq TPM budgets.
	maxPDFTextChars = 12_000
	maxVisionPages  = 2
)

func Prepare(filename string, data []byte) (PreparedDocument, error) {
	kind, mime, err := ValidateUpload(filename, data)
	if err != nil {
		return PreparedDocument{}, err
	}

	doc := PreparedDocument{
		Kind:     kind,
		MIME:     mime,
		Filename: filename,
	}

	switch kind {
	case KindImage:
		compacted, outMIME, err := CompactImage(data)
		if err != nil {
			return doc, fmt.Errorf("image preprocess: %w", err)
		}
		doc.Images = []Image{{MIME: outMIME, Data: compacted}}
		doc.MIME = outMIME
		doc.Mode = "vision"
		return doc, nil

	case KindPDF:
		text, textErr := TextFromPDF(data, defaultMaxPages)
		if textErr == nil {
			trimmed := strings.TrimSpace(text)
			if len(trimmed) >= minPDFTextChars {
				if len(trimmed) > maxPDFTextChars {
					trimmed = trimmed[:maxPDFTextChars]
				}
				doc.Text = trimmed
				doc.Mode = "text"
				slog.Debug("preprocess ready", "kind", "pdf", "mode", doc.Mode, "text_chars", len(doc.Text))
				return doc, nil
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		pages, renderErr := PDFPagesToJPEG(ctx, data, maxVisionPages)
		if renderErr == nil && len(pages) > 0 {
			for _, p := range pages {
				compacted, outMIME, err := CompactImage(p)
				if err != nil {
					return doc, fmt.Errorf("pdf page preprocess: %w", err)
				}
				doc.Images = append(doc.Images, Image{MIME: outMIME, Data: compacted})
			}
			doc.Mode = "vision"
			slog.Debug("preprocess ready", "kind", "pdf", "pages", len(doc.Images), "mode", doc.Mode)
			return doc, nil
		}

		if textErr != nil {
			if renderErr != nil {
				return doc, fmt.Errorf("pdf has no usable text and render failed: %w", renderErr)
			}
			return doc, fmt.Errorf("pdf text extract failed: %w", textErr)
		}
		if renderErr != nil {
			slog.Warn("pdf render failed; falling back to short text", "err", renderErr)
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return doc, fmt.Errorf("pdf has no usable text and render failed: %w", renderErr)
		}
		if len(trimmed) > maxPDFTextChars {
			trimmed = trimmed[:maxPDFTextChars]
		}
		doc.Text = trimmed
		doc.Mode = "text"
		return doc, nil

	default:
		return doc, fmt.Errorf("%w: %s", ErrUnsupportedMedia, mime)
	}
}
