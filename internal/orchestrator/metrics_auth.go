package orchestrator

import (
	"net/http"
	"strings"
)

// MetricsAuthorized reports whether a scrape of orchestrator /metrics is allowed.
// Local -insecure-http keeps Prometheus working without a header. Production
// TLS requires a Bearer JWT or the internal API token.
func MetricsAuthorized(r *http.Request, jwtSecret []byte, internalToken string, insecureHTTP bool) bool {
	if insecureHTTP {
		return true
	}
	header := r.Header.Get("Authorization")
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	token := parts[1]
	if internalToken != "" && token == internalToken {
		return true
	}
	if len(jwtSecret) >= 32 {
		if _, err := ValidateToken(token, jwtSecret); err == nil {
			return true
		}
	}
	return false
}
