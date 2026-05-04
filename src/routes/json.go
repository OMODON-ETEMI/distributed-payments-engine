package routes

import (
	"encoding/json"
	"log"
	"net/http"
)

func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	if statusCode > 499 {
		log.Println("Responding with 5XX error: ", message)
	}
	type errResponse struct {
		Error string `json:"error"`
	}
	respondeWithJson(w, statusCode, errResponse{
		Error: message,
	})
}

func respondeWithJson(w http.ResponseWriter, statusCode int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal Json response: %v", payload)
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(data)
}

var RespondWithError = respondWithError
var RespondWithJson = respondeWithJson
