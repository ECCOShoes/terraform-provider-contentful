package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	httpClient := srv.Client()
	httpClient.Transport = &bearerTokenTransport{token: "test-token", base: httpClient.Transport}
	return &Client{httpClient: httpClient, apiURL: srv.URL}
}

func TestCreateWebhook(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/spaces/test-space/webhook_definitions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Authorization header to be set, got %q", got)
		}
		_, _ = w.Write([]byte(`{"sys":{"id":"wh1","version":1},"name":"n","url":"https://example.com","topics":["Entry.publish"],"active":true}`))
	})

	wh, err := c.CreateWebhook(context.Background(), "test-space", WebhookDraft{Name: "n", URL: "https://example.com", Topics: []string{"Entry.publish"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wh.ID != "wh1" || wh.Version != 1 {
		t.Errorf("got id=%q version=%d, want id=wh1 version=1", wh.ID, wh.Version)
	}
}

func TestGetWebhook_notFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"sys":{"type":"Error","id":"NotFound"},"message":"not found"}`))
	})

	_, err := c.GetWebhook(context.Background(), "test-space", "missing")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected IsNotFound to be true, got %v", err)
	}
}

func TestUpdateWebhook_sendsVersionHeader(t *testing.T) {
	var gotVersion string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("X-Contentful-Version")
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"sys":{"id":"wh1","version":2},"name":"n2","url":"https://example.com","topics":["Entry.publish"]}`))
	})

	wh, err := c.UpdateWebhook(context.Background(), "test-space", "wh1", 1, WebhookDraft{Name: "n2", URL: "https://example.com", Topics: []string{"Entry.publish"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVersion != "1" {
		t.Errorf("expected X-Contentful-Version=1, got %q", gotVersion)
	}
	if wh.Version != 2 {
		t.Errorf("expected version 2 in response, got %d", wh.Version)
	}
}

func TestDeleteWebhook(t *testing.T) {
	var called bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if got := r.Header.Get("X-Contentful-Version"); got != "3" {
			t.Errorf("expected X-Contentful-Version=3, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteWebhook(context.Background(), "test-space", "wh1", 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected the server to be called")
	}
}

func TestWebhookDefinition_secretHeaderValueOmitted(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sys":{"id":"wh1","version":1},"name":"n","url":"https://example.com","topics":["Entry.publish"],"headers":[{"key":"X-Secret","secret":true}]}`))
	})

	wh, err := c.GetWebhook(context.Background(), "test-space", "wh1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wh.Headers) != 1 || !wh.Headers[0].Secret || wh.Headers[0].Value != "" {
		t.Errorf("expected one secret header with empty value, got %+v", wh.Headers)
	}
}

func TestWebhookFilter_marshal(t *testing.T) {
	equals := "master"
	regexp := "^ci-.+$"

	tests := []struct {
		name   string
		filter WebhookFilter
		want   string
	}{
		{
			name:   "equals",
			filter: WebhookFilter{Property: "sys.environment.sys.id", Equals: &equals},
			want:   `{"equals":[{"doc":"sys.environment.sys.id"},"master"]}`,
		},
		{
			name:   "not equals",
			filter: WebhookFilter{Property: "sys.environment.sys.id", Equals: &equals, Negate: true},
			want:   `{"not":{"equals":[{"doc":"sys.environment.sys.id"},"master"]}}`,
		},
		{
			name:   "in",
			filter: WebhookFilter{Property: "sys.contentType.sys.id", In: []string{"blogPost", "author"}},
			want:   `{"in":[{"doc":"sys.contentType.sys.id"},["blogPost","author"]]}`,
		},
		{
			name:   "not regexp",
			filter: WebhookFilter{Property: "sys.id", Regexp: &regexp, Negate: true},
			want:   `{"not":{"regexp":[{"doc":"sys.id"},{"pattern":"^ci-.+$"}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.filter)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() = %s, want %s", got, tt.want)
			}

			var roundTripped WebhookFilter
			if err := json.Unmarshal(got, &roundTripped); err != nil {
				t.Fatalf("unexpected error unmarshaling: %v", err)
			}
			if !equalFilters(roundTripped, tt.filter) {
				t.Errorf("round trip = %+v, want %+v", roundTripped, tt.filter)
			}
		})
	}
}

// equalFilters compares two WebhookFilter values by their pointer-dereferenced
// content (WebhookFilter contains pointer/slice fields, so == doesn't apply).
func equalFilters(a, b WebhookFilter) bool {
	if a.Property != b.Property || a.Negate != b.Negate {
		return false
	}
	if (a.Equals == nil) != (b.Equals == nil) || (a.Equals != nil && *a.Equals != *b.Equals) {
		return false
	}
	if (a.Regexp == nil) != (b.Regexp == nil) || (a.Regexp != nil && *a.Regexp != *b.Regexp) {
		return false
	}
	if len(a.In) != len(b.In) {
		return false
	}
	for i := range a.In {
		if a.In[i] != b.In[i] {
			return false
		}
	}
	return true
}

func TestWebhookFilter_unmarshalUnsupportedShape(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "and", body: `{"and":[{"equals":[{"doc":"sys.id"},"a"]},{"equals":[{"doc":"sys.id"},"b"]}]}`},
		{name: "double not", body: `{"not":{"not":{"equals":[{"doc":"sys.id"},"a"]}}}`},
		{name: "unknown operator", body: `{"contains":[{"doc":"sys.id"},"a"]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f WebhookFilter
			if err := json.Unmarshal([]byte(tt.body), &f); err == nil {
				t.Errorf("expected an error for unsupported shape %q", tt.body)
			}
		})
	}
}
