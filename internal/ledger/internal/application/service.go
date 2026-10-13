package application

import (
	"github.com/irfanali17899/fintech/internal/ledger/internal/application/port"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
)

// Service is the ledger's application layer: every use case is a method on it,
// one file per use case. It only holds dependencies; what callers may reach is
// decided by the api interfaces in front of it, not by splitting this type.
type Service struct {
	uow    port.UnitOfWork
	stores persistence.Stores // pool-bound, for reads outside a unit of work
	rails  ports.RailRegistry
}

func NewService(uow port.UnitOfWork, stores persistence.Stores, rails ports.RailRegistry) *Service {
	return &Service{uow: uow, stores: stores, rails: rails}
}
