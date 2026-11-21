package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const testBucket = "test-bucket"

func keyName(i int) string { return fmt.Sprintf("dir/file-%05d.txt", i) }

func TestListObjectsReturnsEveryKeyOnceAcrossPages(t *testing.T) {
	f := newFakeS3(false)
	const n = 2500
	for i := range n {
		f.put(keyName(i))
	}

	var buf bytes.Buffer
	if err := listObjects(context.Background(), f, testBucket, &buf); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != n {
		t.Fatalf("got %d keys, want %d", len(lines), n)
	}
	for i, l := range lines {
		if l != keyName(i) {
			t.Fatalf("line %d = %q, want %q", i, l, keyName(i))
		}
	}
}

func TestListObjectsHidesDeletedKeys(t *testing.T) {
	f := newFakeS3(true)
	f.put("kept")
	f.put("deleted")
	f.putDeleteMarker("deleted")

	var buf bytes.Buffer
	if err := listObjects(context.Background(), f, testBucket, &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "kept\n" {
		t.Fatalf("got %q, want %q", got, "kept\n")
	}
}

func TestListBucketsAcrossPages(t *testing.T) {
	f := newFakeS3(false)
	f.pageSize = 2
	f.buckets = []string{"c", "a", "e", "b", "d"}

	var buf bytes.Buffer
	if err := listBuckets(context.Background(), f, &buf); err != nil {
		t.Fatal(err)
	}
	want := "0 a\n1 b\n2 c\n3 d\n4 e\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGetBucketLocation(t *testing.T) {
	tests := []struct {
		constraint types.BucketLocationConstraint
		want       string
	}{
		{"", "us-east-1"},
		{"EU", "eu-west-1"},
		{"eu-central-1", "eu-central-1"},
	}
	for _, tt := range tests {
		f := newFakeS3(false)
		f.location = tt.constraint
		got, err := getBucketLocation(context.Background(), f, testBucket)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("constraint %q: got %q, want %q", tt.constraint, got, tt.want)
		}
	}
}

func TestCountBucketContents(t *testing.T) {
	f := newFakeS3(true)
	f.pageSize = 7
	for i := range 20 {
		f.put(keyName(i))
		f.put(keyName(i)) // second version
	}
	for i := range 5 {
		f.putDeleteMarker(keyName(i))
	}

	stats, err := countBucketContents(context.Background(), f, testBucket)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Versions != 40 || stats.DeleteMarkers != 5 {
		t.Fatalf("got %+v, want 40 versions and 5 delete markers", stats)
	}
}

func identifiers(n int) []types.ObjectIdentifier {
	objs := make([]types.ObjectIdentifier, n)
	for i := range objs {
		objs[i] = types.ObjectIdentifier{Key: aws.String(keyName(i))}
	}
	return objs
}

func TestDeleteObjectsSplitsIntoBatchesOf1000(t *testing.T) {
	tests := []struct {
		n    int
		want []int
	}{
		{0, nil},
		{1, []int{1}},
		{1000, []int{1000}},
		{1001, []int{1000, 1}},
		{2500, []int{1000, 1000, 500}},
	}
	for _, tt := range tests {
		f := newFakeS3(false)
		for i := range tt.n {
			f.put(keyName(i))
		}
		if err := deleteObjects(context.Background(), f, testBucket, identifiers(tt.n), io.Discard); err != nil {
			t.Fatalf("n=%d: %v", tt.n, err)
		}
		if !slices.Equal(f.deleteBatches, tt.want) {
			t.Errorf("n=%d: batches %v, want %v", tt.n, f.deleteBatches, tt.want)
		}
		if len(f.entries) != 0 {
			t.Errorf("n=%d: %d objects left", tt.n, len(f.entries))
		}
	}
}

