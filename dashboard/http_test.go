package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerSPAFallback(t *testing.T) {
	h, err := Handler()
	if err != nil {
		t.Skip(err.Error())
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	for _, path := range []string{"/", "/login", "/cluster/acme/keys"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: HTTP %d", path, res.StatusCode)
		}
		if !strings.Contains(string(body), `id="root"`) {
			t.Fatalf("%s: expected SPA shell, got %q", path, truncate(string(body), 180))
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
