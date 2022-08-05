package transactor

import (
	"context"
)

// Transactor represents behavior for transactors
type Transactor interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}
