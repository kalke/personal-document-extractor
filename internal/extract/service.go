package extract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kalke/personal-document-extractor/internal/llm/groq"
	"github.com/kalke/personal-document-extractor/internal/preprocess"
)

var (
	ErrUnknownDocType = errors.New("unknown doc_type")
	ErrInvalidJSON    = errors.New("llm returned invalid json")
	ErrLLM            = errors.New("llm request failed")
)

type DocType interface {
	Name() string
	SystemPrompt() string
	SchemaHint() string
	EmptyResult() any
	Normalize(result any)
}

type Result struct {
	DocType string `json:"doc_type"`
	Data    any    `json:"data"`
	Meta    Meta   `json:"meta"`
}

type Meta struct {
	Model    string `json:"model"`
	Chars    int    `json:"chars"`
	Mode     string `json:"mode"`
	Images   int    `json:"images"`
	Filename string `json:"filename,omitempty"`
}

type Service struct {
	llm      *groq.Client
	registry map[string]DocType
}

func NewService(llm *groq.Client, types ...DocType) *Service {
	reg := make(map[string]DocType, len(types))
	for _, t := range types {
		reg[t.Name()] = t
	}
	return &Service{llm: llm, registry: reg}
}

func (s *Service) KnownTypes() []string {
	out := make([]string, 0, len(s.registry))
	for k := range s.registry {
		out = append(out, k)
	}
	return out
}

func (s *Service) Extract(ctx context.Context, docType string, doc preprocess.PreparedDocument) (Result, error) {
	dt, ok := s.registry[docType]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownDocType, docType)
	}

	start := time.Now()
	slog.Debug("extract start",
		"doc_type", docType,
		"filename", doc.Filename,
		"kind", doc.Kind,
		"mode", doc.Mode,
		"text_chars", len(doc.Text),
		"images", len(doc.Images),
	)

	messages := []groq.Message{
		{Role: "system", Content: dt.SystemPrompt()},
		{Role: "user", Content: buildUserContent(dt, doc)},
	}

	content, err := s.llm.Chat(ctx, messages)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrLLM, err)
	}

	parsed, err := decodeInto(dt, content)
	if err != nil {
		slog.Warn("extract json parse failed; attempting repair",
			"doc_type", docType,
			"err", err,
			"reply_chars", len(content),
		)
		repaired, repairErr := s.repairJSON(ctx, dt, content)
		if repairErr != nil {
			return Result{}, fmt.Errorf("%w: %v (repair failed: %v)", ErrInvalidJSON, err, repairErr)
		}
		parsed = repaired
	}

	dt.Normalize(parsed)

	slog.Info("extract ok",
		"doc_type", docType,
		"mode", doc.Mode,
		"images", len(doc.Images),
		"duration_ms", time.Since(start).Milliseconds(),
		"model", s.llm.Model(),
	)

	return Result{
		DocType: dt.Name(),
		Data:    parsed,
		Meta: Meta{
			Model:    s.llm.Model(),
			Chars:    len(doc.Text),
			Mode:     doc.Mode,
			Images:   len(doc.Images),
			Filename: doc.Filename,
		},
	}, nil
}

func (s *Service) repairJSON(ctx context.Context, dt DocType, broken string) (any, error) {
	content, err := s.llm.Chat(ctx, []groq.Message{
		{Role: "system", Content: "You fix malformed JSON. Return ONLY valid JSON matching the schema. No markdown."},
		{Role: "user", Content: fmt.Sprintf("Schema:\n%s\n\nBroken JSON:\n%s", dt.SchemaHint(), broken)},
	})
	if err != nil {
		return nil, err
	}
	return decodeInto(dt, content)
}

func buildUserContent(dt DocType, doc preprocess.PreparedDocument) any {
	prompt := buildUserPrompt(dt, doc)
	if doc.Mode != "vision" || len(doc.Images) == 0 {
		return prompt
	}

	parts := []groq.ContentPart{{Type: "text", Text: prompt}}
	for _, img := range doc.Images {
		mime := img.MIME
		if mime == "" {
			mime = "image/png"
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(img.Data))
		parts = append(parts, groq.ContentPart{
			Type:     "image_url",
			ImageURL: &groq.ImageURL{URL: dataURL},
		})
	}
	return parts
}

func buildUserPrompt(dt DocType, doc preprocess.PreparedDocument) string {
	var b strings.Builder
	b.WriteString("Extract structured data from the Brazilian document")
	if doc.Mode == "vision" {
		b.WriteString(" image(s)")
	} else {
		b.WriteString(" text")
	}
	b.WriteString(" below.\n")
	b.WriteString("Return ONLY a single JSON object matching this schema:\n\n")
	b.WriteString(dt.SchemaHint())
	b.WriteString("\n\n")
	b.WriteString("Important extraction rules:\n")
	b.WriteString("- CPF: find label \"CPF\" (CNH field 6). Copy ###.###.###-## exactly; 11 digits only. Do not leave empty if readable.\n")
	b.WriteString("- CNH validade: use field 4b VALIDADE only. Field 4a is emissão — do not put 4a into validade.\n")
	b.WriteString("- Birth date: field 3 only; double-check each year digit (9 vs 0).\n")
	b.WriteString("- numero_documento: CNH field 7 Nº REGISTRO (not CPF).\n\n")

	if strings.TrimSpace(doc.Text) != "" && doc.Mode == "text" {
		b.WriteString("Document text:\n---\n")
		b.WriteString(doc.Text)
		b.WriteString("\n---\n")
	}
	if doc.Mode == "vision" {
		b.WriteString("Read labeled fields from the attached image(s). Prefer printed field numbers/labels over nearby noise.\n")
	}
	return b.String()
}

func decodeInto(dt DocType, content string) (any, error) {
	cleaned := stripFences(content)
	if i := strings.Index(cleaned, "{"); i >= 0 {
		if j := strings.LastIndex(cleaned, "}"); j > i {
			cleaned = cleaned[i : j+1]
		}
	}
	target := dt.EmptyResult()
	if err := json.Unmarshal([]byte(cleaned), target); err != nil {
		return nil, err
	}
	return target, nil
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	for {
		start := strings.Index(s, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "</think>")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+end+len("</think>"):]
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```JSON")
		s = strings.TrimPrefix(s, "```")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	return s
}
