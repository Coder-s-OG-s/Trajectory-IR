package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAboutHomeFacts(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedEconomy(t, st, "econ-a")
	pkgDir := filepath.Join(st.Root(), packagesDir, "econ-a")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "demo.tir"), []byte("tir"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := st.About()
	if err != nil {
		t.Fatal(err)
	}
	if got.Product != "Trajectory console" {
		t.Fatalf("product %q", got.Product)
	}
	if got.License.Name != "Apache License 2.0" || got.License.SPDX != "Apache-2.0" {
		t.Fatalf("license %+v", got.License)
	}
	if got.License.Paid || got.License.Expires != nil {
		t.Fatalf("must not look like a paid or expiring product key: %+v", got.License)
	}
	if got.DataDir != st.Root() {
		t.Fatalf("data_dir %q want %q", got.DataDir, st.Root())
	}
	if got.RunCount != 1 || got.EventCount != 3 || got.PackageCount != 1 {
		t.Fatalf("counts runs=%d events=%d packages=%d", got.RunCount, got.EventCount, got.PackageCount)
	}
	if got.DataBytes <= 0 {
		t.Fatalf("data_bytes %d", got.DataBytes)
	}
	if got.Health != "ok" || !got.LoopbackTools {
		t.Fatalf("health %+v", got)
	}
}

func TestAboutHTTP(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedEconomy(t, st, "econ")
	srv := NewServer(st, "secret")

	req := httptest.NewRequest(http.MethodGet, "/v1/about", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/about", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var got AboutView
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.AuthRequired {
		t.Fatal("auth_required should be true when the server has a token")
	}
	if got.License.SPDX != "Apache-2.0" || got.RunCount != 1 {
		t.Fatalf("about %+v", got)
	}
}

func TestAboutUIShell(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	html := rr.Body.String()
	for _, want := range []string{
		`id="about"`,
		`id="open-about"`,
		"License",
		"Apache License 2.0",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in html", want)
		}
	}

	req2 := httptest.NewRequest(http.MethodGet, "/ui/app.js", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	js := rr2.Body.String()
	for _, want := range []string{"/v1/about", "renderAbout", "Apache License 2.0"} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q in js", want)
		}
	}
	for _, banned := range []string{
		"SUBNET", "billed tokens", "product key expires", "\u2014", "\u2013",
	} {
		if strings.Contains(html, banned) || strings.Contains(js, banned) {
			t.Fatalf("about copy must not contain %q", banned)
		}
	}
}
