package symbolicatorprocessor

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestSymbolicator(t *testing.T) {
	ctx := context.Background()

	fs := newFileStore("../test_assets", zaptest.NewLogger(t))
	sym := newBasicSymbolicator(ctx, 5*time.Second, fs)

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
