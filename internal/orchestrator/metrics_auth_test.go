package orchestrator

import (
	"net/http"
	"testing"
)

func TestMetricsAuthorized(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	internal := "internal-token"
	jwt, err := GenerateToken("admin", secret)
	if err != nil {
		t.Fatal(err)
	}

	req := func(auth string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		return r
	}

	if !MetricsAuthorized(req(""), secret, internal, true) {
		t.Fatal("insecure-http must allow scrapes")
	}
	if MetricsAuthorized(req(""), secret, internal, false) {
		t.Fatal("production scrape without token")
	}
	if !MetricsAuthorized(req("Bearer "+internal), secret, internal, false) {
		t.Fatal("internal token")
	}
	if !MetricsAuthorized(req("Bearer "+jwt), secret, internal, false) {
		t.Fatal("operator JWT")
	}
	if MetricsAuthorized(req("Bearer nope"), secret, internal, false) {
		t.Fatal("garbage token")
	}
}
