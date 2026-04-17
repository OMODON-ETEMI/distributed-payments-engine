package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type parameters struct {
	IdempKey    string                 `json:"idemp_key"`
	ID          string                 `json:"id"`
	ExternalRef string                 `json:"external_ref"`
	FullName    string                 `json:"full_name"`
	Email       string                 `json:"email"`
	Phone       string                 `json:"phone"`
	NationalID  string                 `json:"national_id"`
	Status      string                 `json:"status"`
	Metadata    map[string]interface{} `json:"metadata"`
	Limit       int                    `json:"limit"`
	Offset      int                    `json:"offset"`
}

func (api *ApiConfig) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}

	// basic validation
	if params.ExternalRef == "" || params.FullName == "" || params.Email == "" {
		respondWithError(w, 400, "missing required fields: external_ref, full_name or email")
		return
	}

	// idempotency: if a customer with the external_ref exists, return it
	existing, err := api.Db.GetCustomerByExternalRef(r.Context(), params.ExternalRef)
	if err == nil {
		respondeWithJson(w, 200, UserResponseObject(existing))
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		respondWithError(w, 500, fmt.Sprintf("Error checking existing user: %v", err))
		return
	}

	metadataBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}

	user, err := api.Db.CreateCustomer(r.Context(), database.CreateCustomerParams{
		ExternalRef: params.ExternalRef,
		FullName:    params.FullName,
		Email:       params.Email,
		Phone:       pgtype.Text{String: params.Phone, Valid: params.Phone != ""},
		NationalID:  pgtype.Text{String: params.NationalID, Valid: params.NationalID != ""},
		Status:      params.Status,
		Metadata:    metadataBytes,
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating Customer: %v", err))
		return
	}
	respondeWithJson(w, 200, UserResponseObject(user))

}

func (api *ApiConfig) HandleGetUserByExternalRef(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}
	if params.ExternalRef == "" {
		respondWithError(w, 400, "external_ref is required")
		return
	}
	user, err := api.Db.GetCustomerByExternalRef(r.Context(), params.ExternalRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "user not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error getting user by External Ref: %v", err))
		return
	}
	respondeWithJson(w, 200, UserResponseObject(user))
}

func (api *ApiConfig) HandleGetUserById(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}
	id, err := StringtoPgUuid(params.ID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}
	user, err := api.Db.GetCustomerByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "user not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error getting user by ID: %v", err))
		return
	}
	respondeWithJson(w, 200, UserResponseObject(user))
}

func (api *ApiConfig) HandleUserUpdateStatus(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}
	id, err := StringtoPgUuid(params.ID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing ID: %v", err))
		return
	}
	if params.Status == "" {
		respondWithError(w, 400, "status is required")
		return
	}
	user, err := api.Db.UpdateCustomerStatus(r.Context(), database.UpdateCustomerStatusParams{
		ID:     id,
		Status: params.Status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "user not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error updating user status: %v", err))
		return
	}
	respondeWithJson(w, 200, UserResponseObject(user))
}

func (api *ApiConfig) HandleListCustomers(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}

	limit := int32(50)
	offset := int32(0)
	if params.Limit > 0 {
		limit = int32(params.Limit)
	}
	if params.Offset > 0 {
		offset = int32(params.Offset)
	}
	if limit > 1000 {
		limit = 1000
	}

	users, err := api.Db.ListCustomers(r.Context(), database.ListCustomersParams{Limit: limit, Offset: offset})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error listing users: %v", err))
		return
	}

	var out []UserResponse
	for _, u := range users {
		out = append(out, UserResponseObject(u))
	}
	respondeWithJson(w, 200, out)
}

// {
//   "id": "61fd6667-9a2e-476e-a4f2-733f205d8c52",
//   "external_ref": "ext_ref_77889",
//   "full_name": "Oritsetemi Glory Omodon",
//   "email": "glory@example.com",
//   "phone": "+2348012345678",
//   "national_id": "NIN-123456789",
//   "status": "active",
//   "metadata": "eyJyaXNrX3Njb3JlIjogMC41LCAicmVmZXJyZWRfYnkiOiAic3RhcnR1cF9hbHBoYSIsICJvbmJvYXJkaW5nX3NvdXJjZSI6ICJ3ZWJfYXBwIn0=",
//   "created_at": "2026-04-16T18:12:20.821268Z",
//   "updated_at": "2026-04-16T18:12:20.821268Z",
//   "deleted_at": null
// }

// {
//   "idemp_key": "user_create_9921_abc",
//   "external_ref": "ext_ref_77889",
//   "full_name": "Oritsetemi Glory Omodon",
//   "email": "glory@example.com",
//   "phone": "+2348012345678",
//   "national_id": "NIN-123456789",
//   "status": "active",
//   "metadata": {
//     "onboarding_source": "web_app",
//     "referred_by": "startup_alpha",
//     "risk_score": 0.5
//   }
// }
