package dashboard

import (
	"io/fs"
	"net/http"
	"strings"
)

// Handler serves the embedded operator UI. Unknown paths fall back to
// index.html so React Router can handle /login and /cluster/:id/*.
func Handler() (http.Handler, error) {
	sub, err := Dist()
	if err != nil {
		return nil, err
	}
	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		serveIndex := path == ""
		if !serveIndex {
			f, openErr := sub.Open(path)
			if openErr != nil {
				serveIndex = true
			} else {
				st, stErr := f.Stat()
				_ = f.Close()
				if stErr == nil && st.IsDir() {
					serveIndex = true
				}
			}
		}
		if serveIndex {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			_, _ = w.Write(indexHTML)
			return
		}
		files.ServeHTTP(w, r)
	}), nil
}
