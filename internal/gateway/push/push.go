package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Penlk/avito-labs/internal/http/request"
)

const (
	pushServiceEndPoint = "/api/v1/push"
	contentType         = "application/json"
)

type gateway struct {
	client   *http.Client
	endpoint string
}

func NewGateway(baseURL string, timeout time.Duration) *gateway {
	return &gateway{
		client:   &http.Client{Timeout: timeout},
		endpoint: strings.TrimRight(baseURL, "/") + pushServiceEndPoint,
	}
}

func (g *gateway) Push(ctx context.Context, pushRequest Request) (Response, error) {
	body, err := json.Marshal(pushRequest)
	if err != nil {
		return Response{}, fmt.Errorf("encode push request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create push request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	response, err := g.client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("send push request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusAccepted:
		var result Response
		if err := request.DecodeJSON(response.Body, &result); err != nil {
			return Response{}, fmt.Errorf("decode push response: %w", err)
		}
		return result, nil
	case http.StatusBadRequest:
		return Response{}, fmt.Errorf("push service returned HTTP %d: %w", response.StatusCode, ErrInvalidRequest)
	case http.StatusTooManyRequests:
		var retryAfter time.Time
		delay, err := time.ParseDuration(response.Header.Get("Retry-After") + "s")
		if err == nil && delay >= 0 {
			retryAfter = time.Now().Add(delay)
		}
		return Response{}, TooManyRequests{RetryAfter: retryAfter}
	default:
		if response.StatusCode >= http.StatusInternalServerError && response.StatusCode < 600 {
			return Response{}, fmt.Errorf("push service returned HTTP %d: %w", response.StatusCode, ErrUnavailable)
		}
		return Response{}, fmt.Errorf("unexpected push service status: HTTP %d", response.StatusCode)
	}
}
