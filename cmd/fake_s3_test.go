package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeEntry is a single object version or delete marker stored in fakeS3.
type fakeEntry struct {
	key          string
	versionID    string
	deleteMarker bool
}

// fakeS3 is an in-memory, single-bucket S3 with the pagination and deletion
// semantics s3destroyer relies on:
//   - list pages are capped at pageSize (S3 uses 1000),
//   - continuation tokens and markers are key based, so deleting already listed
//     entries does not shift later pages,
//   - deleting a key without a version in a versioned bucket adds a delete marker,
//   - DeleteObjects rejects more than 1000 keys and reports per-key failures,
//   - DeleteBucket fails while the bucket is not empty.
type fakeS3 struct {
	versioned bool
	pageSize  int
	location  types.BucketLocationConstraint
	buckets   []string

	entries     []fakeEntry // sorted by key asc, then versionID desc (newest first)
	nextVersion int

	locked           map[string]bool // keys that DeleteObjects refuses to delete
	denied           map[string]bool // operations (e.g. "ListObjectsV2") failing with AccessDenied
	deleteObjectsErr error           // returned by every DeleteObjects call when set

	deleteBatches     []int // number of keys in each DeleteObjects call
	markersCreated    int   // delete markers added by DeleteObjects calls without VersionId
	deleteBucketCalls int
	bucketDeleted     bool
}

var _ s3API = (*fakeS3)(nil)

func newFakeS3(versioned bool) *fakeS3 {
	return &fakeS3{versioned: versioned, pageSize: 1000, locked: map[string]bool{}, denied: map[string]bool{}}
}

func (f *fakeS3) checkAccess(op string) error {
	if f.denied[op] {
		return fmt.Errorf("AccessDenied: not authorized to perform %s", op)
	}
	return nil
}

func (f *fakeS3) newVersionID() string {
	if !f.versioned {
		return "null"
	}
	f.nextVersion++
	return fmt.Sprintf("v%08d", f.nextVersion)
}

func (f *fakeS3) sort() {
	slices.SortFunc(f.entries, func(a, b fakeEntry) int {
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return strings.Compare(b.versionID, a.versionID)
	})
}

// put stores a new version of key (or overwrites it in an unversioned bucket).
func (f *fakeS3) put(key string) {
	if !f.versioned {
		f.removeVersion(key, "null")
	}
	f.entries = append(f.entries, fakeEntry{key: key, versionID: f.newVersionID()})
	f.sort()
}

func (f *fakeS3) putDeleteMarker(key string) {
	f.entries = append(f.entries, fakeEntry{key: key, versionID: f.newVersionID(), deleteMarker: true})
	f.sort()
}

func (f *fakeS3) removeVersion(key, versionID string) {
	f.entries = slices.DeleteFunc(f.entries, func(e fakeEntry) bool {
		return e.key == key && e.versionID == versionID
	})
}

func (f *fakeS3) limit(maxKeys *int32) int {
	if maxKeys != nil && int(*maxKeys) > 0 && int(*maxKeys) < f.pageSize {
		return int(*maxKeys)
	}
	return f.pageSize
}

func (f *fakeS3) ListBuckets(_ context.Context, in *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	if err := f.checkAccess("ListBuckets"); err != nil {
		return nil, err
	}
	names := slices.Sorted(slices.Values(f.buckets))
	start := 0
	if in.ContinuationToken != nil {
		start, _ = slices.BinarySearch(names, *in.ContinuationToken)
		start++
	}
	end := min(start+f.limit(in.MaxBuckets), len(names))

	out := &s3.ListBucketsOutput{}
	for _, n := range names[start:end] {
		out.Buckets = append(out.Buckets, types.Bucket{Name: aws.String(n)})
	}
	if end < len(names) {
		out.ContinuationToken = aws.String(names[end-1])
	}
	return out, nil
}

func (f *fakeS3) GetBucketLocation(context.Context, *s3.GetBucketLocationInput, ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	if err := f.checkAccess("GetBucketLocation"); err != nil {
		return nil, err
	}
	return &s3.GetBucketLocationOutput{LocationConstraint: f.location}, nil
}

