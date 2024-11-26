package symbolicatorprocessor

import (
	"context"
	"fmt"
	"math"

	"github.com/honeycombio/symbolic-go"
)

type sourceMapStore interface {
	GetSourceMap(ctx context.Context, url string) (string, string, error)
}

type basicSymbolicator struct {
	store sourceMapStore
}

func newBasicSymbolicator(store sourceMapStore) *basicSymbolicator {
	return &basicSymbolicator{store: store}
}

type MappedStackFrame struct {
	FunctionName string
	Src          string
	Line		 int64
	Col 		 int64
}

// symbolicate takes a line, column, function name, and URL and returns a string
func (ns *basicSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url string) (string, MappedStackFrame, error) {
	if column < 0 || column > math.MaxUint32 {
		return "", MappedStackFrame{}, fmt.Errorf("column must be uint32: %d", column)
	}

	if line < 0 || line > math.MaxUint32 {
		return "", MappedStackFrame{}, fmt.Errorf("line must be uint32: %d", line)
	}

	// TODO: we should look to see if we have already made a SourceMapCache for this URL
	source, sMap, err := ns.store.GetSourceMap(ctx, url)

	if err != nil {
		return "", MappedStackFrame{}, err
	}

	// Create a new source map cache
	// TODO: we should cache this but they are not thread safe
	// so we need to lock around them
	// TODO: we should also have a way to evict old source maps
	smc, err := symbolic.NewSourceMapCache(source, sMap)

	if err != nil {
		return "", MappedStackFrame{}, err
	}

	t, err := smc.Lookup(uint32(line), uint32(column), 0)

	if err != nil {
		return "", MappedStackFrame{}, err
	}

	return fmt.Sprintf("at %s(%s:%d:%d)", t.FunctionName, t.Src, t.Line, t.Col), MappedStackFrame{
			FunctionName: t.FunctionName,
			Src: t.Src,
			Line: int64(t.Line),
			Col: int64(t.Col),
		}, nil
}
