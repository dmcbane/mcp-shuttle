package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeaderTransport_InjectsHeaders(t *testing.T) {
	// Set up a test server that echoes back the received headers.
	var gotAuth, gotCustom string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &http.Client{
		Transport: HeaderTransport(nil, map[string]string{
			"Authorization": "Bearer test-token",
			"X-Custom":      "custom-value",
		}),
	}

	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotAuth != "Bearer test-token" {
		t.Errorf("got Authorization=%q, want %q", gotAuth, "Bearer test-token")
	}
	if gotCustom != "custom-value" {
		t.Errorf("got X-Custom=%q, want %q", gotCustom, "custom-value")
	}
}

func TestHeaderTransport_PreservesExistingHeaders(t *testing.T) {
	var gotExisting string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotExisting = r.Header.Get("Existing")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &http.Client{
		Transport: HeaderTransport(nil, map[string]string{
			"X-Added": "value",
		}),
	}

	req, _ := http.NewRequest("GET", ts.URL, nil)
	req.Header.Set("Existing", "keep-me")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotExisting != "keep-me" {
		t.Errorf("got Existing=%q, want %q", gotExisting, "keep-me")
	}
}
