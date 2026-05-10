package routes

import "net/http"

// HandleError is a test endpoint for error handling validation.
// @Summary Error test endpoint
// @Description Testing endpoint for error handling validation.
// @Tags Health
// @Produce json
// @Failure 400 {object} errResponse
// @Router /err [get]
func HandleError(w http.ResponseWriter, r *http.Request) {
	respondWithError(w, 400, "server error")
}
