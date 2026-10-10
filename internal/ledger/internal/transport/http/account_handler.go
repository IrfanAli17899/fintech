package transporthttp

import (
	"net/http"

	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/pkg/httpx"
)

type AccountHandler struct {
	accSvc *application.AccountService
}

func NewAccountHandler(accSvc *application.AccountService) *AccountHandler {
	return &AccountHandler{accSvc}
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
	acc, err := h.accSvc.ListByOwner(r.Context(), domain.OwnerType(q.OwnerType), q.OwnerID)
	httpx.OK(w, toAccountListResponse(acc), err)
}

func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	acc, err := h.accSvc.GetAccount(r.Context(), id)
	httpx.OK(w, toAccountResponse(acc), err)
}

func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.Decode[createAccountRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	acc, err := h.accSvc.CreateAccount(r.Context(), req.toDomain())
	httpx.Created(w, toAccountResponse(acc), err)
}

func (h *AccountHandler) GetAccountBalance(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	balance, err := h.accSvc.GetAccountBalance(r.Context(), id)
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
	hist, err := h.accSvc.History(r.Context(), id, q.Before, q.Limit)
	httpx.OK(w, toHistoryResponse(hist), err)
}
