package routes

import "net/http"

func HandleReadiness(w http.ResponseWriter, r *http.Request) {
	respondeWithJson(w, 200, struct{}{})
}
