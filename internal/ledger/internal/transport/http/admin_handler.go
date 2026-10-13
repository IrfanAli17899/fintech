package transporthttp

import (
	"net/http"

	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/pkg/httpx"
)

type AdminHandler struct {
	admin api.LedgerAdmin
}

func NewAdminHandler(admin api.LedgerAdmin) *AdminHandler {
	return &AdminHandler{admin: admin}
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/balances/rebuild", h.RebuildBalances)
	mux.HandleFunc("GET /admin/balances/verify", h.VerifyBalances)
	mux.HandleFunc("GET /admin/reconciliation", h.Reconcile)
}

func (h *AdminHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	rep, err := h.admin.Reconcile(r.Context())
	httpx.OK(w, toReconciliationResponse(rep), err)
}

func (h *AdminHandler) RebuildBalances(w http.ResponseWriter, r *http.Request) {
	err := h.admin.RebuildBalances(r.Context())
	httpx.NoContent(w, err)
}

func (h *AdminHandler) VerifyBalances(w http.ResponseWriter, r *http.Request) {
	ms, err := h.admin.VerifyBalances(r.Context())
	httpx.OK(w, toVerifyBalancesResponse(ms), err)
}
