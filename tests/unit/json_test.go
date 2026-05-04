package routes

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
)

func TestRespondeWithJson_Success(t *testing.T) {
	w := httptest.NewRecorder()
	routes.RespondWithJson(w, 201, map[string]string{"ok": "true"})
	if w.Code != 201 {
		t.Fatalf("expected status 201 got %d", w.Code)
	}
	if c := w.Header().Get("Content-Type"); c != "application/json" {
		t.Fatalf("expected content-type application/json got %s", c)
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if out["ok"] != "true" {
		t.Fatalf("unexpected response body: %v", out)
	}
}

func TestRespondeWithJson_MarshalFailure(t *testing.T) {
	w := httptest.NewRecorder()
	ch := make(chan int)
	routes.RespondWithJson(w, 200, ch)
	if w.Code != 500 {
		t.Fatalf("expected status 500 for marshal failure got %d", w.Code)
	}
}

func TestRespondWithError_FormatsError(t *testing.T) {
	w := httptest.NewRecorder()
	routes.RespondWithError(w, 400, "oops")
	if w.Code != 400 {
		t.Fatalf("expected 400 got %d", w.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if out["error"] != "oops" {
		t.Fatalf("unexpected error body: %v", out)
	}
}
