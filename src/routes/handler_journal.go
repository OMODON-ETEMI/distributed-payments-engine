package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type JournalParameters struct {
	JournalTransactionID string `json:"journal_transaction_id"`
}

// func (api *ApiConfig) HandleGetJournalTransactionByRef(w http.ResponseWriter, r *http.Request) {
// 	decoder := json.NewDecoder(r.Body)
// 	var payload struct {
// 		TransactionRef string `json:"transaction_ref"`
// 	}
// 	if err := decoder.Decode(&payload); err != nil {
// 		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
// 		return
// 	}
// 	if payload.TransactionRef == "" {
// 		respondWithError(w, 400, "transaction_ref is required")
// 		return
// 	}
// 	jt, err := api.Db.GetJournalTransactionByRef(r.Context(), payload.TransactionRef)
// 	if err != nil {
// 		if errors.Is(err, pgx.ErrNoRows) {
// 			respondWithError(w, 404, "journal transaction not found")
// 			return
// 		}
// 		respondWithError(w, 500, fmt.Sprintf("Error getting journal transaction: %v", err))
// 		return
// 	}
// 	respondeWithJson(w, 200, JournalTransactionResponseObject(jt))
// }

func (api *ApiConfig) HandleListJournalLinesForTransaction(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	payload := JournalParameters{}
	if err := decoder.Decode(&payload); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if payload.JournalTransactionID == "" {
		respondWithError(w, 400, "journal_transaction_id is required")
		return
	}
	txID, err := StringtoPgUuid(payload.JournalTransactionID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing journal_transaction_id: %v", err))
		return
	}
	lines, err := api.Db.Queries.ListJournalLinesForTransaction(r.Context(), txID)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error listing journal lines: %v", err))
		return
	}
	out := make([]JournalLineResponse, 0, len(lines))
	for _, l := range lines {
		out = append(out, JournalLineResponseObject(l))
	}
	respondeWithJson(w, 200, out)
}