// currentKeys returns keys whose newest entry is an object, not a delete marker.
func (f *fakeS3) currentKeys() []string {
	var keys []string
	for i, e := range f.entries {
		newest := i == 0 || f.entries[i-1].key != e.key
		if newest && !e.deleteMarker {
			keys = append(keys, e.key)
		}
	}
	return keys
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if err := f.checkAccess("ListObjectsV2"); err != nil {
		return nil, err
	}
	keys := f.currentKeys()
	start := 0
	if in.ContinuationToken != nil {
		start, _ = slices.BinarySearch(keys, *in.ContinuationToken)
		if start < len(keys) && keys[start] == *in.ContinuationToken {
			start++
		}
	}
	end := min(start+f.limit(in.MaxKeys), len(keys))

	out := &s3.ListObjectsV2Output{
		ContinuationToken: in.ContinuationToken, // S3 echoes the request token here
		IsTruncated:       aws.Bool(end < len(keys)),
	}
	for _, k := range keys[start:end] {
		out.Contents = append(out.Contents, types.Object{Key: aws.String(k)})
	}
	if end < len(keys) {
		out.NextContinuationToken = aws.String(keys[end-1])
	}
	return out, nil
}

func (f *fakeS3) ListObjectVersions(_ context.Context, in *s3.ListObjectVersionsInput, _ ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error) {
	if err := f.checkAccess("ListObjectVersions"); err != nil {
		return nil, err
	}
	start := 0
	if in.KeyMarker != nil {
		start = slices.IndexFunc(f.entries, func(e fakeEntry) bool {
			if e.key != *in.KeyMarker {
				return e.key > *in.KeyMarker
			}
			// same key: versions are newest first, so "after" means older
			return in.VersionIdMarker != nil && e.versionID < *in.VersionIdMarker
		})
		if start < 0 {
			start = len(f.entries)
		}
	}
	end := min(start+f.limit(in.MaxKeys), len(f.entries))

	out := &s3.ListObjectVersionsOutput{IsTruncated: aws.Bool(end < len(f.entries))}
	for _, e := range f.entries[start:end] {
		if e.deleteMarker {
			out.DeleteMarkers = append(out.DeleteMarkers, types.DeleteMarkerEntry{Key: aws.String(e.key), VersionId: aws.String(e.versionID)})
		} else {
			out.Versions = append(out.Versions, types.ObjectVersion{Key: aws.String(e.key), VersionId: aws.String(e.versionID)})
		}
	}
	if end < len(f.entries) {
		last := f.entries[end-1]
		out.NextKeyMarker = aws.String(last.key)
		out.NextVersionIdMarker = aws.String(last.versionID)
	}
	return out, nil
}

func (f *fakeS3) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	if err := f.checkAccess("DeleteObjects"); err != nil {
		return nil, err
	}
	if f.deleteObjectsErr != nil {
		return nil, f.deleteObjectsErr
	}
	objs := in.Delete.Objects
	if len(objs) == 0 || len(objs) > 1000 {
		return nil, fmt.Errorf("MalformedXML: DeleteObjects got %d keys", len(objs))
	}
	f.deleteBatches = append(f.deleteBatches, len(objs))

	out := &s3.DeleteObjectsOutput{}
	for _, o := range objs {
		key := aws.ToString(o.Key)
		switch {
		case f.locked[key]:
			out.Errors = append(out.Errors, types.Error{
				Key: o.Key, VersionId: o.VersionId,
				Code: aws.String("AccessDenied"), Message: aws.String("Access Denied because object protected by object lock."),
			})
			continue
		case o.VersionId != nil:
			f.removeVersion(key, *o.VersionId)
		case f.versioned:
			f.putDeleteMarker(key)
			f.markersCreated++
		default:
			f.removeVersion(key, "null")
		}
		if !aws.ToBool(in.Delete.Quiet) {
			out.Deleted = append(out.Deleted, types.DeletedObject{Key: o.Key, VersionId: o.VersionId})
		}
	}
	return out, nil
}

func (f *fakeS3) DeleteBucket(context.Context, *s3.DeleteBucketInput, ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	if err := f.checkAccess("DeleteBucket"); err != nil {
		return nil, err
	}
	f.deleteBucketCalls++
	if len(f.entries) > 0 {
		return nil, errors.New("BucketNotEmpty: The bucket you tried to delete is not empty")
	}
	f.bucketDeleted = true
	return &s3.DeleteBucketOutput{}, nil
}
