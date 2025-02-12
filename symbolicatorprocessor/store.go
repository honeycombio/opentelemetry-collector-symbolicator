package symbolicatorprocessor

import (
	"context"
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

var (
	mappingURLRegex                  = regexp.MustCompile(`\/\/[#@]\s(sourceMappingURL)=\s*(\S+)`)
	errFailedToFindSourceFile        = fmt.Errorf("failed to find source file")
	errFailedToFindSourceMapLocation = fmt.Errorf("failed to find source map location")
	errFailedToFindSourceMap         = fmt.Errorf("failed to find source map")
)

type store struct {
	fetch  func(ctx context.Context, key string) ([]byte, error)
	logger *zap.Logger
	prefix string
}

func (s *store) GetSourceMap(ctx context.Context, url string) ([]byte, []byte, error) {
	u, err := neturl.Parse(url)

	if err != nil {
		return nil, nil, err
	}

	path := filepath.Join(s.prefix, u.Path)

	source, err := s.fetch(ctx, path)

	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", errFailedToFindSourceFile, path)
	}

	matches := mappingURLRegex.FindSubmatch(source)

	if len(matches) <= 0 {
		return nil, nil, fmt.Errorf("%w: %s", errFailedToFindSourceMapLocation, path)
	}

	// the capture group we want is the last one
	mapName := string(matches[len(matches)-1])

	// the map name is relative to the source file
	path = filepath.Join(filepath.Dir(path), mapName)

	sourceMap, err := s.fetch(ctx, path)

	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", errFailedToFindSourceMap, path)
	}

	return source, sourceMap, nil

}

func newFileStore(_ context.Context, logger *zap.Logger, cfg *LocalSourceMapConfiguration) (*store, error) {
	return &store{
		fetch: func(ctx context.Context, key string) ([]byte, error) {
			return os.ReadFile(key)
		},
		logger: logger,
		prefix: cfg.Path,
	}, nil
}

func newS3Store(ctx context.Context, logger *zap.Logger, cfg *S3SourceMapConfiguration) (*store, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no S3 configuration provided")
	}

	options := make([]func(*config.LoadOptions) error, 0)

	if cfg.Region != "" {
		options = append(options, config.WithRegion(cfg.Region))
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, options...)

	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsConfig)

	return &store{
		fetch: func(ctx context.Context, key string) ([]byte, error) {
			result, err := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(cfg.BucketName),
				Key:    aws.String(key),
			})

			if err != nil {
				return nil, err
			}

			defer result.Body.Close()

			return io.ReadAll(result.Body)
		},
		logger: logger,
		prefix: cfg.Prefix,
	}, nil
}
