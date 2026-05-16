package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func CreateUser(ctx context.Context, params internal.UserParameters, queries *database.Queries) (*internal.UserResponse, error) {
	existing, err := queries.GetCustomerByExternalRef(ctx, params.ExternalRef)
	if err == nil {
		resp := internal.UserResponseObject(existing)
		return &resp, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	if params.Metadata == nil {
		params.Metadata = make(map[string]interface{})
	}
	metadataBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		return nil, fmt.Errorf("error parsing metadata: %w", err)
	}

	user, err := queries.CreateCustomer(ctx, database.CreateCustomerParams{
		ExternalRef: params.ExternalRef,
		FullName:    params.FullName,
		Email:       params.Email,
		Phone:       pgtype.Text{String: params.Phone, Valid: params.Phone != ""},
		NationalID:  pgtype.Text{String: params.NationalID, Valid: params.NationalID != ""},
		Status:      params.Status,
		Metadata:    metadataBytes,
	})
	if err != nil {
		return nil, err
	}

	resp := internal.UserResponseObject(user)
	return &resp, nil
}

func GetUserByExternalRef(ctx context.Context, externalRef string, queries *database.Queries) (*internal.UserResponse, error) {
	user, err := queries.GetCustomerByExternalRef(ctx, externalRef)
	if err != nil {
		return nil, err
	}
	resp := internal.UserResponseObject(user)
	return &resp, nil
}

func GetUserByID(ctx context.Context, ID string, queries *database.Queries) (*internal.UserResponse, error) {
	id, err := internal.StringtoPgUuid(ID)
	if err != nil {
		return nil, fmt.Errorf("invalid ID: %w", err)
	}
	user, err := queries.GetCustomerByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := internal.UserResponseObject(user)
	return &resp, nil
}

func UpdateUserStatus(ctx context.Context, params internal.UserParameters, queries *database.Queries) (*internal.UserResponse, error) {
	id, err := internal.StringtoPgUuid(params.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid ID: %w", err)
	}
	user, err := queries.UpdateCustomerStatus(ctx, database.UpdateCustomerStatusParams{
		ID:     id,
		Status: params.Status,
	})
	if err != nil {
		return nil, err
	}
	resp := internal.UserResponseObject(user)
	return &resp, nil
}

func ListUsers(ctx context.Context, params internal.UserParameters, queries *database.Queries) ([]internal.UserResponse, error) {
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

	users, err := queries.ListCustomers(ctx, database.ListCustomersParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}

	out := make([]internal.UserResponse, 0, len(users))
	for _, u := range users {
		out = append(out, internal.UserResponseObject(u))
	}
	return out, nil
}
