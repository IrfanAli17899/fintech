package transporthttp

import (
	"net/http"

	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/pkg/httpx"
)

type AccountHandler struct {
	ledger api.Ledger
}

func NewAccountHandler(ledger api.Ledger) *AccountHandler {
	return &AccountHandler{ledger}
}

func (h *AccountHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /accounts", h.CreateAccount)
	mux.HandleFunc("GET /accounts", h.ListAccounts)
	mux.HandleFunc("GET /accounts/{id}", h.GetAccount)
	mux.HandleFunc("GET /accounts/{id}/balance", h.GetAccountBalance)
	mux.HandleFunc("GET /accounts/{id}/transactions", h.History)

}

func (h *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	q, err := httpx.DecodeQuery[listAccountsQuery](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	if err := q.Validate(); err != nil {
		httpx.WriteError(w, err)
		return
	}
	acc, err := h.ledger.ListAccounts(r.Context(), api.AccountQuery{OwnerType: q.OwnerType, OwnerID: q.OwnerID})
	httpx.OK(w, toAccountListResponse(acc), err)
}

func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	acc, err := h.ledger.GetAccount(r.Context(), id)
	httpx.OK(w, toAccountResponse(acc), err)
}

func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.Decode[createAccountRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	acc, err := h.ledger.CreateAccount(r.Context(), req.toCommand())
	httpx.Created(w, toAccountResponse(acc), err)
}

func (h *AccountHandler) GetAccountBalance(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	balance, err := h.ledger.Balance(r.Context(), id)
	httpx.OK(w, toBalanceResponse(balance), err)
}

func (h *AccountHandler) History(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	q, err := httpx.DecodeQuery[historyQuery](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	hist, err := h.ledger.History(r.Context(), id, api.Page{Before: q.Before, Limit: q.Limit})
	httpx.OK(w, toHistoryResponse(hist), err)
}
