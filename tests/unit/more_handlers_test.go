package routes

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
)

func TestUserAndAccountHandlers_EarlyValidation(t *testing.T) {
	api := &routes.ApiConfig{}

	// GetUserByExternalRef -> missing external_ref
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/user/external", bytes.NewBufferString(`{}`))
	api.HandleGetUserByExternalRef(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleGetUserByExternalRef: expected 400 for missing external_ref got %d", w.Code)
	}

	// GetUserById -> invalid id
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/user/id", bytes.NewBufferString(`{"id":"not-a-uuid"}`))
	api.HandleGetUserById(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleGetUserById: expected 400 for invalid id got %d", w.Code)
	}

	// HandleUserUpdateStatus -> missing status
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/user/update-status", bytes.NewBufferString(`{"id":"00000000-0000-0000-0000-000000000000"}`))
	api.HandleUserUpdateStatus(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleUserUpdateStatus: expected 400 for missing status got %d", w.Code)
	}

	// HandleGetAccountByAccountNumber -> missing account_number
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/account/number", bytes.NewBufferString(`{}`))
	api.HandleGetAccountByAccountNumber(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleGetAccountByAccountNumber: expected 400 for missing account_number got %d", w.Code)
	}

	// HandleListAccountByCustomer -> missing customer_id
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/accounts/list", bytes.NewBufferString(`{}`))
	api.HandleListAccountByCustomer(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleListAccountByCustomer: expected 400 for missing customer_id got %d", w.Code)
	}

	// HandleUpdateAccountStatus -> missing status
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/account/update-status", bytes.NewBufferString(`{"id":"00000000-0000-0000-0000-000000000000"}`))
	api.HandleUpdateAccountStatus(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleUpdateAccountStatus: expected 400 for missing status got %d", w.Code)
	}
}

func TestConsumeHold_InvalidAmount(t *testing.T) {
	api := &routes.ApiConfig{}

	// invalid amount should yield 400
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/admin/hold/consume", bytes.NewBufferString(`{"amount":"not-a-number","id":"00000000-0000-0000-0000-000000000000"}`))
	api.ConsumeHold(w, req)
	if w.Code != 400 {
		t.Fatalf("ConsumeHold: expected 400 for invalid amount got %d", w.Code)
	}
}
