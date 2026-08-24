package client

import (
	"context"
	"encoding/json"
	"fmt"
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

// WebhookFilter is one entry of a webhook's filter list, restricting which
// events the webhook is called for based on a property of the triggering
// entity (Property, e.g. "sys.environment.sys.id"). Exactly one of Equals,
// In or Regexp must be set; Negate inverts the match (Contentful's "not").
type WebhookFilter struct {
	Property string
	Equals   *string
	In       []string
	Regexp   *string
	Negate   bool
}

// webhookFilterDoc is the {"doc": "..."} constraint Contentful uses to
// reference a property of the entity in a filter expression.
type webhookFilterDoc struct {
	Doc string `json:"doc"`
}

// webhookFilterPattern is the argument shape for the "regexp" operator.
type webhookFilterPattern struct {
	Pattern string `json:"pattern"`
}

// MarshalJSON renders the filter in Contentful's positional-array shape,
// e.g. {"equals":[{"doc":"sys.environment.sys.id"},"master"]}, optionally
// wrapped once more in {"not":{...}} when Negate is set.
func (f WebhookFilter) MarshalJSON() ([]byte, error) {
	doc := webhookFilterDoc{Doc: f.Property}

	var inner map[string]any
	switch {
	case f.Equals != nil:
		inner = map[string]any{"equals": []any{doc, *f.Equals}}
	case f.In != nil:
		inner = map[string]any{"in": []any{doc, f.In}}
	case f.Regexp != nil:
		inner = map[string]any{"regexp": []any{doc, webhookFilterPattern{Pattern: *f.Regexp}}}
	default:
		return nil, fmt.Errorf("webhook filter for %q must set exactly one of equals, in or regexp", f.Property)
	}

	if f.Negate {
		return json.Marshal(map[string]any{"not": inner})
	}
	return json.Marshal(inner)
}

// UnmarshalJSON parses Contentful's positional-array filter shape back into
// a WebhookFilter. It only understands the operators this provider can
// express (equals/in/regexp, optionally wrapped in a single "not"); any
// other shape (e.g. "and"/"or", multi-level "not") is reported as an error.
func (f *WebhookFilter) UnmarshalJSON(data []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("parsing webhook filter: %w", err)
	}

	negate := false
	if raw, ok := obj["not"]; ok {
		if len(obj) != 1 {
			return fmt.Errorf("unsupported webhook filter shape: %q combined with other keys", "not")
		}
		// Unmarshal into a fresh map: json.Unmarshal reuses (rather than
		// clears) an existing non-nil map, which would leave the outer
		// "not" key behind alongside the inner operator key.
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(raw, &inner); err != nil {
			return fmt.Errorf("parsing negated webhook filter: %w", err)
		}
		obj = inner
		negate = true
	}

	if len(obj) != 1 {
		return fmt.Errorf("unsupported webhook filter shape: expected exactly one of equals, in or regexp")
	}

	for op, raw := range obj {
		var doc webhookFilterDoc
		switch op {
		case "equals":
			var args [2]json.RawMessage
			var value string
			if err := unmarshalFilterArgs(raw, &args, &doc, &value); err != nil {
				return err
			}
			*f = WebhookFilter{Property: doc.Doc, Equals: &value, Negate: negate}
		case "in":
			var args [2]json.RawMessage
			var values []string
			if err := unmarshalFilterArgs(raw, &args, &doc, &values); err != nil {
				return err
			}
			*f = WebhookFilter{Property: doc.Doc, In: values, Negate: negate}
		case "regexp":
			var args [2]json.RawMessage
			var pattern webhookFilterPattern
			if err := unmarshalFilterArgs(raw, &args, &doc, &pattern); err != nil {
				return err
			}
			*f = WebhookFilter{Property: doc.Doc, Regexp: &pattern.Pattern, Negate: negate}
		default:
			return fmt.Errorf("unsupported webhook filter operator %q", op)
		}
	}
	return nil
}

// unmarshalFilterArgs decodes a two-element filter argument array (raw) into
// [doc constraint, operator-specific value].
func unmarshalFilterArgs(raw json.RawMessage, args *[2]json.RawMessage, doc *webhookFilterDoc, value any) error {
	if err := json.Unmarshal(raw, args); err != nil {
		return fmt.Errorf("parsing webhook filter arguments: %w", err)
	}
	if err := json.Unmarshal(args[0], doc); err != nil {
		return fmt.Errorf("parsing webhook filter property: %w", err)
	}
	if err := json.Unmarshal(args[1], value); err != nil {
		return fmt.Errorf("parsing webhook filter value: %w", err)
	}
	return nil
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

	// Filters restricts which events the webhook fires for.
	Filters []WebhookFilter `json:"filters,omitempty"`

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

	Filters []WebhookFilter `json:"filters,omitempty"`

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

// CreateWebhook creates a new webhook definition from the given draft in the
// given space.
func (c *Client) CreateWebhook(ctx context.Context, spaceID string, draft WebhookDraft) (*WebhookDefinition, error) {
	var wh WebhookDefinition
	if err := c.doJSON(ctx, http.MethodPost, spaceID, "/webhook_definitions", nil, draft, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// GetWebhook fetches a webhook definition by its id within the given space.
func (c *Client) GetWebhook(ctx context.Context, spaceID, id string) (*WebhookDefinition, error) {
	var wh WebhookDefinition
	if err := c.doJSON(ctx, http.MethodGet, spaceID, "/webhook_definitions/"+id, nil, nil, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// UpdateWebhook replaces a webhook definition at the specified version.
func (c *Client) UpdateWebhook(ctx context.Context, spaceID, id string, version int, draft WebhookDraft) (*WebhookDefinition, error) {
	var wh WebhookDefinition
	if err := c.doJSON(ctx, http.MethodPut, spaceID, "/webhook_definitions/"+id, versionHeader(version), draft, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// DeleteWebhook deletes a webhook definition by id at the specified version.
func (c *Client) DeleteWebhook(ctx context.Context, spaceID, id string, version int) error {
	return c.doJSON(ctx, http.MethodDelete, spaceID, "/webhook_definitions/"+id, versionHeader(version), nil, nil)
}
