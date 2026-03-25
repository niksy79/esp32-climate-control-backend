package api

import "net/http"

var corsAllowedOrigins = map[string]bool{
	"http://localhost:5173":         true,
	"http://127.0.0.1:5173":        true,
	"https://climate.gotocloud.xyz":     true,
	"https://climate-app.gotocloud.xyz": true,
}

// corsMiddleware sets CORS headers for allowed origins and short-circuits
// OPTIONS preflight requests with 200 OK.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if corsAllowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
