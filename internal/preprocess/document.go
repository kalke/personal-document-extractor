package preprocess

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
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

// Options controls PDF preparation. PreferVision is used for scanned ID / address /
// invoice PDFs that often carry a junk text layer long enough to bypass OCR/vision.
type Options struct {
	PreferVision bool
}

const (
	// Prefer text when a PDF already has a usable text layer (typical for digital CVs).
	minPDFTextChars = 200
	// Cap prompt text so prompt+completion stay under free-tier Groq TPM budgets.
	maxPDFTextChars = 12_000
	maxVisionPages  = 2
)

func Prepare(filename string, data []byte) (PreparedDocument, error) {
	return PrepareWithOptions(filename, data, Options{})
}

func PrepareWithOptions(filename string, data []byte, opts Options) (PreparedDocument, error) {
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
		if opts.PreferVision {
			if visionDoc, ok, err := preparePDFVision(doc, data); err != nil {
				return doc, err
			} else if ok {
				return visionDoc, nil
			}
			slog.Info("prefer_vision render unavailable; falling back to text",
				"filename", filename)
		}

		text, textErr := TextFromPDF(data, defaultMaxPages)
		if textErr == nil && usablePDFText(text) {
			trimmed := strings.TrimSpace(text)
			if len(trimmed) > maxPDFTextChars {
				trimmed = trimmed[:maxPDFTextChars]
			}
			doc.Text = trimmed
			doc.Mode = "text"
			slog.Debug("preprocess ready", "kind", "pdf", "mode", doc.Mode, "text_chars", len(doc.Text))
			return doc, nil
		}

		if visionDoc, ok, err := preparePDFVision(doc, data); err != nil {
			return doc, err
		} else if ok {
			return visionDoc, nil
		}

		if textErr != nil {
			return doc, fmt.Errorf("pdf has no usable text and render failed: %w", textErr)
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return doc, fmt.Errorf("pdf has no usable text and render failed")
		}
		if len(trimmed) > maxPDFTextChars {
			trimmed = trimmed[:maxPDFTextChars]
		}
		doc.Text = trimmed
		doc.Mode = "text"
		slog.Warn("pdf render failed; falling back to short/weak text", "text_chars", len(doc.Text))
		return doc, nil

	default:
		return doc, fmt.Errorf("%w: %s", ErrUnsupportedMedia, mime)
	}
}

func preparePDFVision(doc PreparedDocument, data []byte) (PreparedDocument, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pages, renderErr := PDFPagesToJPEG(ctx, data, maxVisionPages)
	if renderErr != nil || len(pages) == 0 {
		return doc, false, nil
	}
	for _, p := range pages {
		compacted, outMIME, err := CompactImage(p)
		if err != nil {
			return doc, false, fmt.Errorf("pdf page preprocess: %w", err)
		}
		doc.Images = append(doc.Images, Image{MIME: outMIME, Data: compacted})
	}
	doc.Mode = "vision"
	slog.Debug("preprocess ready", "kind", "pdf", "pages", len(doc.Images), "mode", doc.Mode)
	return doc, true, nil
}

// usablePDFText rejects short or junk text layers (common on scanned CNH-e PDFs)
// so we fall through to vision instead of feeding garbage to the LLM.
func usablePDFText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < minPDFTextChars {
		return false
	}

	letters := 0
	digits := 0
	digitRun := 0
	hasDigitRun := false
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r):
			letters++
			digitRun = 0
		case unicode.IsDigit(r):
			digits++
			digitRun++
			if digitRun >= 8 {
				hasDigitRun = true
			}
		default:
			digitRun = 0
		}
	}
	if letters < 60 {
		return false
	}

	lower := strings.ToLower(trimmed)
	hasDocSignal := hasDigitRun ||
		strings.Contains(lower, "cpf") ||
		strings.Contains(lower, "nome") ||
		strings.Contains(lower, "cnh") ||
		strings.Contains(lower, "rg") ||
		strings.Contains(lower, "experiencia") ||
		strings.Contains(lower, "experiência") ||
		strings.Contains(lower, "education") ||
		strings.Contains(lower, "educação") ||
		strings.Contains(lower, "curriculo") ||
		strings.Contains(lower, "currículo") ||
		strings.Contains(lower, "resumo") ||
		strings.Contains(lower, "endereco") ||
		strings.Contains(lower, "endereço") ||
		strings.Contains(lower, "cep")

	// PDF object/xref dumps are long but useless — never treat as a text layer.
	if strings.Contains(lower, "xref") ||
		strings.Contains(lower, "endobj") ||
		strings.Contains(lower, "trailer") {
		return false
	}

	spaces := strings.Count(trimmed, " ")
	// Long prose-like text (digital CV) is fine even without keywords.
	if letters >= 400 && spaces >= 40 {
		return true
	}
	return hasDocSignal
}
