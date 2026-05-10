package routes

import "net/http"

// HandleHealthCheck returns a 200 OK status, indicating the service is alive.
// This is a basic liveness check. For a readiness check, you might want to
// check database, Redis, and Kafka connections.
// @Summary Health check endpoint
// @Description Returns 200 OK if the service is alive and responding. Used for liveness probes.
// @Tags Health
// @Produce plain
// @Success 200 {string} string "OK"
// @Failure 500 {object} errResponse
// @Router /healthz [get]
func HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
