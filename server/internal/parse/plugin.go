package parse

import (
	"context"

	"github.com/shreemangalam/stratum/server/internal/core"
)

// Parser is the interface that language plugins implement.
type Parser interface {
	Parse(ctx context.Context, source []byte) (*core.Tree, error)
	Language() string
	Extensions() []string
}
