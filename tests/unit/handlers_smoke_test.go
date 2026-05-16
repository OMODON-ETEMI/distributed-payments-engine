package routes

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/routes"
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
	req = httptest.NewRequest("POST", "/v1/deposit", bytes.NewBufferString(`{}`))
	api.HandleDeposit(w, req)
	if w.Code != 400 {
		t.Fatalf("HandleDeposit: expected 400 for missing fields got %d", w.Code)
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
}
