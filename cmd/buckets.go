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
	"fmt"
	"log"

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

		objects := []types.ObjectIdentifier{}

		for _, item := range out.Contents {
			fmt.Printf("%s\n", *item.Key)
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(*item.Key)})
		}

		//deleteObjects(profile, region, bucketName, objects)
		if out.IsTruncated {
			in.ContinuationToken = out.ContinuationToken
		} else {
			break
		}
	}
}

func listObjectVersions(profile string, region string, bucketName string) {
	bucketRegion := getBucketLocation(profile, region, bucketName)
	clt := getS3Slient(profile, bucketRegion)
	in := &s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)}
	for {
		out, err := clt.ListObjectVersions(context.TODO(), in)
		if err != nil {
			log.Fatalf("Failed to list version objects: %v", err)
		}
		//deleteObjects(profile, bucketName, *out)
		// for _, item := range out.DeleteMarkers {
		// 	fmt.Printf("%s %s\n", bucketName, item.Key, item.VersionId)
		// }

		// for _, item := range out.Versions {
		// 	fmt.Printf("%s %s\n", bucketName, item.Key, item.VersionId)
		// }

		if out.IsTruncated {
			in.VersionIdMarker = out.NextVersionIdMarker
			in.KeyMarker = out.NextKeyMarker
		} else {
			break
		}
	}
}

func getBucketLocation(profile string, region string, bucketName string) (location string) {
	clt := getS3Slient(profile, region)
	in := &s3.GetBucketLocationInput{Bucket: aws.String(bucketName)}
	out, err := clt.GetBucketLocation(context.TODO(), in)
	if err != nil {
		panic(err)
	}
	location = string(out.LocationConstraint)
	fmt.Println(location)
	return location
}

func deleteObjects(profile string, region string, bucketName string, objs []types.ObjectIdentifier) {
	bucket_region := getBucketLocation(profile, region, bucketName)
	clt := getS3Slient(profile, bucket_region)
	maxLen := 1000

	for i := 0; i < len(objs); i += maxLen {
		high := i + maxLen
		if high > len(objs) {
			high = len(objs)
		}

		objects := []types.ObjectIdentifier{}
		fmt.Printf("objects count: %d", len(objects))
		fmt.Printf("objs count: %d", len(objs))
		for _, obj := range objs[i:high] {
			if obj.VersionId != nil {
				fmt.Printf("Failed: %s %s\n", *obj.Key, *aws.String(*obj.VersionId))
				objects = append(objects, types.ObjectIdentifier{Key: aws.String(*obj.Key), VersionId: aws.String(*obj.VersionId)})
			} else {
				fmt.Printf("Failed: %s\n", *obj.Key)
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
