package httputil

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"golang.org/x/xerrors"
)

type THeader struct {
	Key   string
	Value string
}

func newLine() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

func ParseRequest(ctx context.Context, reqStr string, body io.Reader) (*http.Request, error) {
	var method, host, path string
	var headers []*THeader
	lines := strings.Split(reqStr, newLine())
	parts := strings.Split(lines[0], " ")
	if len(parts) != 3 {
		return nil, xerrors.Errorf("invalid first request line: %s", lines[0])
	}
	major, minor, ok := http.ParseHTTPVersion(parts[2])
	if !ok {
		return nil, xerrors.Errorf("invalid http version %q", parts[2])
	}
	method, path = parts[0], parts[1]
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		parts = strings.Split(line, ": ")
		key := parts[0]
		value := parts[1]
		switch key {
		case "Host":
			host = value
		default:
			headers = append(headers, &THeader{
				Key:   key,
				Value: value,
			})
		}
	}
	u, err := url.Parse(path)
	if err != nil {
		return nil, xerrors.Errorf("unable to parse path %v: %w", path, err)
	}
	u.Scheme = "https"
	u.Host = host
	req, err := http.NewRequestWithContext(ctx, method, "", body)
	if err != nil {
		return nil, xerrors.Errorf("unable to initialize request: %w", err)
	}
	req.ProtoMajor = major
	req.ProtoMinor = minor
	req.URL = u
	for _, header := range headers {
		req.Header.Add(header.Key, header.Value)
	}
	return req, nil
}
