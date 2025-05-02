package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lmittmann/tint"
	"golang.org/x/xerrors"

	"github.com/dmitriy-dokshin/storypark_crawler/parser"
)

func main() {
	ctx := context.Background()
	logger := slog.New(tint.NewHandler(os.Stderr, nil))
	err := run(ctx, logger)
	if err != nil {
		logger.InfoContext(ctx, "run failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	client := new(http.Client)

	basePath := `C:\Users\dmitr\Downloads\Storypark`
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return xerrors.New("stories base path does not exist")
	}

	from := time.Now().Add(-10 * 365 * 24 * time.Hour)
	//from, err := time.Parse(time.RFC3339, "22025-02-26T11:21:10+08:00")
	//if err != nil {
	//	return xerrors.Errorf("failed to parse from time: %w", err)
	//}
	until := time.Now()
	//until, err := time.Parse(time.RFC3339, "2025-04-21T18:10:14+08:00")
	//if err != nil {
	//	return xerrors.Errorf("failed to parse until time: %w", err)
	//}
	count := -1
	for {
		logger := logger.With("until", until.Format(time.RFC3339))
		logger.InfoContext(ctx, "loading activities")
		activities, err := parser.LoadActivities(ctx, logger, client, until)
		if err != nil {
			return xerrors.Errorf("failed to load activities: %w", err)
		}
		if len(activities) == 0 {
			logger.InfoContext(ctx, "no more activities found")
			return nil
		}

		for _, activity := range activities {
			if count == 0 {
				logger.InfoContext(ctx, "count reached 0")
				return nil
			}
			if count > 0 {
				count--
			}

			logger := logger.With("updated_at", activity.UpdatedAt.Format(time.RFC3339), "post_id", activity.PostID)
			if activity.UpdatedAt.Compare(from) <= 0 {
				logger.InfoContext(ctx, "specified from time reached", "from", from)
				return nil
			}

			until = activity.UpdatedAt

			logger.InfoContext(ctx, "loading story")
			story, err := parser.LoadStory(ctx, logger, client, activity.PostID)
			if err != nil {
				return xerrors.Errorf("failed to load story: %w", err)
			}

			storyInfo, err := story.GetInfo()
			if err != nil {
				return xerrors.Errorf("failed to load story info: %w", err)
			}

			title := story.Title
			if title == "" {
				title = "(no title)"
			} else {
				title = sanitizeTitle(title)
			}

			updatedAt := strings.ReplaceAll(activity.UpdatedAt.Format(time.RFC3339), ":", "-")
			storyName := fmt.Sprintf("%s - %s - %s", updatedAt, story.ID, title)
			var storyPath string
			if len(story.Children) == 1 {
				storyPath = filepath.Join(basePath, story.Children[0].DisplayName, storyName)
			} else {
				storyPath = filepath.Join(basePath, "All", storyName)
			}

			err = os.MkdirAll(storyPath, os.ModePerm)
			if err != nil && !os.IsExist(err) {
				return xerrors.Errorf("failed to mkdir %s: %w", storyPath, err)
			}
			err = setFileTime(storyPath, activity.UpdatedAt)
			if err != nil {
				return xerrors.Errorf("failed to set file time of %s: %w", storyPath, err)
			}

			if storyInfo.Text != "" {
				readmePath := filepath.Join(storyPath, "_README.html")
				err = os.WriteFile(readmePath, []byte(storyInfo.Text), os.ModePerm)
				if err != nil {
					return xerrors.Errorf("failed to write readme: %w", err)
				}
				err = setFileTime(readmePath, activity.UpdatedAt)
				if err != nil {
					return xerrors.Errorf("failed to set file time of %s: %w", readmePath, err)
				}
			}

			if len(storyInfo.Media) == 0 {
				logger.WarnContext(ctx, "no media in the story")
				continue
			}

			for idx, media := range storyInfo.Media {
				logger := logger.With("media_id", media.ID)
				logger.InfoContext(ctx, "loading media")
				err := parser.LoadMedia(ctx, logger, client, media, storyPath, idx, len(storyInfo.Media))
				if errors.Is(err, parser.ErrMediaForbidden) {
					logger.WarnContext(ctx, "media forbidden")
					continue
				}
				if err != nil {
					return xerrors.Errorf("failed to load media %q: %w", media.OriginalUrl, err)
				}
			}
		}
	}
}

func sanitizeTitle(title string) string {
	title = replace(title, "\\\"", "\"")
	title = replace(title, "\"", "'")
	title = replace(title, "\\/:*?<>|", "-")
	title = strings.Trim(title, " .")
	return title
}

func replace(s, old, new string) string {
	var args []string
	for i := 0; i < len(old); i++ {
		args = append(args, old[i:i+1], new)
	}
	return strings.NewReplacer(args...).Replace(s)
}

func setFileTime(path string, updatedAt time.Time) error {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return xerrors.Errorf("failed to convert path to UTF16: %w", err)
	}
	h, err := syscall.CreateFile(pathPtr, syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return xerrors.Errorf("failed to open %s: %w", path, err)
	}
	defer syscall.Close(h)
	updatedAtFiletime := syscall.NsecToFiletime(updatedAt.UnixNano())
	return syscall.SetFileTime(h, &updatedAtFiletime, nil, nil)
}
