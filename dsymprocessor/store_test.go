package dsymprocessor

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

	source, err := fs.GetDSYM(ctx, "6A8CB813-45F6-3652-AD33-778FD1EAB196", "Chateaux Bufeaux")

	assert.NoError(t, err)
	assert.NotEmpty(t, source)

	_, err = fs.GetDSYM(ctx, "6A8CB813-45F6-3652-AD33-778FD1EAB196", "Not A Binary")
	assert.ErrorIs(t, err, errFailedToFindDSYM)
}
