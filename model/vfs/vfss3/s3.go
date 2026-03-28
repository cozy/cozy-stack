package vfss3

import (
	"context"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/minio/minio-go/v7"
)

// maxNbFilesToDelete is the max number of objects per RemoveObjects batch.
const maxNbFilesToDelete = 1000

// deletePrefixObjects deletes all objects in a bucket under a given prefix.
func deletePrefixObjects(ctx context.Context, client *minio.Client, bucket, prefix string) error {
	objectsCh := make(chan minio.ObjectInfo)
	var listErr error
	go func() {
		defer close(objectsCh)
		for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		}) {
			if obj.Err != nil {
				listErr = obj.Err
				return
			}
			objectsCh <- obj
		}
	}()
	rmErr := removeObjects(ctx, client, bucket, objectsCh)
	if listErr != nil {
		return listErr
	}
	return rmErr
}

// deleteObjects deletes a list of named objects from a bucket.
func deleteObjects(ctx context.Context, client *minio.Client, bucket string, objNames []string) error {
	if len(objNames) == 0 {
		return nil
	}
	objectsCh := make(chan minio.ObjectInfo, len(objNames))
	for _, name := range objNames {
		objectsCh <- minio.ObjectInfo{Key: name}
	}
	close(objectsCh)
	return removeObjects(ctx, client, bucket, objectsCh)
}

func removeObjects(ctx context.Context, client *minio.Client, bucket string, objectsCh <-chan minio.ObjectInfo) error {
	var errm error
	for err := range client.RemoveObjects(ctx, bucket, objectsCh, minio.RemoveObjectsOptions{}) {
		errm = multierror.Append(errm, err.Err)
	}
	return errm
}
