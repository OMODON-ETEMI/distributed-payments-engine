package routes

import (
	"encoding/json"
	"time"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
)

type UserResponse struct {
	ID          string          `json:"id"`
	ExternalRef string          `json:"external_ref"`
	FullName    string          `json:"full_name"`
	Email       string          `json:"email"`
	Phone       string          `json:"phone"`
	NationalID  string          `json:"national_id"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   time.Time       `json:"deleted_at"`
}

func UserResponseObject(dbUser database.Customer) UserResponse {
	return UserResponse{
		ID:          dbUser.ID.String(),
		ExternalRef: dbUser.ExternalRef,
		FullName:    dbUser.FullName,
		Email:       dbUser.Email.String,
		Phone:       dbUser.Phone.String,
		NationalID:  dbUser.NationalID.String,
		Status:      dbUser.Status,
		Metadata:    dbUser.Metadata,
		CreatedAt:   dbUser.CreatedAt.Time,
		UpdatedAt:   dbUser.UpdatedAt.Time,
		DeletedAt:   dbUser.DeletedAt.Time,
	}
}
