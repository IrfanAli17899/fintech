package transporthttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/api"
	"github.com/irfanali17899/fintech/ledger/codes"
	transporthttp "github.com/irfanali17899/fintech/ledger/internal/transport/http"
	"github.com/irfanali17899/fintech/platform/errdefs"
)

// fakeLedger answers with whatever the test set and records the last command.
// Methods a test does not set panic through the nil embedded interface.
type fakeLedger struct {
	api.Ledger
	result   api.Result
	err      error
	capture  api.Capture
	accounts []api.Account
}

func (f *fakeLedger) Post(_ context.Context, _ api.PostTransaction) (api.Result, error) {
	return f.result, f.err
}

func (f *fakeLedger) Capture(_ context.Context, cmd api.Capture) (api.Result, error) {
	f.capture = cmd
	return f.result, f.err
}

func (f *fakeLedger) Void(_ context.Context, _ api.Void) (api.Result, error) { return f.result, f.err }

func (f *fakeLedger) GetTransaction(_ context.Context, _ uuid.UUID) (api.Transaction, error) {
	return f.result.Transaction, f.err
}

func (f *fakeLedger) ListAccounts(_ context.Context, _ api.AccountQuery) ([]api.Account, error) {
	return f.accounts, f.err
}

type fakeAdmin struct {
	api.LedgerAdmin
	report api.ReconciliationReport
}

func (f *fakeAdmin) Reconcile(context.Context) (api.ReconciliationReport, error) {
	return f.report, nil
}

func serve(l api.Ledger, a api.LedgerAdmin) *http.ServeMux {
	mux := http.NewServeMux()
	transporthttp.NewTransactionHandler(l).RegisterRoutes(mux)
	transporthttp.NewAccountHandler(l).RegisterRoutes(mux)
	transporthttp.NewAdminHandler(a).RegisterRoutes(mux)
	return mux
}

func do(t *testing.T, mux *http.ServeMux, method, path, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: response is not a JSON object: %s", method, path, rec.Body.String())
		}
	}
	return rec.Code, out
}

const postBody = `{"kind":"topup","idempotency_key":"k1","entries":[]}`

func TestPost_NewIs201ReplayIs200(t *testing.T) {
	f := &fakeLedger{result: api.Result{Transaction: api.Transaction{
		ID: uuid.New(), Status: api.StatusPending, Rail: "card",
		Summary: api.Summary{Total: 5000, Remaining: 5000, Settlement: api.StatusPending},
	}}}
	mux := serve(f, &fakeAdmin{})

	code, body := do(t, mux, "POST", "/transactions", postBody)
	if code != http.StatusCreated {
		t.Fatalf("new transaction: status %d, want 201", code)
	}
	if body["status"] != "pending" || body["summary"].(map[string]any)["total"] != float64(5000) {
		t.Fatalf("response shape: %v", body)
	}

	f.result.Replayed = true
	if code, _ := do(t, mux, "POST", "/transactions", postBody); code != http.StatusOK {
		t.Fatalf("replay: status %d, want 200", code)
	}
}

func TestErrors_MapToStatusAndCode(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{errdefs.Validationf(codes.IdempotencyKeyRequired, "idempotency key is required"), http.StatusBadRequest, codes.IdempotencyKeyRequired},
		{errdefs.Validationf(codes.InsufficientFunds, "not enough"), http.StatusBadRequest, codes.InsufficientFunds},
		{errdefs.Conflictf(codes.HoldAlreadySettling, "settled"), http.StatusConflict, codes.HoldAlreadySettling},
		{errdefs.NotFoundf(codes.TransactionNotFound, "no such transaction"), http.StatusNotFound, codes.TransactionNotFound},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			mux := serve(&fakeLedger{err: c.err}, &fakeAdmin{})
			code, body := do(t, mux, "POST", "/transactions", postBody)
			if code != c.status || body["code"] != c.code {
				t.Fatalf("status %d code %v, want %d %s", code, body["code"], c.status, c.code)
			}
		})
	}
}

func TestCapture_PassesAmountAndFinalThrough(t *testing.T) {
	f := &fakeLedger{result: api.Result{Transaction: api.Transaction{ID: uuid.New(), Status: api.StatusPosted}}}
	hold := uuid.New()
	code, _ := do(t, serve(f, &fakeAdmin{}), "POST", "/transactions/"+hold.String()+"/capture",
		`{"amount":300,"final":true,"idempotency_key":"k2"}`)
	if code != http.StatusCreated {
		t.Fatalf("status %d, want 201", code)
	}
	if f.capture != (api.Capture{IdempotencyKey: "k2", HoldID: hold, Amount: 300, Final: true}) {
		t.Fatalf("command reaching the ledger: %+v", f.capture)
	}
}

func TestRequestErrors_NeverReachTheLedger(t *testing.T) {
	mux := serve(&fakeLedger{}, &fakeAdmin{}) // any call into the ledger would panic
	for _, c := range []struct{ method, path, body, code string }{
		{"POST", "/transactions", `{"kind":`, "INVALID_JSON"},
		{"GET", "/transactions/not-a-uuid", "", ""},
		{"GET", "/accounts", "", "MISSING_QUERY_PARAM"},
	} {
		code, body := do(t, mux, c.method, c.path, c.body)
		if code != http.StatusBadRequest || (c.code != "" && body["code"] != c.code) {
			t.Fatalf("%s %s: status %d code %v, want 400 %s", c.method, c.path, code, body["code"], c.code)
		}
	}
}

func TestReconciliation_ReportShape(t *testing.T) {
	a := &fakeAdmin{report: api.ReconciliationReport{
		AttemptsChecked: 3,
		Findings: []api.Finding{
			{Kind: "divergence", Reason: "voided_but_live"},
			{Kind: "divergence", Reason: "voided_but_live"},
			{Kind: "orphan_attempt"},
		},
		TrialBalance: []api.TrialBalanceLine{{Currency: "PKR", AccountType: "asset", Sums: api.Sums{Debits: 700}}},
	}}
	code, body := do(t, serve(&fakeLedger{}, a), "GET", "/admin/reconciliation", "")
	if code != http.StatusOK || body["clean"] != false {
		t.Fatalf("status %d clean %v", code, body["clean"])
	}
	counts := body["counts"].(map[string]any)
	if counts["divergence:voided_but_live"] != float64(2) || counts["orphan_attempt"] != float64(1) {
		t.Fatalf("counts: %v", counts)
	}
	line := body["trial_balance"].([]any)[0].(map[string]any)
	if line["currency"] != "PKR" || line["account_type"] != "asset" || line["debits"] != float64(700) {
		t.Fatalf("trial balance line: %v", line)
	}
}