func TestDeleteObjectsReportsObjectsS3RefusedToDelete(t *testing.T) {
	f := newFakeS3(false)
	for i := range 3 {
		f.put(keyName(i))
	}
	f.locked[keyName(1)] = true

	err := deleteObjects(context.Background(), f, testBucket, identifiers(3), io.Discard)
	if err == nil {
		t.Fatal("expected an error for the locked object")
	}
	for _, want := range []string{"failed to delete 1 object(s)", keyName(1), "AccessDenied"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestDeleteObjectsReturnsRequestError(t *testing.T) {
	f := newFakeS3(false)
	f.deleteObjectsErr = errors.New("boom")

	err := deleteObjects(context.Background(), f, testBucket, identifiers(1), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v, want error wrapping boom", err)
	}
}

func TestRemoveBucketUnversioned(t *testing.T) {
	f := newFakeS3(false)
	for i := range 2500 {
		f.put(keyName(i))
	}

	if err := removeBucket(context.Background(), f, testBucket, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !f.bucketDeleted {
		t.Fatalf("bucket not deleted, %d entries left", len(f.entries))
	}
}

func TestRemoveBucketVersioned(t *testing.T) {
	f := newFakeS3(true)
	for i := range 1500 {
		f.put(keyName(i))
		f.put(keyName(i))
	}
	for i := range 300 {
		f.putDeleteMarker(keyName(i))
	}

	if err := removeBucket(context.Background(), f, testBucket, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !f.bucketDeleted {
		t.Fatalf("bucket not deleted, %d entries left", len(f.entries))
	}
	for i, b := range f.deleteBatches {
		if b > 1000 {
			t.Errorf("batch %d has %d keys", i, b)
		}
	}
}

func TestRemoveBucketStopsWhenObjectCannotBeDeleted(t *testing.T) {
	f := newFakeS3(false)
	for i := range 10 {
		f.put(keyName(i))
	}
	f.locked[keyName(3)] = true

	err := removeBucket(context.Background(), f, testBucket, io.Discard)
	if err == nil || !strings.Contains(err.Error(), keyName(3)) {
		t.Fatalf("got %v, want error naming %s", err, keyName(3))
	}
	if f.deleteBucketCalls != 0 {
		t.Error("DeleteBucket must not be called when objects are left")
	}
}

// The --dry-run preview (countBucketContents) and the real deletion (removeBucket)
// must list the bucket through the same API, so that a role that can preview a
// bucket can also empty it, and a role that cannot is stopped already at preview.
func TestPreviewAndDestroyNeedTheSameListPermission(t *testing.T) {
	for _, op := range []string{"ListObjectsV2", "ListObjectVersions"} {
		for _, versioned := range []bool{false, true} {
			t.Run(fmt.Sprintf("deny %s versioned=%v", op, versioned), func(t *testing.T) {
				f := newFakeS3(versioned)
				for i := range 1200 {
					f.put(keyName(i))
				}
				f.denied[op] = true

				_, previewErr := countBucketContents(context.Background(), f, testBucket)
				destroyErr := removeBucket(context.Background(), f, testBucket, io.Discard)

				if (previewErr == nil) != (destroyErr == nil) {
					t.Fatalf("preview error = %v, destroy error = %v; both must succeed or both fail", previewErr, destroyErr)
				}
				if destroyErr != nil && len(f.deleteBatches) > 0 {
					t.Errorf("objects were deleted although listing failed: %v", f.deleteBatches)
				}
				if destroyErr == nil && !f.bucketDeleted {
					t.Errorf("bucket not deleted, %d entries left", len(f.entries))
				}
			})
		}
	}
}

// Deleting without a VersionId in a versioned bucket only hides the object behind a
// new delete marker. If the run is interrupted, those objects stay hidden but billed,
// so removeBucket must always delete exact versions and never create markers.
func TestRemoveBucketDoesNotCreateDeleteMarkers(t *testing.T) {
	f := newFakeS3(true)
	for i := range 1500 {
		f.put(keyName(i))
		f.put(keyName(i))
	}
	for i := range 100 {
		f.putDeleteMarker(keyName(i))
	}

	if err := removeBucket(context.Background(), f, testBucket, io.Discard); err != nil {
		t.Fatal(err)
	}
	if f.markersCreated != 0 {
		t.Errorf("removeBucket created %d delete markers, want 0", f.markersCreated)
	}
	if !f.bucketDeleted {
		t.Errorf("bucket not deleted, %d entries left", len(f.entries))
	}
}

func TestDeleteObjectsReportsOnlyConfirmedDeletions(t *testing.T) {
	t.Run("all deleted", func(t *testing.T) {
		f := newFakeS3(false)
		for i := range 3 {
			f.put(keyName(i))
		}
		var buf bytes.Buffer
		if err := deleteObjects(context.Background(), f, testBucket, identifiers(3), &buf); err != nil {
			t.Fatal(err)
		}
		want := "Deleted: " + keyName(0) + "\nDeleted: " + keyName(1) + "\nDeleted: " + keyName(2) + "\n"
		if buf.String() != want {
			t.Fatalf("output %q, want %q", buf.String(), want)
		}
	})

	t.Run("some refused", func(t *testing.T) {
		f := newFakeS3(false)
		for i := range 3 {
			f.put(keyName(i))
		}
		f.locked[keyName(1)] = true
		var buf bytes.Buffer
		if err := deleteObjects(context.Background(), f, testBucket, identifiers(3), &buf); err == nil {
			t.Fatal("expected an error for the locked object")
		}
		want := "Deleted: " + keyName(0) + "\nDeleted: " + keyName(2) + "\n"
		if buf.String() != want {
			t.Fatalf("output %q, want %q (the refused key must not be listed)", buf.String(), want)
		}
	})

	t.Run("request failed", func(t *testing.T) {
		f := newFakeS3(false)
		f.deleteObjectsErr = errors.New("connection reset")
		var buf bytes.Buffer
		err := deleteObjects(context.Background(), f, testBucket, identifiers(3), &buf)
		if err == nil || !strings.Contains(err.Error(), "none of them confirmed deleted") {
			t.Fatalf("got %v, want an error saying the batch is unconfirmed", err)
		}
		if buf.Len() != 0 {
			t.Fatalf("nothing was confirmed, but output is %q", buf.String())
		}
	})

	t.Run("failure in a later batch", func(t *testing.T) {
		f := newFakeS3(false)
		for i := range 1500 {
			f.put(keyName(i))
		}
		f.locked[keyName(1200)] = true
		var buf bytes.Buffer
		if err := deleteObjects(context.Background(), f, testBucket, identifiers(1500), &buf); err == nil {
			t.Fatal("expected an error for the locked object")
		}
		if got := strings.Count(buf.String(), "Deleted: "); got != 1499 {
			t.Fatalf("reported %d deletions, want 1499", got)
		}
		if strings.Contains(buf.String(), keyName(1200)) {
			t.Fatal("the refused key must not be reported as deleted")
		}
	})
}
