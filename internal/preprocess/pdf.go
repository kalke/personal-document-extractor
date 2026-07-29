package preprocess

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// TextFromPDF extracts plain text from PDF bytes.
func TextFromPDF(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}

	total := reader.NumPage()
	if total == 0 {
		return "", fmt.Errorf("pdf has no pages")
	}

	var b strings.Builder
	for i := 1; i <= total; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("page %d text: %w", i, err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("no extractable text (scanned PDFs need OCR; not in MVP)")
	}
	return out, nil
}
