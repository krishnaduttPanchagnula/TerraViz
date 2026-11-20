package remote

import (
	"context"
	"fmt"
	"io"
	"os"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

func gcpProject() string {
	if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
		return p
	}
	return os.Getenv("GCLOUD_PROJECT")
}

type gcpClient struct {
	client  *storage.Client
	project string
}

func newGCP(ctx context.Context) (Client, error) {
	project := gcpProject()
	if project == "" {
		return nil, fmt.Errorf("%w: set GOOGLE_CLOUD_PROJECT (or GCLOUD_PROJECT)", ErrNotConfigured)
	}
	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	return &gcpClient{client: c, project: project}, nil
}

func (g *gcpClient) Buckets(ctx context.Context) ([]Bucket, error) {
	out := []Bucket{}
	it := g.client.Buckets(ctx, g.project)
	for {
		b, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, Bucket{Name: b.Name})
	}
	return out, nil
}

func (g *gcpClient) List(ctx context.Context, bucket, prefix, _ string) (ListResult, error) {
	res := ListResult{Objects: []Object{}}
	q := &storage.Query{Prefix: prefix, Delimiter: "/"}
	it := g.client.Bucket(bucket).Objects(ctx, q)
	for {
		a, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return res, err
		}
		if a.Prefix != "" {
			res.Objects = append(res.Objects, Object{Key: a.Prefix, IsDir: true})
			continue
		}
		res.Objects = append(res.Objects, Object{
			Key: a.Name, Size: a.Size, Modified: a.Updated.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return res, nil
}

func (g *gcpClient) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	r, err := g.client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 64<<20))
}
