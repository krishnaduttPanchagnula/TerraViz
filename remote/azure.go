package remote

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

func azureAccount() string {
	if a := os.Getenv("AZURE_STORAGE_ENDPOINT"); a != "" {
		return a
	}
	return os.Getenv("AZURE_STORAGE_ACCOUNT")
}

type azureClient struct {
	client *azblob.Client
}

func newAzure(ctx context.Context) (Client, error) {
	account := azureAccount()
	if account == "" {
		return nil, fmt.Errorf("%w: set AZURE_STORAGE_ACCOUNT (or AZURE_STORAGE_ENDPOINT)", ErrNotConfigured)
	}
	endpoint := account
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https://" + endpoint + ".blob.core.windows.net"
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	c, err := azblob.NewClient(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	return &azureClient{client: c}, nil
}

func (a *azureClient) Buckets(ctx context.Context) ([]Bucket, error) {
	out := []Bucket{}
	pager := a.client.NewListContainersPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range page.ContainerItems {
			if c.Name != nil {
				out = append(out, Bucket{Name: *c.Name})
			}
		}
	}
	return out, nil
}

func (a *azureClient) List(ctx context.Context, bucket, prefix, _ string) (ListResult, error) {
	res := ListResult{Objects: []Object{}}
	dirs := map[string]bool{}
	pager := a.client.NewListBlobsFlatPager(bucket, &azblob.ListBlobsFlatOptions{Prefix: &prefix})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return res, err
		}
		if page.Segment == nil {
			continue
		}
		for _, b := range page.Segment.BlobItems {
			if b == nil || b.Name == nil {
				continue
			}
			name := *b.Name
			rest := strings.TrimPrefix(name, prefix)
			if i := strings.Index(rest, "/"); i >= 0 {
				dirs[prefix+rest[:i+1]] = true
				continue
			}
			obj := Object{Key: name}
			if b.Properties != nil {
				if b.Properties.ContentLength != nil {
					obj.Size = *b.Properties.ContentLength
				}
				if b.Properties.LastModified != nil {
					obj.Modified = b.Properties.LastModified.UTC().Format("2006-01-02T15:04:05Z")
				}
			}
			res.Objects = append(res.Objects, obj)
		}
	}
	for d := range dirs {
		res.Objects = append(res.Objects, Object{Key: d, IsDir: true})
	}
	sort.Slice(res.Objects, func(i, j int) bool { return res.Objects[i].Key < res.Objects[j].Key })
	return res, nil
}

func (a *azureClient) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	resp, err := a.client.DownloadStream(ctx, bucket, key, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
