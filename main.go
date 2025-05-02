package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
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

	from := time.Now().Add(-10 * 365 * 24 * time.Hour)
	//from, err := time.Parse(time.RFC3339, "2024-12-20T11:51:07+08:00")
	//if err != nil {
	//	return xerrors.Errorf("failed to parse from time: %w", err)
	//}
	until := time.Now()
	//until, err := time.Parse(time.RFC3339, "2025-04-21T18:10:14+08:00")
	//if err != nil {
	//	return xerrors.Errorf("failed to parse until time: %w", err)
	//}
	count := 50
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
			count--

			until = activity.UpdatedAt
			logger := logger.With("post_id", activity.PostID)

			if until.Compare(from) <= 0 {
				logger.InfoContext(ctx, "specified from time reached", "from", from)
				return nil
			}

			logger.InfoContext(ctx, "loading story")
			story, err := parser.LoadStory(ctx, logger, client, activity.PostID)
			if err != nil {
				return xerrors.Errorf("failed to load story: %w", err)
			}

			title := story.Title
			if title == "" {
				title = "(no title)"
			}

			storyInfo, err := story.GetInfo()
			if err != nil {
				return xerrors.Errorf("failed to load story info: %w", err)
			}

			basePath := "/Users/bytedance/Downloads/Stories"
			if _, err := os.Stat(basePath); os.IsNotExist(err) {
				return xerrors.New("stories base path does not exist")
			}

			storyName := fmt.Sprintf("%s - %s - %s", activity.UpdatedAt.Format(time.RFC3339), story.ID, title)
			var storyPath string
			if len(story.Children) == 1 {
				storyPath = path.Join(basePath, story.Children[0].DisplayName, storyName)
			} else {
				storyPath = path.Join(basePath, "All", storyName)
			}

			err = os.MkdirAll(storyPath, os.ModePerm)
			if err != nil && !os.IsExist(err) {
				return xerrors.Errorf("failed to mkdir %s: %w", storyPath, err)
			}

			if storyInfo.Text != "" {
				err = os.WriteFile(path.Join(storyPath, "_README.html"), []byte(storyInfo.Text), os.ModePerm)
				if err != nil {
					return xerrors.Errorf("failed to write readme: %w", err)
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
				if err != nil {
					return xerrors.Errorf("failed to load media %q: %w", media.OriginalUrl, err)
				}
			}
		}
	}
}
