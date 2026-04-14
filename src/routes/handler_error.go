package routes

import "net/http"

func HandleError(w http.ResponseWriter, r *http.Request) {
	respondWithError(w, 400, "server error")
}
