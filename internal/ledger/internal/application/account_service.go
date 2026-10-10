package application

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/pkg/database"
)

const (
	historyDefaultLimit = 50
	historyMaxLimit     = 100
)

type History struct {
	Entries    []domain.AccountEntry
	NextBefore int64 // 0 when there is no further page
}

type AccountService struct {
	txRunner    database.TxRunner
	accRepo     *persistence.AccountRepo
	balanceRepo *persistence.BalanceRepo
	entryRepo   *persistence.EntryRepo
}

func NewAccountService(
	txRunner database.TxRunner,
	accRepo *persistence.AccountRepo,
	balanceRepo *persistence.BalanceRepo,
	entryRepo *persistence.EntryRepo,
) *AccountService {
	return &AccountService{txRunner, accRepo, balanceRepo, entryRepo}
}

func (s *AccountService) CreateAccount(ctx context.Context, a domain.Account) (*domain.Account, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}

	var created *domain.Account

	// create account and its zero balance row
	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		accounts := s.accRepo.WithTx(tx)
		balances := s.balanceRepo.WithTx(tx)

		acc, err := accounts.Create(ctx, a)
		if err != nil {
			return err
		}
		created = acc
		return balances.Apply(ctx, created.ID, domain.AccountSums{})
	})

	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *AccountService) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	return s.accRepo.GetByID(ctx, id)
}

func (s *AccountService) GetAccountBalance(ctx context.Context, id uuid.UUID) (*domain.Balance, error) {
	acc, err := s.accRepo.GetByID(ctx, id)

	if err != nil {
		return nil, err
	}

	sums, err := s.balanceRepo.GetBalance(ctx, id)

	if err != nil {
		return nil, err
	}

	b := acc.Type.Balance(*sums)
	b.AccountID, b.Currency, b.AsOf = id, acc.Currency, time.Now()

	return &b, nil

}

func (s *AccountService) ListByOwner(ctx context.Context, ownerType domain.OwnerType, ownerID string) ([]domain.Account, error) {
	if !ownerType.Valid() {
		return nil, domain.ErrInvalidOwnerType(ownerType)
	}
	return s.accRepo.ListByOwner(ctx, ownerType, ownerID)
}

func (s *AccountService) SeedSystem(ctx context.Context) error {
	return s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		accounts := s.accRepo.WithTx(tx)
		balances := s.balanceRepo.WithTx(tx)

		ids, err := accounts.SeedAccounts(ctx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := balances.Apply(ctx, id, domain.AccountSums{}); err != nil {
				return err
			}
		}
		return nil
	})
}

// History returns an account's statement newest first, cursor-paginated on entry seq.
func (s *AccountService) History(ctx context.Context, accountID uuid.UUID, beforeSeq int64, limit int) (*History, error) {
	if _, err := s.accRepo.GetByID(ctx, accountID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = historyDefaultLimit
	}
	if limit > historyMaxLimit {
		limit = historyMaxLimit
	}
	entries, err := s.entryRepo.ListByAccount(ctx, accountID, beforeSeq, limit)
	if err != nil {
		return nil, err
	}
	h := &History{Entries: entries}
	if len(entries) == limit {
		h.NextBefore = entries[len(entries)-1].Seq
	}
	return h, nil
}
