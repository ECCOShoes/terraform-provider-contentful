package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// WebhookHeader is a single custom header sent with every webhook call. When
// Secret is true, Contentful never returns Value in API responses; callers
// must remember the value they configured themselves.
type WebhookHeader struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

// WebhookTransformation customizes the HTTP method, content type and body of
// the outgoing webhook call.
type WebhookTransformation struct {
	Method               string  `json:"method,omitempty"`
	ContentType          string  `json:"contentType,omitempty"`
	IncludeContentLength *bool   `json:"includeContentLength,omitempty"`
	Body                 *string `json:"body,omitempty"`
}

// WebhookDraft is the payload used to create or replace (PUT) a webhook
// definition.
type WebhookDraft struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Topics []string `json:"topics"`
	Active *bool    `json:"active,omitempty"`

	// HTTPBasicUsername/HTTPBasicPassword configure HTTP basic auth for the
	// webhook call. The password is write-only: Contentful never returns it.
	HTTPBasicUsername *string `json:"httpBasicUsername,omitempty"`
	HTTPBasicPassword *string `json:"httpBasicPassword,omitempty"`

	Headers []WebhookHeader `json:"headers,omitempty"`

	// Filters is a raw passthrough of Contentful's filter expression array
	// (e.g. [{"equals":[{"doc":"sys.environment.sys.id"},"master"]}]).
	Filters json.RawMessage `json:"filters,omitempty"`

	Transformation *WebhookTransformation `json:"transformation,omitempty"`
}

// WebhookDefinition is the Contentful representation of a webhook, as
// returned by the API. It never contains HTTPBasicPassword or the values of
// secret headers.
type WebhookDefinition struct {
	ID      string
	Version int

	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Topics []string `json:"topics"`
	Active *bool    `json:"active,omitempty"`

	HTTPBasicUsername *string `json:"httpBasicUsername,omitempty"`

	Headers []WebhookHeader `json:"headers,omitempty"`

	Filters json.RawMessage `json:"filters,omitempty"`

	Transformation *WebhookTransformation `json:"transformation,omitempty"`
}

// webhookSys mirrors the "sys" metadata block Contentful attaches to a
// webhook definition response.
type webhookSys struct {
	Sys struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	} `json:"sys"`
}

// UnmarshalJSON pulls ID/Version out of the nested "sys" object in addition
// to the regular fields.
func (w *WebhookDefinition) UnmarshalJSON(data []byte) error {
	type alias WebhookDefinition
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*w = WebhookDefinition(a)

	var sys webhookSys
	if err := json.Unmarshal(data, &sys); err != nil {
		return err
	}
	w.ID = sys.Sys.ID
	w.Version = sys.Sys.Version
	return nil
}

// CreateWebhook creates a new webhook definition from the given draft.
func (c *Client) CreateWebhook(ctx context.Context, draft WebhookDraft) (*WebhookDefinition, error) {
	var wh WebhookDefinition
	if err := c.doJSON(ctx, http.MethodPost, "/webhook_definitions", nil, draft, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// GetWebhook fetches a webhook definition by its id.
func (c *Client) GetWebhook(ctx context.Context, id string) (*WebhookDefinition, error) {
	var wh WebhookDefinition
	if err := c.doJSON(ctx, http.MethodGet, "/webhook_definitions/"+id, nil, nil, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// UpdateWebhook replaces a webhook definition at the specified version.
func (c *Client) UpdateWebhook(ctx context.Context, id string, version int, draft WebhookDraft) (*WebhookDefinition, error) {
	var wh WebhookDefinition
	if err := c.doJSON(ctx, http.MethodPut, "/webhook_definitions/"+id, versionHeader(version), draft, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// DeleteWebhook deletes a webhook definition by id at the specified version.
func (c *Client) DeleteWebhook(ctx context.Context, id string, version int) error {
	return c.doJSON(ctx, http.MethodDelete, "/webhook_definitions/"+id, versionHeader(version), nil, nil)
}
