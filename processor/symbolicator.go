package symbolicatorprocessor

import (
	"context"

	_ "github.com/honeycombio/symbolic-go"
)

// noopSymbolicator is a symbolicator that does nothing.
type noopSymbolicator struct{}

// symbolicate does nothing and returns an empty string.
func (ns *noopSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url string) string {
	return ""
}
