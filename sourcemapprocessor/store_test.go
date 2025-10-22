package sourcemapprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestFileStore(t *testing.T) {
	ctx := context.Background()

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)

	source, sMap, err := fs.GetSourceMap(ctx, jsFile, "")
	assert.NoError(t, err)
	assert.NotEmpty(t, source)
	assert.NotEmpty(t, sMap)

	source, sMap, err = fs.GetSourceMap(ctx, uuidFile, uuid)
	assert.NoError(t, err)
	assert.NotEmpty(t, source)
	assert.NotEmpty(t, sMap)

	source, sMap, err = fs.GetSourceMap(ctx, noFile, "")
	assert.ErrorIs(t, err, errFailedToFindSourceFile)
}
