// Package client is a minimal, hand-written Contentful Content Management API
// (CMA) client.
//
// It implements only what the Terraform provider needs: bearer-token
// authentication and the Webhook Definition endpoints. It deliberately avoids
// depending on the official Contentful Go SDK.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Config holds the settings required to talk to a Contentful space via the
// Content Management API.
type Config struct {
	// ManagementToken is a Contentful Content Management API access token.
	ManagementToken string
	SpaceID         string
	// APIURL is the Content Management API base URL, e.g.
	// https://api.contentful.com
	APIURL string
}

// Client is a Contentful Content Management API client scoped to a single
// space.
type Client struct {
	httpClient *http.Client
	apiURL     string
	spaceID    string
}

// bearerTokenTransport injects the Authorization and Content-Type headers
// required by the Content Management API on every request.
type bearerTokenTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/vnd.contentful.management.v1+json")
	}
	return t.base.RoundTrip(req)
}

// New builds a Client that authenticates every request with a static bearer
// token.
func New(cfg Config) (*Client, error) {
	if cfg.ManagementToken == "" {
		return nil, errors.New("management_token is required")
	}
	if cfg.SpaceID == "" {
		return nil, errors.New("space_id is required")
	}
	if cfg.APIURL == "" {
		return nil, errors.New("api_url is required")
	}

	httpClient := &http.Client{
		Transport: &bearerTokenTransport{token: cfg.ManagementToken, base: http.DefaultTransport},
		Timeout:   30 * time.Second,
	}

	return &Client{
		httpClient: httpClient,
		apiURL:     strings.TrimRight(cfg.APIURL, "/"),
		spaceID:    cfg.SpaceID,
	}, nil
}

// APIError represents a non-2xx response from the Content Management API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Contentful API error (status %d): %s", e.StatusCode, e.Body)
}

// errorSys mirrors the "sys" block of a Contentful error response, e.g.
// {"sys":{"type":"Error","id":"NotFound"},"message":"..."}
type errorSys struct {
	Sys struct {
		ID string `json:"id"`
	} `json:"sys"`
}

// sysID returns the Contentful error identifier (e.g. "NotFound",
// "VersionMismatch", "RateLimitExceeded"), or "" if the body doesn't parse.
func (e *APIError) sysID() string {
	var parsed errorSys
	if err := json.Unmarshal([]byte(e.Body), &parsed); err != nil {
		return ""
	}
	return parsed.Sys.ID
}

// IsNotFound reports whether err is an APIError for a missing resource.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound || apiErr.sysID() == "NotFound"
	}
	return false
}

// HasErrorCode reports whether err is an APIError with the given Contentful
// sys.id error identifier (e.g. "VersionMismatch", "RateLimitExceeded").
func HasErrorCode(err error, id string) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.sysID() == id
}

// doJSON performs an HTTP request against the space-scoped API. path must
// start with a leading slash and is appended after /spaces/{spaceID}.
// headers, if non-nil, are set on the request in addition to the defaults
// applied by the client's transport.
func (c *Client) doJSON(ctx context.Context, method, path string, headers map[string]string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	endpoint := c.apiURL + "/spaces/" + c.spaceID + path

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("unmarshaling response body: %w", err)
		}
	}
	return nil
}

// versionHeader builds the X-Contentful-Version header Contentful requires
// for optimistic-concurrency updates and deletes.
func versionHeader(version int) map[string]string {
	return map[string]string{"X-Contentful-Version": strconv.Itoa(version)}
}
