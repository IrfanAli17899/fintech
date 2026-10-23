package application

import (
	"context"
	"log"
	"time"
)

// ReconcileJob runs Reconcile and logs what it found; wrap it in a scheduler.Loop.
func (s *Service) ReconcileJob(maxLogged int) func(context.Context) {
	return func(ctx context.Context) {
		rep, err := s.Reconcile(ctx, time.Now())
		if err != nil {
			log.Printf("reconcile: %v", err)
			return
		}
		log.Printf("reconcile: %d findings over %d attempts", len(rep.Findings), rep.AttemptsChecked)
		for i, f := range rep.Findings {
			if i == maxLogged {
				log.Printf("reconcile: %d more not logged; GET /admin/reconciliation", len(rep.Findings)-maxLogged)
				break
			}
			log.Printf("reconcile: %s %s %s/%s txn=%s: %s", f.Kind, f.Reason, f.Rail, f.Ref, f.TransactionID, f.Detail)
		}
	}
}
