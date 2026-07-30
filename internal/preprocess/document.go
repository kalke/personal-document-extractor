package preprocess

import (
	"fmt"
	"log/slog"
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

func Prepare(filename, contentType string, data []byte) (PreparedDocument, error) {
	kind, mime, err := ValidateUpload(filename, contentType, data)
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
			slog.Warn("image compact failed; using original", "err", err)
			doc.Images = []Image{{MIME: mime, Data: data}}
		} else {
			doc.Images = []Image{{MIME: outMIME, Data: compacted}}
			doc.MIME = outMIME
		}
		doc.Mode = "vision"
		slog.Debug("preprocess ready", "kind", "image", "mime", doc.MIME, "bytes", len(doc.Images[0].Data))
		return doc, nil

	case KindPDF:
		text, textErr := TextFromPDF(data)
		if textErr != nil {
			slog.Debug("pdf text extract failed; using vision", "err", textErr)
		} else {
			doc.Text = text
		}

		pages, renderErr := PDFPagesToPNG(data, defaultMaxPages)
		if renderErr != nil {
			if doc.Text == "" {
				return doc, fmt.Errorf("pdf has no usable text and render failed: %w", renderErr)
			}
			slog.Warn("pdf render failed; falling back to text", "err", renderErr)
			doc.Mode = "text"
			return doc, nil
		}

		for _, p := range pages {
			compacted, outMIME, err := CompactImage(p)
			if err != nil {
				doc.Images = append(doc.Images, Image{MIME: "image/jpeg", Data: p})
				continue
			}
			doc.Images = append(doc.Images, Image{MIME: outMIME, Data: compacted})
		}

		doc.Mode = "vision"
		slog.Debug("preprocess ready",
			"kind", "pdf",
			"text_chars", len(doc.Text),
			"pages", len(doc.Images),
			"mode", doc.Mode,
		)
		return doc, nil

	default:
		return doc, fmt.Errorf("%w: %s", ErrUnsupportedMedia, mime)
	}
}
