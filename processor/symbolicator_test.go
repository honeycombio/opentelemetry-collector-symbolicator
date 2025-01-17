package symbolicatorprocessor

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestSymbolicator(t *testing.T) {
	ctx := context.Background()

	fs := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	sym := newBasicSymbolicator(fs)

	sf, err := sym.symbolicate(ctx, 0, 34, "b", "basic-mapping.js")
	line := formatStackFrame(sf)

	assert.NoError(t, err)
	assert.Equal(t, "    at bar(basic-mapping-original.js:8:1)", line)

	_, err = sym.symbolicate(ctx, 0, 34, "b", "does-not-exist.js")
	assert.Error(t, err)

	_, err = sym.symbolicate(ctx, math.MaxInt64, 34, "b", "basic-mapping.js")
	assert.Error(t, err)

	_, err = sym.symbolicate(ctx, 0, math.MaxInt64, "b", "basic-mapping.js")
	assert.Error(t, err)
}
