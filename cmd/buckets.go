/*
Copyright © 2022 Przemek Malak przemek.malak@gmail.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// maxDeleteBatch is the maximum number of keys accepted by a single DeleteObjects call.
const maxDeleteBatch = 1000

// maxListBuckets is the largest page ListBuckets accepts. Setting MaxBuckets makes the
// request paginated, which S3 requires for accounts with a bucket quota above 10,000.
const maxListBuckets = 10000

// s3API is the subset of the S3 client used by s3destroyer.
// It is satisfied by *s3.Client and by fakes in tests.
type s3API interface {
	s3.ListBucketsAPIClient
	s3.ListObjectsV2APIClient
	s3.ListObjectVersionsAPIClient
	GetBucketLocation(ctx context.Context, params *s3.GetBucketLocationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
	DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
}

func listBuckets(ctx context.Context, clt s3API, w io.Writer) error {
	i := 0
	p := s3.NewListBucketsPaginator(clt, &s3.ListBucketsInput{MaxBuckets: aws.Int32(maxListBuckets)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list buckets: %w", err)
		}
		for _, b := range page.Buckets {
			fmt.Fprintf(w, "%d %s\n", i, aws.ToString(b.Name))
			i++
		}
	}
	return nil
}

func listObjects(ctx context.Context, clt s3API, bucketName string, w io.Writer) error {
	p := s3.NewListObjectsV2Paginator(clt, &s3.ListObjectsV2Input{Bucket: aws.String(bucketName)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}
		for _, item := range page.Contents {
			fmt.Fprintln(w, aws.ToString(item.Key))
		}
	}
	return nil
}

func getBucketLocation(ctx context.Context, clt s3API, bucketName string) (string, error) {
	out, err := clt.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(bucketName)})
	if err != nil {
		return "", fmt.Errorf("failed to get location of bucket %s: %w", bucketName, err)
	}
	switch location := string(out.LocationConstraint); location {
	case "":
		return "us-east-1", nil
	case "EU":
		return "eu-west-1", nil
	default:
		return location, nil
	}
}

// bucketStats describes what destroying a bucket would remove.
type bucketStats struct {
	Versions      int
	DeleteMarkers int
}

// countBucketContents counts all object versions and delete markers in a bucket.
// For buckets without versioning every object is reported as a single version.
func countBucketContents(ctx context.Context, clt s3API, bucketName string) (bucketStats, error) {
	var stats bucketStats
	p := s3.NewListObjectVersionsPaginator(clt, &s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return stats, fmt.Errorf("failed to list object versions: %w", err)
		}
		stats.Versions += len(page.Versions)
		stats.DeleteMarkers += len(page.DeleteMarkers)
	}
	return stats, nil
}

// removeBucket empties the bucket and then deletes it. Deleted keys are reported to w.
//
// It lists the bucket with ListObjectVersions only - the same call countBucketContents
// uses for --dry-run - so a successful preview means the listing will also work here.
// ListObjectVersions also covers buckets without versioning (their objects have the
// version "null"), and deleting exact versions never creates new delete markers.
func removeBucket(ctx context.Context, clt s3API, bucketName string, w io.Writer) error {
	pv := s3.NewListObjectVersionsPaginator(clt, &s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)})
	for pv.HasMorePages() {
		page, err := pv.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list object versions: %w", err)
		}

		objects := make([]types.ObjectIdentifier, 0, len(page.DeleteMarkers)+len(page.Versions))
		for _, item := range page.DeleteMarkers {
			objects = append(objects, types.ObjectIdentifier{Key: item.Key, VersionId: item.VersionId})
		}
		for _, item := range page.Versions {
			objects = append(objects, types.ObjectIdentifier{Key: item.Key, VersionId: item.VersionId})
		}

		if err := deleteObjects(ctx, clt, bucketName, objects, w); err != nil {
			return err
		}
	}

	//delete bucket
	_, err := clt.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucketName)})
	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}
	return nil
}

// deleteObjects removes objects in batches of up to maxDeleteBatch.
//
// Only deletions confirmed by S3 in the response are written to w, so the output
// is an accurate record of what was removed. It fails if a request fails (nothing
// from that batch is reported as deleted) or if S3 reports any object it could not
// delete (the rest of that batch is still reported).
func deleteObjects(ctx context.Context, clt s3API, bucketName string, objs []types.ObjectIdentifier, w io.Writer) error {
	deleted := 0
	for i := 0; i < len(objs); i += maxDeleteBatch {
		batch := objs[i:min(i+maxDeleteBatch, len(objs))]

		out, err := clt.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &types.Delete{
				Objects: batch,
				Quiet:   aws.Bool(false), // S3 lists every deleted object, so output is confirmed
			},
		})
		if err != nil {
			return fmt.Errorf("delete request for a batch of %d object(s) failed, none of them confirmed deleted "+
				"(%d deleted before this batch): %w", len(batch), deleted, err)
		}

		for _, d := range out.Deleted {
			if d.VersionId != nil {
				fmt.Fprintf(w, "Deleted: %s VersionId: %s\n", aws.ToString(d.Key), aws.ToString(d.VersionId))
			} else {
				fmt.Fprintf(w, "Deleted: %s\n", aws.ToString(d.Key))
			}
		}
		deleted += len(out.Deleted)

		if len(out.Errors) > 0 {
			errs := make([]error, 0, len(out.Errors))
			for _, e := range out.Errors {
				errs = append(errs, fmt.Errorf("%s (version %s): %s: %s",
					aws.ToString(e.Key), aws.ToString(e.VersionId), aws.ToString(e.Code), aws.ToString(e.Message)))
			}
			return fmt.Errorf("failed to delete %d object(s):\n%w", len(out.Errors), errors.Join(errs...))
		}
	}
	return nil
}
