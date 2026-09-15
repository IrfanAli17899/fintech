package handlers

import (
	"net/http"

	"github.com/irfanali17899/fintech/internal/services"
	"github.com/irfanali17899/fintech/pkg/httpx"
)

type AccountHandler struct {
	accSvc *services.AccountService
}

func NewAccountHandler(accSvc *services.AccountService) *AccountHandler {
	return &AccountHandler{accSvc}
}

func (h *AccountHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /accounts/{id}", h.GetAccount)
	mux.HandleFunc("POST /accounts", h.CreateAccount)
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
