package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/services"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/go-chi/chi"
	"github.com/jackc/pgx/v5"
)

// HandleCreateUser creates a new customer/user.
// @Summary Create a new customer/user
// @Description Creates a new customer account with the provided information. Idempotent by external_ref.
// @Tags Users
// @Accept json
// @Produce json
// @Param body body parameters true "User Creation Details"
// @Success 200 {object} UserResponse
// @Failure 400 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /create/user [post]
func (api *ApiConfig) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := internal.UserParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}

	if params.ExternalRef == "" || params.FullName == "" || params.Email == "" {
		internal.RespondWithError(w, 400, "missing required fields: external_ref, full_name or email")
		return
	}

	user, err := services.CreateUser(r.Context(), params, api.Db.Queries)
	if err != nil {
		internal.RespondWithError(w, 500, err.Error())
		return
	}
	internal.RespondWithJson(w, 200, user)
}

func (api *ApiConfig) HandleGetUserByExternalRef(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := internal.UserParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}
	if params.ExternalRef == "" {
		internal.RespondWithError(w, 400, "external_ref is required")
		return
	}
	user, err := services.GetUserByExternalRef(r.Context(), params.ExternalRef, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "user not found")
			return
		}
		internal.RespondWithError(w, 500, err.Error())
		return
	}
	internal.RespondWithJson(w, 200, user)
}

// HandleGetUserById retrieves a specific customer by their UUID.
// @Summary Get user by ID
// @Description Retrieves a specific customer by their UUID.
// @Tags Users
// @Produce json
// @Param id path string true "Customer UUID" format(uuid)
// @Success 200 {object} UserResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /user/{id} [get]
func (api *ApiConfig) HandleGetUserById(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		internal.RespondWithError(w, 400, "User Id is required")
		return
	}

	user, err := services.GetUserByID(r.Context(), idStr, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "user not found")
			return
		}
		internal.RespondWithError(w, 500, err.Error())
		return
	}
	internal.RespondWithJson(w, 200, user)
}

// HandleUserUpdateStatus updates the status of an existing customer.
// @Summary Update user status
// @Description Updates the status of an existing customer (e.g., active, inactive, suspended).
// @Tags Users
// @Accept json
// @Produce json
// @Param body body parameters true "User Status Update Details (requires id and status)"
// @Success 200 {object} UserResponse
// @Failure 400 {object} errResponse
// @Failure 404 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /update/user [post]
func (api *ApiConfig) HandleUserUpdateStatus(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := internal.UserParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}
	if params.Status == "" {
		internal.RespondWithError(w, 400, "status is required")
		return
	}
	user, err := services.UpdateUserStatus(r.Context(), params, api.Db.Queries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			internal.RespondWithError(w, 404, "user not found")
			return
		}
		internal.RespondWithError(w, 500, err.Error())
		return
	}
	internal.RespondWithJson(w, 200, user)
}

// HandleListCustomers returns a paginated list of customers.
// @Summary List customers with pagination
// @Description Returns paginated list of customers with optional limit and offset.
// @Tags Users
// @Accept json
// @Produce json
// @Param body body parameters true "Pagination details"
// @Success 200 {array} UserResponse
// @Failure 400 {object} errResponse
// @Failure 500 {object} errResponse
// @Router /list/users [post]
func (api *ApiConfig) HandleListCustomers(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := internal.UserParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		internal.RespondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}

	users, err := services.ListUsers(r.Context(), params, api.Db.Queries)
	if err != nil {
		internal.RespondWithError(w, 500, err.Error())
		return
	}

	internal.RespondWithJson(w, 200, users)
}
