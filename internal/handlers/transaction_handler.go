package handlers

import (
	"net/http"

	"github.com/irfanali17899/fintech/internal/services"
	"github.com/irfanali17899/fintech/pkg/httpx"
)

type TransactionHandler struct {
	ledgerSvc *services.LedgerService
}

func NewTransactionHandler(ledgerSvc *services.LedgerService) *TransactionHandler {
	return &TransactionHandler{ledgerSvc}
}

func (h *TransactionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /transactions/{id}", h.GetTransaction)
	mux.HandleFunc("POST /transactions", h.CreateTransaction)
}

func (h *TransactionHandler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	txn, err := h.ledgerSvc.GetTransaction(r.Context(), id)
	httpx.OK(w, toTransactionResponse(txn), err)
}

func (h *TransactionHandler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.Decode[createTransactionRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	tx, err := h.ledgerSvc.Post(r.Context(), req.toDomain())
	httpx.Created(w, toTransactionResponse(tx), err)
}
