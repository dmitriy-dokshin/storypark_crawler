package parser

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/xerrors"

	"github.com/dmitriy-dokshin/storypark_crawler/constdef"
	"github.com/dmitriy-dokshin/storypark_crawler/httputil"
)

type Story struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Children []*Child `json:"children"`
	Content  *Content `json:"content"`
	Media    []*Media `json:"media"`
}

type Child struct {
	DisplayName string `json:"display_name"`
}

type StoryInfo struct {
	Text  string
	Media []*Media
}

func (s *Story) GetInfo() (*StoryInfo, error) {
	if s.Content == nil {
		return nil, xerrors.Errorf("story content is empty")
	}

	contentInfo := new(ContentInfo)
	err := s.Content.GetInfo(contentInfo)
	if err != nil {
		return nil, xerrors.Errorf("failed to get content info: %w", err)
	}

	mediaMap := make(map[string]*Media)
	for _, m := range s.Media {
		if _, ok := mediaMap[m.ID]; ok {
			return nil, xerrors.Errorf("media %s is duplicated", m.ID)
		}
		mediaMap[m.ID] = m
	}

	if len(contentInfo.Media) != len(mediaMap) {
		return nil, xerrors.Errorf("media count mismatch")
	}

	storyInfo := new(StoryInfo)
	storyInfo.Text = contentInfo.Text.String()
	for _, m := range contentInfo.Media {
		media, ok := mediaMap[m.MediaItemKey]
		if !ok {
			return nil, xerrors.Errorf("media %s not found", m.MediaItemKey)
		}
		storyInfo.Media = append(storyInfo.Media, media)
	}
	return storyInfo, nil
}

type ContentType string

const (
	ContentTypeVerticalLayout   ContentType = "vertical_layout"
	ContentTypeHorizontalLayout ContentType = "horizontal_layout"
	ContentTypeTitle            ContentType = "title"
	ContentTypeText             ContentType = "text"
	ContentTypeMedia            ContentType = "media"
)

type Content struct {
	Type             ContentType              `json:"type"`
	VerticalLayout   *ContentVerticalLayout   `json:"vertical_layout"`
	HorizontalLayout *ContentHorizontalLayout `json:"horizontal_layout"`
	Title            *ContentTitle            `json:"title"`
	Text             *ContentText             `json:"text"`
	Media            *ContentMedia            `json:"media"`
}

type ContentVerticalLayout struct {
	Blocks []*Content `json:"blocks"`
}

type ContentHorizontalLayout struct {
	Blocks []*Content `json:"blocks"`
}

type ContentTitle struct {
	Text string `json:"text"`
}

type ContentText struct {
	HTML string `json:"html"`
}

type ContentMedia struct {
	MediaItemKey string `json:"media_item_key"`
}

func (c *Content) GetInfo(info *ContentInfo) error {
	switch c.Type {
	case ContentTypeVerticalLayout:
		if c.VerticalLayout == nil {
			return xerrors.New("content has no vertical layout")
		}
		for i, b := range c.VerticalLayout.Blocks {
			err := b.GetInfo(info)
			if err != nil {
				return xerrors.Errorf("failed to get info for vertical block %d: %w", i, err)
			}
		}
	case ContentTypeHorizontalLayout:
		if c.HorizontalLayout == nil {
			return xerrors.New("content has no vertical layout")
		}
		for i, b := range c.HorizontalLayout.Blocks {
			err := b.GetInfo(info)
			if err != nil {
				return xerrors.Errorf("failed to get info for vertical block %d: %w", i, err)
			}
		}
	case ContentTypeTitle:
		if c.Title == nil {
			return xerrors.New("content has no title")
		}
		if c.Title.Text == "" {
			return xerrors.New("title has no text")
		}
		info.Text.WriteString("<h1>")
		info.Text.WriteString(template.HTMLEscapeString(c.Title.Text))
		info.Text.WriteString("</h1>")
		info.Text.WriteString("\n")
	case ContentTypeText:
		if c.Text == nil {
			return xerrors.New("content has no text")
		}
		if c.Text.HTML == "" {
			return xerrors.New("text has no html")
		}
		info.Text.WriteString(c.Text.HTML)
		info.Text.WriteString("\n")
	case ContentTypeMedia:
		if c.Media == nil {
			return xerrors.New("content has no media")
		}
		info.Media = append(info.Media, c.Media)
	default:
		return xerrors.Errorf("unknown content type: %s", c.Type)
	}
	return nil
}

type ContentInfo struct {
	Text  strings.Builder
	Media []*ContentMedia
}

type Media struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	ContentType string `json:"content_type"`
	OriginalUrl string `json:"original_url"`
	ResizedUrl  string `json:"resized_url"`
}

type StoryResponse struct {
	Story *Story `json:"story"`
}

func LoadStory(
	ctx context.Context,
	logger *slog.Logger,
	client *http.Client,
	id string,
) (*Story, error) {
	exp := regexp.MustCompile("stories/[0-9]+")
	reqStr := exp.ReplaceAllString(constdef.RequestStory, fmt.Sprintf("stories/%v", id))
	req, err := httputil.ParseRequest(ctx, reqStr, nil)
	if err != nil {
		return nil, xerrors.Errorf("unable to parse request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("unable to do request: %w", err)
	}

	reader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, xerrors.Errorf("unable to create gzip reader: %w", err)
	}
	defer func(reader *gzip.Reader) {
		err := reader.Close()
		if err != nil {
			logger.ErrorContext(ctx, "unable to close gzip reader", "error", err)
		}
	}(reader)

	storyJson, err := io.ReadAll(reader)
	if err != nil {
		return nil, xerrors.Errorf("unable to read story json: %w", err)
	}
	logger.DebugContext(ctx, "story json read", "json", string(storyJson))

	var storyResponse *StoryResponse
	err = json.Unmarshal(storyJson, &storyResponse)
	if err != nil {
		return nil, xerrors.Errorf("unable to unmarshal story json: %w", err)
	}
	return storyResponse.Story, nil
}
