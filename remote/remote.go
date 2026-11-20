// Package remote provides a provider-agnostic browser for Terraform remote
// state stored in cloud object storage: AWS S3, Google Cloud Storage, Azure
// Blob Storage and OCI Object Storage.
//
// Credentials always come from each provider's standard chain. Nothing is
// cached in this package beyond the constructed clients.
package remote

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"terraviz/s3source"
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

var ErrNotConfigured = errors.New("remote state credentials are not configured")

func Providers() []string { return []string{"aws", "gcp", "azure", "oci"} }

func Normalize(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "" {
		return "aws"
	}
	switch p {
	case "s3", "amazon", "amazonaws":
		return "aws"
	case "gcs", "google", "googlecloud":
		return "gcp"
	case "azurerm", "azureblob", "microsoft":
		return "azure"
	case "oracle", "objectstorage":
		return "oci"
	}
	return p
}

func New(ctx context.Context, provider string) (Client, error) {
	switch Normalize(provider) {
	case "aws":
		c, err := s3source.New(ctx)
		if err != nil {
			if errors.Is(err, s3source.ErrNotConfigured) {
				return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
			}
			return nil, err
		}
		return &awsClient{c}, nil
	case "gcp":
		return newGCP(ctx)
	case "azure":
		return newAzure(ctx)
	case "oci":
		return newOCI(ctx)
	default:
		return nil, fmt.Errorf("unsupported remote state provider %q (supported: %s)",
			provider, strings.Join(Providers(), ", "))
	}
}

type awsClient struct{ c s3source.Client }

// FromS3 adapts an existing S3 client (useful for tests and custom setups).
func FromS3(c s3source.Client) Client { return &awsClient{c} }

func (a *awsClient) Buckets(ctx context.Context) ([]Bucket, error) {
	bs, err := a.c.Buckets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Bucket, 0, len(bs))
	for _, b := range bs {
		out = append(out, Bucket{Name: b.Name})
	}
	return out, nil
}

func (a *awsClient) List(ctx context.Context, bucket, prefix, token string) (ListResult, error) {
	r, err := a.c.List(ctx, bucket, prefix, token)
	if err != nil {
		return ListResult{}, err
	}
	objects := make([]Object, 0, len(r.Objects))
	for _, o := range r.Objects {
		objects = append(objects, Object{Key: o.Key, IsDir: o.IsDir, Size: o.Size, Modified: o.Modified})
	}
	return ListResult{Objects: objects, NextToken: r.NextToken}, nil
}

func (a *awsClient) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	return a.c.Get(ctx, bucket, key)
}

// StateName derives a state name from an object key, following the CLI rules.
func StateName(key string) string { return s3source.StateName(key) }

// ConfigStatus describes how a provider is configured, for the UI status pill.
func ConfigStatus(provider string) map[string]any {
	m := map[string]any{"configured": false, "provider": Normalize(provider)}
	switch Normalize(provider) {
	case "aws":
		m["region"] = s3source.Region()
		if ep := s3source.Endpoint(); ep != "" {
			m["endpoint"] = ep
		}
	case "gcp":
		m["project"] = gcpProject()
	case "azure":
		m["account"] = azureAccount()
	case "oci":
		m["compartment"] = ociCompartment()
	}
	return m
}

// MarkNotConfigured annotates a missing-credentials error with provider context.
func MarkNotConfigured(provider string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotConfigured) {
		return fmt.Errorf("%w: %s credentials not found (provider %s)", ErrNotConfigured, provider, provider)
	}
	return err
}
