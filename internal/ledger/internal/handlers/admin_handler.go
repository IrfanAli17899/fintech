package handlers

import (
	"net/http"

	"github.com/irfanali17899/fintech/internal/ledger/internal/services"
	"github.com/irfanali17899/fintech/pkg/httpx"
)

type AdminHandler struct {
	adminSvc *services.AdminService
}

func NewAdminHandler(adminSvc *services.AdminService) *AdminHandler {
	return &AdminHandler{adminSvc: adminSvc}
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/balances/rebuild", h.RebuildBalances)
	mux.HandleFunc("GET /admin/balances/verify", h.VerifyBalances)
}

func (h *AdminHandler) RebuildBalances(w http.ResponseWriter, r *http.Request) {
	err := h.adminSvc.RebuildBalances(r.Context())
	httpx.NoContent(w, err)
}

func (h *AdminHandler) VerifyBalances(w http.ResponseWriter, r *http.Request) {
	ms, err := h.adminSvc.VerifyBalances(r.Context())
	httpx.OK(w, toVerifyBalancesResponse(ms), err)
}
