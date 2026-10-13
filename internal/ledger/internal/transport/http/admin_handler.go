package transporthttp

import (
	"net/http"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/pkg/httpx"
)

type AdminHandler struct {
	svc *application.Service
}

func NewAdminHandler(svc *application.Service) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/balances/rebuild", h.RebuildBalances)
	mux.HandleFunc("GET /admin/balances/verify", h.VerifyBalances)
	mux.HandleFunc("GET /admin/reconciliation", h.Reconcile)
}

func (h *AdminHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	rep, err := h.svc.Reconcile(r.Context(), time.Now())
	httpx.OK(w, toReconciliationResponse(rep), err)
}

func (h *AdminHandler) RebuildBalances(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RebuildBalances(r.Context())
	httpx.NoContent(w, err)
}

func (h *AdminHandler) VerifyBalances(w http.ResponseWriter, r *http.Request) {
	ms, err := h.svc.VerifyBalances(r.Context())
	httpx.OK(w, toVerifyBalancesResponse(ms), err)
}
