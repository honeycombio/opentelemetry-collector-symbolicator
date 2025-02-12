package symbolicatorprocessor

import (
	"context"
	"fmt"
	"io"
	neturl "net/url"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type s3Store struct {
	logger *zap.Logger
	client *s3.Client
	bucket string
	prefix string
}

func (s *s3Store) GetSourceMap(ctx context.Context, url string) ([]byte, []byte, error) {
	u, err := neturl.Parse(url)

	if err != nil {
		return nil, nil, err
	}

	path := filepath.Join(s.prefix, u.Path)

	source, err := s.loadContent(ctx, path)

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

	sourceMap, err := s.loadContent(ctx, path)

	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", errFailedToFindSourceMap, path)
	}

	return source, sourceMap, nil
}

func (s *s3Store) loadContent(ctx context.Context, key string) ([]byte, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, err
	}

	defer result.Body.Close()

	return io.ReadAll(result.Body)
}

func newS3Store(ctx context.Context, logger *zap.Logger, cfg *S3SourceMapConfiguration) (*s3Store, error) {
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

	return &s3Store{
		logger: logger,
		client: client,
		prefix: cfg.Prefix,
		bucket: cfg.BucketName,
	}, nil
}
