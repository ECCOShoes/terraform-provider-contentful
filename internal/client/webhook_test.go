package client

import (
	"context"
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
	return &Client{httpClient: httpClient, apiURL: srv.URL, spaceID: "test-space"}
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

	wh, err := c.CreateWebhook(context.Background(), WebhookDraft{Name: "n", URL: "https://example.com", Topics: []string{"Entry.publish"}})
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

	_, err := c.GetWebhook(context.Background(), "missing")
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

	wh, err := c.UpdateWebhook(context.Background(), "wh1", 1, WebhookDraft{Name: "n2", URL: "https://example.com", Topics: []string{"Entry.publish"}})
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

	if err := c.DeleteWebhook(context.Background(), "wh1", 3); err != nil {
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

	wh, err := c.GetWebhook(context.Background(), "wh1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wh.Headers) != 1 || !wh.Headers[0].Secret || wh.Headers[0].Value != "" {
		t.Errorf("expected one secret header with empty value, got %+v", wh.Headers)
	}
}
