package handlers

import (
	"net/http"

	"github.com/irfanali17899/fintech/internal/ledger/internal/services"
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
	mux.HandleFunc("POST /transactions/{id}/reverse", h.ReverseTransaction)
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

	t, replayed, err := h.ledgerSvc.Post(r.Context(), req.toDomain())
	if replayed {
		httpx.OK(w, toTransactionResponse(t), err)
		return
	}
	httpx.Created(w, toTransactionResponse(t), err)
}

func (h *TransactionHandler) ReverseTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	req, err := httpx.Decode[reverseTransactionRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	t, replayed, err := h.ledgerSvc.Reverse(r.Context(), req.toCommand(id))
	if replayed {
		httpx.OK(w, toTransactionResponse(t), err)
		return
	}
	httpx.Created(w, toTransactionResponse(t), err)
}
