// Package s3source lists S3 buckets/objects and downloads Terraform state
// files. Credentials and region come from the standard AWS environment
// (env vars, shared config, IMDS); AWS_ENDPOINT_URL_S3 can point at an
// S3-compatible emulator.
package s3source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Bucket struct {
	Name string `json:"name"`
}

type Object struct {
	Key      string `json:"key"`
	IsDir    bool   `json:"is_dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

type ListResult struct {
	Objects   []Object `json:"objects"`
	NextToken string   `json:"next_token"`
}

type Client interface {
	Buckets(ctx context.Context) ([]Bucket, error)
	List(ctx context.Context, bucket, prefix, token string) (ListResult, error)
	Get(ctx context.Context, bucket, key string) ([]byte, error)
}

// ErrNotConfigured means no usable AWS credentials were found. The API turns
// this into a helpful message instead of a generic 500.
var ErrNotConfigured = errors.New("AWS credentials are not configured")

func Region() string {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	return os.Getenv("AWS_DEFAULT_REGION")
}

func Endpoint() string {
	if e := os.Getenv("AWS_ENDPOINT_URL_S3"); e != "" {
		return e
	}
	return os.Getenv("AWS_ENDPOINT_URL")
}

type s3Client struct {
	s3 *s3.Client
}

// New builds a client from the ambient AWS config and verifies credentials
// with a short timeout so a laptop without AWS setup fails fast and clearly.
func New(ctx context.Context) (Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if r := Region(); r != "" {
		opts = append(opts, awsconfig.WithRegion(r))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	if Region() == "" && Endpoint() == "" {
		return nil, fmt.Errorf("%w: set AWS_REGION (or AWS_ENDPOINT_URL_S3)", ErrNotConfigured)
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := cfg.Credentials.Retrieve(probe); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	var s3opts []func(*s3.Options)
	if ep := Endpoint(); ep != "" {
		s3opts = append(s3opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(ep)
			o.UsePathStyle = true
		})
	}
	return &s3Client{s3: s3.NewFromConfig(cfg, s3opts...)}, nil
}

func (c *s3Client) Buckets(ctx context.Context) ([]Bucket, error) {
	out, err := c.s3.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}
	buckets := make([]Bucket, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		if b.Name != nil {
			buckets = append(buckets, Bucket{Name: *b.Name})
		}
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Name < buckets[j].Name })
	return buckets, nil
}

func (c *s3Client) List(ctx context.Context, bucket, prefix, token string) (ListResult, error) {
	in := &s3.ListObjectsV2Input{Bucket: aws.String(bucket), MaxKeys: aws.Int32(500)}
	if prefix != "" {
		in.Prefix = aws.String(prefix)
	}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	}
	in.Delimiter = aws.String("/")
	out, err := c.s3.ListObjectsV2(ctx, in)
	if err != nil {
		return ListResult{}, err
	}
	res := ListResult{Objects: []Object{}}
	for _, p := range out.CommonPrefixes {
		if p.Prefix != nil {
			res.Objects = append(res.Objects, Object{Key: *p.Prefix, IsDir: true})
		}
	}
	for _, o := range out.Contents {
		if o.Key == nil {
			continue
		}
		if strings.HasSuffix(*o.Key, "/") {
			res.Objects = append(res.Objects, Object{Key: *o.Key, IsDir: true})
			continue
		}
		obj := Object{Key: *o.Key, Size: aws.ToInt64(o.Size)}
		if o.LastModified != nil {
			obj.Modified = o.LastModified.UTC().Format(time.RFC3339)
		}
		res.Objects = append(res.Objects, obj)
	}
	if out.NextContinuationToken != nil {
		res.NextToken = *out.NextContinuationToken
	}
	return res, nil
}

func (c *s3Client) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(out.Body, 64<<20)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// StateName derives a state name from an S3 key, matching the CLI's rules.
func StateName(key string) string {
	base := key[strings.LastIndex(key, "/")+1:]
	if base == "terraform.tfstate" {
		parts := strings.Split(strings.TrimSuffix(key, "/"+base), "/")
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" {
				return parts[i]
			}
		}
		return "terraform"
	}
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".tfstate"), ".json")
	if base == "" {
		return strings.Trim(key, "/")
	}
	return base
}
