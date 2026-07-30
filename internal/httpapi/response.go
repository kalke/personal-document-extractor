package httpapi

import (
	"github.com/kalke/personal-document-extractor/internal/extract"
)

type extractResponse struct {
	DocType string `json:"doc_type"`
	Data    any    `json:"data"`
}

func toExtractResponse(result extract.Result) extractResponse {
	return extractResponse{
		DocType: result.DocType,
		Data:    result.Data,
	}
}
