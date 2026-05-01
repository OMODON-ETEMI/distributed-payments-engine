package routes

import (
	"testing"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
	"github.com/shopspring/decimal"
)

func TestStringtoPgUuid_ValidAndInvalid(t *testing.T) {
	id, err := routes.StringtoPgUuid("00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("expected no error for valid uuid, got: %v", err)
	}
	if !id.Valid {
		t.Fatalf("expected UUID to be valid")
	}

	_, err = routes.StringtoPgUuid("not-a-uuid")
	if err == nil {
		t.Fatalf("expected error for invalid uuid")
	}
}

func TestStringToNumeric_and_NumericConversions(t *testing.T) {
	n, err := routes.StringToNumeric("123.45")
	if err != nil {
		t.Fatalf("unexpected error converting numeric: %v", err)
	}
	s := routes.NumericToString(n)
	if s == "" {
		t.Fatalf("expected numeric string conversion to produce a value")
	}

	d, err := routes.NumericToDecimal(n)
	if err != nil {
		t.Fatalf("unexpected error converting to decimal: %v", err)
	}
	expected, _ := decimal.NewFromString(s)
	if !d.Equal(expected) {
		t.Fatalf("decimal mismatch: expected %s got %s", expected.String(), d.String())
	}

	if _, err := routes.StringToNumeric("not-a-number"); err == nil {
		t.Fatalf("expected error for invalid numeric string")
	}
}

func TestHashRequest_IsDeterministic(t *testing.T) {
	h1 := routes.HashRequest([]byte("hello"))
	h2 := routes.HashRequest([]byte("hello"))
	if h1 != h2 {
		t.Fatalf("expected identical hashes for same payload")
	}
}

func TestValidateLedgerBalance_BalancedAndUnbalanced(t *testing.T) {
	a, _ := routes.StringToNumeric("100")
	b, _ := routes.StringToNumeric("100")

	balanced := []routes.JournalLeg{{Amount: a, Side: "DEBIT"}, {Amount: b, Side: "CREDIT"}}
	if err := routes.ValidateLedgerBalance(balanced); err != nil {
		t.Fatalf("expected balanced legs to pass, got: %v", err)
	}

	c, _ := routes.StringToNumeric("50")
	unbalanced := []routes.JournalLeg{{Amount: a, Side: "DEBIT"}, {Amount: c, Side: "CREDIT"}}
	if err := routes.ValidateLedgerBalance(unbalanced); err == nil {
		t.Fatalf("expected unbalanced legs to return an error")
	}
}
