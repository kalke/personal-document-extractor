package extract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kalke/personal-document-extractor/internal/llm/groq"
)

var (
	ErrUnknownDocType = errors.New("unknown doc_type")
	ErrInvalidJSON    = errors.New("llm returned invalid json")
	ErrLLM            = errors.New("llm request failed")
)

// DocType describes a fixed document extraction schema.
type DocType interface {
	Name() string
	SystemPrompt() string
	SchemaHint() string
	EmptyResult() any
}

type Result struct {
	DocType string `json:"doc_type"`
	Data    any    `json:"data"`
	Meta    Meta   `json:"meta"`
}

type Meta struct {
	Model string `json:"model"`
	Chars int    `json:"chars"`
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

func (s *Service) Extract(ctx context.Context, docType, documentText string) (Result, error) {
	dt, ok := s.registry[docType]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownDocType, docType)
	}

	userPrompt := buildUserPrompt(dt, documentText)
	content, err := s.llm.Chat(ctx, []groq.Message{
		{Role: "system", Content: dt.SystemPrompt()},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrLLM, err)
	}

	parsed, err := decodeInto(dt, content)
	if err != nil {
		repaired, repairErr := s.repairJSON(ctx, dt, content)
		if repairErr != nil {
			return Result{}, fmt.Errorf("%w: %v (repair failed: %v)", ErrInvalidJSON, err, repairErr)
		}
		parsed = repaired
	}

	return Result{
		DocType: dt.Name(),
		Data:    parsed,
		Meta: Meta{
			Model: s.llm.Model(),
			Chars: len(documentText),
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

func buildUserPrompt(dt DocType, documentText string) string {
	return fmt.Sprintf(`Extract structured data from the Brazilian document text below.
Return ONLY a single JSON object matching this schema (no markdown fences, no commentary):

%s

Document text:
---
%s
---`, dt.SchemaHint(), documentText)
}

func decodeInto(dt DocType, content string) (any, error) {
	cleaned := stripFences(content)
	target := dt.EmptyResult()
	if err := json.Unmarshal([]byte(cleaned), target); err != nil {
		return nil, err
	}
	return target, nil
}

func stripFences(s string) string {
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
