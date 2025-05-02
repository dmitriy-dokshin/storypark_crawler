package parser

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"
)

func LoadMedia(
	ctx context.Context,
	logger *slog.Logger,
	client *http.Client,
	media *Media,
	storyPath string,
	idx int,
	totalCount int,
) error {
	extension, err := extensionByContentType(media.ContentType)
	if err != nil {
		return xerrors.Errorf("failed to get extension: %w", err)
	}

	url := media.OriginalUrl
	if media.Type == "video" {
		url = media.ResizedUrl
	}

	resp, err := client.Get(url)
	if err != nil {
		return xerrors.Errorf("failed to get original url: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return xerrors.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			logger.ErrorContext(ctx, "failed to close response body", "error", err)
		}
	}(resp.Body)

	padding := int(math.Max(math.Log10(float64(totalCount)), 2))
	path := filepath.Join(storyPath, fmt.Sprintf("%0*d - %s%s", padding, idx, media.ID, extension))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return xerrors.Errorf("failed to open file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logger.ErrorContext(ctx, "failed to close file", "error", err)
		}
	}(file)

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return xerrors.Errorf("failed to copy media to file: %w", err)
	}
	return nil
}

func extensionByContentType(contentType string) (string, error) {
	switch contentType {
	case "image/jpeg":
		return ".jpg", nil
	case "video/mp4":
		return ".mp4", nil
	default:
		return "", xerrors.Errorf("unsupported content type: %s", contentType)
	}
}
