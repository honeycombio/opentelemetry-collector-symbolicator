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

func TestGetSourceMap(t *testing.T) {
	ctx := context.Background()
	s3_key := ""

	s3Store := &s3Store{
		logger: zaptest.NewLogger(t),
		client: mockGetObjectAPI(func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			s3_key = *params.Key

			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("//# sourceMappingURL=basic-mapping.js.map")),
			}, nil
		}),
		bucket: "test_assets",
		prefix: "test-bucket-prefix",
	}

	source, _, err := s3Store.GetSourceMap(ctx, "basic-mapping.js?hash=123")

	assert.NoError(t, err)
	assert.NotEmpty(t, source)
	assert.Equal(t, "test-bucket-prefix/basic-mapping.js?hash=123", s3_key)
}