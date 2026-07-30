package preprocess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultPDFDPI   = 160
	defaultMaxPages = 3
)

func PDFPagesToJPEG(ctx context.Context, data []byte, maxPages int) ([][]byte, error) {
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, fmt.Errorf("pdftoppm not found (install poppler-utils): %w", err)
	}

	dir, err := os.MkdirTemp("", "pdf-render-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	in := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, err
	}

	prefix := filepath.Join(dir, "page")
	cmd := exec.CommandContext(
		ctx,
		"pdftoppm",
		"-jpeg",
		"-jpegopt", "quality=75",
		"-r", fmt.Sprintf("%d", defaultPDFDPI),
		"-f", "1",
		"-l", fmt.Sprintf("%d", maxPages),
		in,
		prefix,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pdftoppm: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	matches, err := filepath.Glob(prefix + "-*.jpg")
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no pages")
	}

	pages := make([][]byte, 0, len(matches))
	for _, path := range matches {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pages = append(pages, b)
	}
	return pages, nil
}
