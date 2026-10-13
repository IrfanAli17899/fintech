package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

const (
	historyDefaultLimit = 50
	historyMaxLimit     = 100
)

type History struct {
	Entries    []domain.AccountEntry
	NextBefore int64 // 0 when there is no further page
}

// History returns an account's statement newest first, cursor-paginated on entry seq.
func (s *Service) History(ctx context.Context, accountID uuid.UUID, beforeSeq int64, limit int) (*History, error) {
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
