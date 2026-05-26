package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"stratadb/db"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	database, err := db.Open(t.TempDir(), 1024, 4)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return httptest.NewServer(New(database).Handler())
}

func TestPutAndGet(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// PUT
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/keys/color", strings.NewReader("blue"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d", resp.StatusCode)
	}

	// GET
	resp, err = http.Get(ts.URL + "/keys/color")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "blue" {
		t.Errorf("expected 'blue', got %q", string(body))
	}
}

func TestGetNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/keys/ghost")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDelete(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/keys/temp", strings.NewReader("here"))
	http.DefaultClient.Do(req)

	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/keys/temp", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	resp, _ = http.Get(ts.URL + "/keys/temp")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestMissingKey(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/keys/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty key, got %d", resp.StatusCode)
	}
}
