package domain

import (
	"time"

	"github.com/google/uuid"
)

type ResourceType string

const (
	ResourceTransaction ResourceType = "transaction"
)

type IdempotencyKey struct {
	Key          string
	RequestHash  string
	ResourceType ResourceType
	ResourceID   *uuid.UUID
	CreatedAt    time.Time
}
