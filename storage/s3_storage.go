package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3lib "github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	Client     *s3lib.Client
	BucketName string
	Prefix     string
}

func NewS3Storage(bucket string, prefix string) (*S3Storage, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	client := s3lib.NewFromConfig(cfg)

	return &S3Storage{
		Client:     client,
		BucketName: bucket,
		Prefix:     prefix,
	}, nil
}

func (s *S3Storage) SaveFile(name string, data io.Reader) error {
	key := strings.TrimPrefix(s.Prefix+"/"+name, "/")

	uploader := manager.NewUploader(s.Client, func(u *manager.Uploader) {
		u.PartSize = 8 * 1024 * 1024
		u.Concurrency = 3
	})

	_, err := uploader.Upload(context.TODO(), &s3lib.PutObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(key),
		Body:   data,
	})

	return err
}
