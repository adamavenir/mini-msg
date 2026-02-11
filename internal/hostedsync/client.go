package hostedsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrBaseMismatch is returned when the server rejects a push due to base mismatch.
var ErrBaseMismatch = errors.New("base mismatch")

// APIError represents a non-2xx response from the hosted sync API.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("hosted sync error: %s (%d): %s", e.Code, e.Status, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("hosted sync error: %s (%d)", e.Code, e.Status)
	}
	if e.Message != "" {
		return fmt.Sprintf("hosted sync error (%d): %s", e.Status, e.Message)
	}
	return fmt.Sprintf("hosted sync error (%d)", e.Status)
}

type apiErrorPayload struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Client talks to the hosted sync API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient constructs a hosted sync client.
func NewClient(baseURL, token string) (*Client, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: normalized,
		token:   token,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

// NormalizeBaseURL normalizes a hosted base URL and ensures it has a scheme.
func NormalizeBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("hosted url cannot be empty")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid hosted url: %w", err)
	}
	if parsed.Scheme == "" {
		return "", fmt.Errorf("hosted url must include scheme (https://)")
	}
	value = strings.TrimRight(value, "/")
	return value, nil
}

// RegisterMachine registers a machine with the hosted backend.
func (c *Client) RegisterMachine(ctx context.Context, req RegisterMachineRequest) (RegisterMachineResponse, error) {
	var resp RegisterMachineResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/sync/register-machine", nil, req, &resp); err != nil {
		return RegisterMachineResponse{}, err
	}
	return resp, nil
}

// Manifest fetches stream metadata for a channel.
func (c *Client) Manifest(ctx context.Context, channelID string) (ManifestResponse, error) {
	var resp ManifestResponse
	query := url.Values{}
	query.Set("channel_id", channelID)
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sync/manifest", query, nil, &resp); err != nil {
		return ManifestResponse{}, err
	}
	return resp, nil
}

// Pull fetches new lines from the hosted backend.
func (c *Client) Pull(ctx context.Context, req PullRequest) (PullResponse, error) {
	var resp PullResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/sync/pull", nil, req, &resp); err != nil {
		return PullResponse{}, err
	}
	return resp, nil
}

// Push uploads new lines to the hosted backend.
func (c *Client) Push(ctx context.Context, req PushRequest) (PushResponse, error) {
	var resp PushResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/sync/push", nil, req, &resp); err != nil {
		if apiErr, ok := err.(*APIError); ok {
			if apiErr.Status == http.StatusConflict && (apiErr.Code == "base_mismatch" || apiErr.Code == "") {
				return PushResponse{}, ErrBaseMismatch
			}
		}
		return PushResponse{}, err
	}
	return resp, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, reqBody any, respBody any) error {
	endpoint, err := c.buildURL(path, query)
	if err != nil {
		return err
	}

	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode}
		var payload apiErrorPayload
		if err := json.Unmarshal(respData, &payload); err == nil {
			apiErr.Code = payload.Error
			apiErr.Message = payload.Message
		} else {
			apiErr.Message = strings.TrimSpace(string(respData))
		}
		return apiErr
	}

	if respBody == nil {
		return nil
	}
	if len(respData) == 0 {
		return nil
	}
	return json.Unmarshal(respData, respBody)
}

func (c *Client) buildURL(path string, query url.Values) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	endpoint := base.ResolveReference(ref)
	if query != nil && len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String(), nil
}

// RegisterMachineRequest describes the register machine API call.
type RegisterMachineRequest struct {
	ChannelID  string            `json:"channel_id"`
	MachineID  string            `json:"machine_id"`
	DeviceInfo map[string]string `json:"device_info,omitempty"`
}

// RegisterMachineResponse is returned after registration.
type RegisterMachineResponse struct {
	OK        bool   `json:"ok"`
	MachineID string `json:"machine_id"`
	Token     string `json:"token"`
}

// ManifestEntry describes a stream in the hosted manifest.
type ManifestEntry struct {
	MachineID string `json:"machine_id"`
	File      string `json:"file"`
	LineCount int64  `json:"line_count"`
	SHA256    string `json:"sha256"`
	LastSeq   int64  `json:"last_seq"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

// ManifestResponse contains all streams for a channel.
type ManifestResponse struct {
	Streams []ManifestEntry `json:"streams"`
}

// PullCursor is sent to request new lines.
type PullCursor struct {
	MachineID  string `json:"machine_id"`
	File       string `json:"file"`
	LineOffset int64  `json:"line_offset"`
}

// PullRequest requests updates for streams.
type PullRequest struct {
	ChannelID string       `json:"channel_id"`
	Cursors   []PullCursor `json:"cursors"`
}

// PullUpdate contains new lines for a stream.
type PullUpdate struct {
	MachineID string   `json:"machine_id"`
	File      string   `json:"file"`
	Lines     []string `json:"lines"`
	NewOffset int64    `json:"new_offset"`
}

// PullResponse contains updates for streams.
type PullResponse struct {
	Updates []PullUpdate `json:"updates"`
}

// PushBase describes the expected base state for a push.
type PushBase struct {
	LineCount int64  `json:"line_count"`
	SHA256    string `json:"sha256,omitempty"`
	LastSeq   int64  `json:"last_seq,omitempty"`
}

// PushRequest uploads new lines to a stream.
type PushRequest struct {
	ChannelID      string   `json:"channel_id"`
	MachineID      string   `json:"machine_id"`
	File           string   `json:"file"`
	Base           PushBase `json:"base"`
	Lines          []string `json:"lines"`
	IdempotencyKey string   `json:"idempotency_key"`
}

// PushResponse contains the new base after a push.
type PushResponse struct {
	OK           bool   `json:"ok"`
	NewLineCount int64  `json:"new_line_count"`
	NewSHA256    string `json:"new_sha256"`
	NewLastSeq   int64  `json:"new_last_seq"`
}
