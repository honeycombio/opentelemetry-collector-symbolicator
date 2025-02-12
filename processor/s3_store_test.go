package symbolicatorprocessor

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

type mockGetObjectAPI func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)

func (m mockGetObjectAPI) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
    return m(ctx, params, optFns...)
}

func TestS3Store(t *testing.T) {
	ctx := context.Background()

	s3Store := &s3Store{
		logger: zaptest.NewLogger(t),
		client: mockGetObjectAPI(func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("")),
			}, nil
		}),
		bucket: "test-bucket",
		prefix: "test-prefix",
	}

	source, sMap, err := s3Store.GetSourceMap(ctx, "basic-mapping.js")

	assert.NoError(t, err)
	assert.NotEmpty(t, source)
	assert.NotEmpty(t, sMap)

	source, sMap, err = s3Store.GetSourceMap(ctx, "does-not-exist.js")
	assert.ErrorIs(t, err, errFailedToFindSourceFile)
}