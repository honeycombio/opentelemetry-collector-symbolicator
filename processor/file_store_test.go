package symbolicatorprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestFileStore(t *testing.T) {
	ctx := context.Background()

	fs := newFileStore("../test_assets", zaptest.NewLogger(t))

	source, sMap, err := fs.GetSourceMap(ctx, "basic-mapping.js")

	assert.NoError(t, err)
	assert.NotEmpty(t, source)
	assert.NotEmpty(t, sMap)

	source, sMap, err = fs.GetSourceMap(ctx, "does-not-exist.js")
	assert.ErrorIs(t, err, errFailedToFindSourceFile)
}
