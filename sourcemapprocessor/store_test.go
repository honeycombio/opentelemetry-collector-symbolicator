package sourcemapprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestFileStore(t *testing.T) {
	ctx := context.Background()

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	require.NoError(t, err)

	source, sMap, err := fs.GetSourceMap(ctx, jsFile, "")
	require.NoError(t, err)
	assert.Contains(t, string(source), "basic-mapping.js.map")
	assert.Contains(t, string(sMap), "basic-mapping.js")

	source, sMap, err = fs.GetSourceMap(ctx, uuidFile, uuid)
	require.NoError(t, err)
	assert.Contains(t, string(source), "uuid-mapping.js.map")
	assert.Contains(t, string(sMap), "uuid-mapping.js")

	_, _, err = fs.GetSourceMap(ctx, noFile, "")
	assert.ErrorIs(t, err, errFailedToFindSourceFile)
}
