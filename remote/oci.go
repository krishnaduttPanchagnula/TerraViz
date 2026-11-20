package remote

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

func ociCompartment() string {
	if c := os.Getenv("OCI_COMPARTMENT_ID"); c != "" {
		return c
	}
	return os.Getenv("OCI_TENANCY_OCID")
}

type ociClient struct {
	client      objectstorage.ObjectStorageClient
	namespace   string
	compartment string
}

func newOCI(ctx context.Context) (Client, error) {
	provider := common.DefaultConfigProvider()
	compartment := ociCompartment()
	if compartment == "" {
		tenancy, err := provider.TenancyOCID()
		if err == nil {
			compartment = tenancy
		}
	}
	if compartment == "" {
		return nil, fmt.Errorf("%w: set OCI_COMPARTMENT_ID in the environment", ErrNotConfigured)
	}
	c, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	ns, err := c.GetNamespace(ctx, objectstorage.GetNamespaceRequest{})
	if err != nil {
		return nil, fmt.Errorf("OCI namespace lookup: %w", err)
	}
	if ns.Value == nil {
		return nil, fmt.Errorf("OCI namespace lookup returned no namespace")
	}
	return &ociClient{client: c, namespace: *ns.Value, compartment: compartment}, nil
}

func (o *ociClient) Buckets(ctx context.Context) ([]Bucket, error) {
	out := []Bucket{}
	page := ""
	for {
		req := objectstorage.ListBucketsRequest{
			NamespaceName: &o.namespace,
			CompartmentId: &o.compartment,
			Limit:         intp(100),
		}
		if page != "" {
			req.Page = &page
		}
		resp, err := o.client.ListBuckets(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, b := range resp.Items {
			if b.Name != nil {
				out = append(out, Bucket{Name: *b.Name})
			}
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = *resp.OpcNextPage
	}
	return out, nil
}

func (o *ociClient) List(ctx context.Context, bucket, prefix, token string) (ListResult, error) {
	res := ListResult{Objects: []Object{}}
	req := objectstorage.ListObjectsRequest{
		NamespaceName: &o.namespace,
		BucketName:    &bucket,
		Prefix:        &prefix,
		Delimiter:     strp("/"),
		Limit:         intp(500),
	}
	if token != "" {
		req.Start = &token
	}
	resp, err := o.client.ListObjects(ctx, req)
	if err != nil {
		return res, err
	}
	for _, p := range resp.Prefixes {
		res.Objects = append(res.Objects, Object{Key: p, IsDir: true})
	}
	for _, obj := range resp.Objects {
		if obj.Name == nil {
			continue
		}
		item := Object{Key: *obj.Name}
		if obj.Size != nil {
			item.Size = *obj.Size
		}
		if obj.TimeModified != nil {
			item.Modified = obj.TimeModified.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		res.Objects = append(res.Objects, item)
	}
	if resp.NextStartWith != nil {
		res.NextToken = *resp.NextStartWith
	}
	return res, nil
}

func (o *ociClient) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	resp, err := o.client.GetObject(ctx, objectstorage.GetObjectRequest{
		NamespaceName: &o.namespace,
		BucketName:    &bucket,
		ObjectName:    &key,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Content.Close()
	return io.ReadAll(io.LimitReader(resp.Content, 64<<20))
}

func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }
