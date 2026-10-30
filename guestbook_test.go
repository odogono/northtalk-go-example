package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestbook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guestbook.sqlite")
	g, err := Open(path, []string{"spam"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(g)
	defer server.Close()

	call := func(method, body string, wantStatus int, want string) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+"/entries", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		got, _ := io.ReadAll(res.Body)
		if res.StatusCode != wantStatus || string(got) != want {
			t.Fatalf("%s %s: %d %s, want %d %s", method, body, res.StatusCode, got, wantStatus, want)
		}
	}

	call("GET", "", 200, `[]`)
	call("POST", `{"author": "ada", "message": "  hello  "}`, 201, `1`)
	call("POST", `{"author": "bob", "message": "buy spam"}`, 422, `{"code":"blocked word","word":"spam"}`)
	call("POST", `{"author": "bob", "message": " "}`, 422, `{"code":"empty message"}`)
	call("POST", `{"author": "cy", "message": "hi"}`, 201, `2`)
	call("GET", "", 200, `[{"id":1,"author":"ada","message":"hello"},{"id":2,"author":"cy","message":"hi"}]`)

	// Entries outlive the server: reopen the database and list them again.
	server.Close()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if g, err = Open(path, nil); err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	server = httptest.NewServer(g)
	call("GET", "", 200, `[{"id":1,"author":"ada","message":"hello"},{"id":2,"author":"cy","message":"hi"}]`)
}
