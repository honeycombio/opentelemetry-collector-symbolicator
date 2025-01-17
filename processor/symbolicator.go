package symbolicatorprocessor

import (
	"context"
	"fmt"
	"math"

	"github.com/honeycombio/symbolic-go"
)

type sourceMapStore interface {
	GetSourceMap(ctx context.Context, url string) ([]byte, []byte, error)
}

type basicSymbolicator struct {
	store sourceMapStore
}

func newBasicSymbolicator(store sourceMapStore) *basicSymbolicator {
	return &basicSymbolicator{store: store}
}

type mappedStackFrame struct {
	FunctionName string
	URL          string
	Line         int64
	Col          int64
}

// symbolicate takes a line, column, function name, and URL and returns a string
func (ns *basicSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url string) (*mappedStackFrame, error) {
	if column < 0 || column > math.MaxUint32 {
		return &mappedStackFrame{}, fmt.Errorf("column must be uint32: %d", column)
	}

	if line < 0 || line > math.MaxUint32 {
		return &mappedStackFrame{}, fmt.Errorf("line must be uint32: %d", line)
	}

	// TODO: we should look to see if we have already made a SourceMapCache for this URL
	source, sMap, err := ns.store.GetSourceMap(ctx, url)

	if err != nil {
		return &mappedStackFrame{}, err
	}

	// Create a new source map cache
	// TODO: we should cache this but they are not thread safe
	// so we need to lock around them
	// TODO: we should also have a way to evict old source maps
	smc, err := symbolic.NewSourceMapCache(string(source), string(sMap))

	if err != nil {
		return &mappedStackFrame{}, err
	}

	t, err := smc.Lookup(uint32(line), uint32(column), 0)

	if err != nil {
		return &mappedStackFrame{}, err
	}

	return &mappedStackFrame{
		FunctionName: t.FunctionName,
		URL:          t.Src,
		Line:         int64(t.Line),
		Col:          int64(t.Col),
	}, nil
}
