package tombi

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wasilibs/go-tombi/internal/runner"
)

//go:embed testdata/in
var inFiles embed.FS

//go:embed testdata/exp
var expFiles embed.FS

// TestLint lints an invalid TOML file and expects a non-zero exit with a
// diagnostic.
func TestLint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "invalid.toml"), []byte("a = = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdin := bytes.Buffer{}
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	ret := runner.Run("tombi", []string{"lint", "--offline", "invalid.toml"}, &stdin, &stdout, &stderr, dir)
	out := stdout.String() + stderr.String()
	if ret == 0 {
		t.Fatalf("expected non-zero exit (lint issues), got 0\noutput:\n%s", out)
	}
	if !strings.Contains(out, "Error") {
		t.Fatalf("expected an error diagnostic in output:\n%s", out)
	}
}

// TestFormat formats and compares with golden data.
// Run with UPDATE_GOLDEN=1 to regenerate testdata/exp.
func TestFormat(t *testing.T) {
	inFS, err := fs.Sub(inFiles, "testdata/in")
	if err != nil {
		t.Fatal(err)
	}
	expFS, err := fs.Sub(expFiles, "testdata/exp")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := fs.WalkDir(inFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("tombi: reading testdata: %w", err)
		}
		if d.IsDir() {
			return nil
		}
		c, _ := fs.ReadFile(inFS, path)
		return os.WriteFile(filepath.Join(dir, path), c, 0o644)
	}); err != nil {
		t.Fatal(err)
	}

	stdin := bytes.Buffer{}
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	ret := runner.Run("tombi", []string{"format", "--offline", "."}, &stdin, &stdout, &stderr, dir)
	if want := 0; ret != want {
		t.Fatalf("unexpected return code: have %d, want %d\nstdout:\n%s\nstderr:\n%s", ret, want, stdout.String(), stderr.String())
	}

	update := os.Getenv("UPDATE_GOLDEN") == "1"
	err = fs.WalkDir(inFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			return fmt.Errorf("tombi: reading formatted file: %w", err)
		}
		if update {
			if err := os.WriteFile(filepath.Join("testdata/exp", path), got, 0o644); err != nil {
				return fmt.Errorf("tombi: writing golden: %w", err)
			}
			return nil
		}
		want, err := fs.ReadFile(expFS, path)
		if err != nil {
			return fmt.Errorf("missing golden for %s: %w", path, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s mismatch:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if update {
		t.Log("updated testdata/exp")
	}
}

// TestFetchSchema exercises the WASI schema-fetch host API.
func TestFetchSchema(t *testing.T) {
	const schema = `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {"port": {"type": "integer"}}
	}`

	var hits atomic.Int32
	var gotPath atomic.Value // string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotPath.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(schema))
	}))
	defer srv.Close()

	dir := t.TempDir()
	// Disable the schemastore catalog so the only fetch is the explicit schema.
	if err := os.WriteFile(filepath.Join(dir, "tombi.toml"), []byte("[schema.catalog]\npaths = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// `port` must be an integer per the schema, but is a string here.
	doc := fmt.Sprintf("#:schema %s/port.json\nport = \"not-an-integer\"\n", srv.URL)
	if err := os.WriteFile(filepath.Join(dir, "doc.toml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	stdin := bytes.Buffer{}
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	ret := runner.Run("tombi", []string{"lint", "doc.toml"}, &stdin, &stdout, &stderr, dir)
	out := stdout.String() + stderr.String()

	if hits.Load() == 0 {
		t.Fatalf("host was never asked to fetch the schema\noutput:\n%s", out)
	}
	if got, _ := gotPath.Load().(string); got != "/port.json" {
		t.Fatalf("host fetched unexpected URL path %q, want %q\noutput:\n%s", got, "/port.json", out)
	}
	if ret == 0 {
		t.Fatalf("expected a schema violation (non-zero exit), got 0\noutput:\n%s", out)
	}
}
