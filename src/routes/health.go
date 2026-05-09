package routes

import "net/http"

// HandleHealthCheck returns a 200 OK status, indicating the service is alive.
// This is a basic liveness check. For a readiness check, you might want to
// check database, Redis, and Kafka connections.
func HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
