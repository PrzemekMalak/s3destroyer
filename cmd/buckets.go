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
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func listBuckets(profile string, region string) {
	clt := getS3Slient(profile, region)
	input := &s3.ListBucketsInput{}
	result, err := clt.ListBuckets(context.TODO(), input)
	if err != nil {
		log.Fatalf("Failed to list buckets: %v", err.Error())
	}
	for i, b := range result.Buckets {
		fmt.Printf("%d %s\n", i, *b.Name)
	}
}

func listObjects(profile string, region string, bucketName string) {
	bucket_region := getBucketLocation(profile, region, bucketName)
	clt := getS3Slient(profile, bucket_region)
	in := &s3.ListObjectsV2Input{Bucket: aws.String(bucketName)}
	for {
		out, err := clt.ListObjectsV2(context.TODO(), in)
		if err != nil {
			log.Fatalf("Failed to list objects: %v", err.Error())
		}

		for _, item := range out.Contents {
			fmt.Printf("%s\n", *item.Key)
		}

		if out.IsTruncated {
			in.ContinuationToken = out.ContinuationToken
		} else {
			break
		}
	}
}

func getBucketLocation(profile string, region string, bucketName string) (location string) {
	clt := getS3Slient(profile, region)
	in := &s3.GetBucketLocationInput{Bucket: aws.String(bucketName)}
	out, err := clt.GetBucketLocation(context.TODO(), in)
	var re s3.ResponseError
	if err != nil {
		if errors.As(err, &re) {
			fmt.Printf("Error getting bucket: %s location\n\n%s", bucketName, err.Error())
			os.Exit(0)
		}
		os.Exit(1)
	}
	location = string(out.LocationConstraint)
	return location
}

func removeBucket(profile string, region string, bucketName string) {
	bucketRegion := getBucketLocation(profile, region, bucketName)
	clt := getS3Slient(profile, bucketRegion)

	//delete files
	in := &s3.ListObjectsV2Input{Bucket: aws.String(bucketName)}
	for {
		out, err := clt.ListObjectsV2(context.TODO(), in)
		if err != nil {
			log.Fatalf("Failed to get objects: %v", err.Error())
		}

		objects := []types.ObjectIdentifier{}

		for _, item := range out.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(*item.Key)})
		}

		deleteObjects(&clt, bucketName, objects)
		if out.IsTruncated {
			in.ContinuationToken = out.ContinuationToken
		} else {
			break
		}
	}

	//delete versions and delete markers
	inVer := &s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)}
	for {
		out, err := clt.ListObjectVersions(context.TODO(), inVer)
		if err != nil {
			log.Fatalf("Failed to list version objects: %v", err)
		}
		objects := []types.ObjectIdentifier{}

		for _, item := range out.DeleteMarkers {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(*item.Key), VersionId: aws.String(*item.VersionId)})
		}

		for _, item := range out.Versions {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(*item.Key), VersionId: aws.String(*item.VersionId)})
		}

		deleteObjects(&clt, bucketName, objects)

		if out.IsTruncated {
			inVer.VersionIdMarker = out.NextVersionIdMarker
			inVer.KeyMarker = out.NextKeyMarker
		} else {
			break
		}
	}

	//delete bucket
	_, err := clt.DeleteBucket(context.TODO(), &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		log.Fatalf("Failed to delete bucket: %v", err)
	}
}

func deleteObjects(clt *s3.Client, bucketName string, objs []types.ObjectIdentifier) {
	maxLen := 1000

	for i := 0; i < len(objs); i += maxLen {
		high := i + maxLen
		if high > len(objs) {
			high = len(objs)
		}

		objects := []types.ObjectIdentifier{}
		for _, obj := range objs[i:high] {
			if obj.VersionId != nil {
				fmt.Printf("Filename: %s VersionId: %s\n", *obj.Key, *aws.String(*obj.VersionId))
				objects = append(objects, types.ObjectIdentifier{Key: aws.String(*obj.Key), VersionId: aws.String(*obj.VersionId)})
			} else {
				fmt.Printf("Filename: %s\n", *obj.Key)
				objects = append(objects, types.ObjectIdentifier{Key: aws.String(*obj.Key)})
			}
		}
		in := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   false,
			},
		}
		_, err := clt.DeleteObjects(context.TODO(), in)
		if err != nil {
			fmt.Printf("Failed: %s\n", err)
		}
	}
}
