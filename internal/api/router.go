package api

import "net/http"

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/jobs", h.CreateJob)
	mux.HandleFunc("GET /api/jobs", h.ListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", h.GetJob)
	mux.HandleFunc("PUT /api/jobs/{id}", h.UpdateJob)
	mux.HandleFunc("DELETE /api/jobs/{id}", h.DeleteJob)
	mux.HandleFunc("POST /api/jobs/{id}/trigger", h.TriggerJob)
	mux.HandleFunc("GET /api/jobs/{id}/executions", h.ListExecutions)
	mux.HandleFunc("GET /api/health", h.Health)

	return withCORS(mux)
}

// withCORS allows the Vue dashboard (served separately, e.g. from a
// different port during local dev) to call this API from the browser.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
