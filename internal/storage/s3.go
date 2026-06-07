package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Options configures an S3-compatible backend (Railway Bucket, MinIO, …).
type S3Options struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
	// Prefix is the key namespace for outputs (default "compressed").
	Prefix string
}

// S3Storage implements Storage against an S3-compatible object store.
// Save uploads (streaming, multipart) and returns the object KEY, which is what
// gets stored in jobs.output_path. Get/Delete/PresignGetURL operate on that key.
type S3Storage struct {
	client   *s3.Client
	presign  *s3.PresignClient
	uploader *manager.Uploader
	bucket   string
	prefix   string
}

func NewS3Storage(opts S3Options) (*S3Storage, error) {
	if opts.Bucket == "" || opts.Endpoint == "" {
		return nil, errors.New("s3 storage requires a bucket and endpoint")
	}
	region := opts.Region
	if region == "" {
		region = "auto"
	}
	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(opts.Endpoint),
		UsePathStyle: opts.ForcePathStyle,
		Credentials: credentials.NewStaticCredentialsProvider(
			opts.AccessKeyID, opts.SecretAccessKey, "",
		),
	})
	prefix := strings.Trim(opts.Prefix, "/")
	if prefix == "" {
		prefix = "compressed"
	}
	return &S3Storage{
		client:   client,
		presign:  s3.NewPresignClient(client),
		uploader: manager.NewUploader(client),
		bucket:   opts.Bucket,
		prefix:   prefix,
	}, nil
}

func (s *S3Storage) key(jobID, filename string) string {
	return fmt.Sprintf("%s/%s/%s", s.prefix, jobID, filename)
}

func (s *S3Storage) Save(jobID string, filename string, reader io.Reader) (string, error) {
	key := s.key(jobID, filename)
	_, err := s.uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentTypeFor(filename)),
	})
	if err != nil {
		return "", fmt.Errorf("s3 upload failed: %w", err)
	}
	return key, nil
}

func (s *S3Storage) Get(key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Storage) Delete(key string) error {
	_, err := s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Storage) GetFullPath(key string) string {
	return key
}

func (s *S3Storage) PresignGetURL(key string, expiry time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("empty key")
	}
	req, err := s.presign.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// extContentTypes covers the compressor's output formats explicitly, since Go's
// mime table doesn't always know webp.
var extContentTypes = map[string]string{
	".webp": "image/webp",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
}

func contentTypeFor(filename string) string {
	return ContentTypeForExt(filepath.Ext(filename))
}

// ContentTypeForExt resolves a MIME type from a file extension (e.g. ".webp").
func ContentTypeForExt(ext string) string {
	ext = strings.ToLower(ext)
	if ct, ok := extContentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
