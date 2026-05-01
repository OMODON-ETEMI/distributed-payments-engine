package routes

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
)

func TestHandlers_EarlyValidation(t *testing.T) {
	api := &routes.ApiConfig{}

	// CreateUser -> missing required fields
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/users", bytes.NewBufferString(`{}`))
	api.HandleCreateUser(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleCreateUser: expected 400 for missing fields got %d", w.Code)
	}

	// CreateAccount -> missing required fields
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/accounts", bytes.NewBufferString(`{}`))
	api.HandleCreateAccount(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleCreateAccount: expected 400 for missing fields got %d", w.Code)
	}

	// CreateTransfer -> missing required fields
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/transfers", bytes.NewBufferString(`{}`))
	api.HandleCreateTransfer(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleCreateTransfer: expected 400 for missing fields got %d", w.Code)
	}

	// Deposite -> missing required fields
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/deposite", bytes.NewBufferString(`{}`))
	api.HandleDeposite(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleDeposite: expected 400 for missing fields got %d", w.Code)
	}

	// Withdraw -> missing required fields
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/withdraw", bytes.NewBufferString(`{}`))
	api.HandleWithdraw(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleWithdraw: expected 400 for missing fields got %d", w.Code)
	}

	// GetBalancesForAccount -> missing account_id
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/balances", bytes.NewBufferString(`{}`))
	api.HandleGetBalancesForAccount(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleGetBalancesForAccount: expected 400 for missing account_id got %d", w.Code)
	}

	// GetHoldBytransferRequest -> missing transfer_id -> 404
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/holds", bytes.NewBufferString(`{}`))
	api.GetHoldBytransferRequest(w, req)
	if w.Code != 404 {
		t.Fatalf("GetHoldBytransferRequest: expected 404 for missing transfer_id got %d", w.Code)
	}

	// HandleError -> should return 400
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/v1/error", nil)
	routes.HandleError(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleError: expected 400 got %d", w.Code)
	}
}
