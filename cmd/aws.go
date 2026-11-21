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

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// newS3Client creates an S3 client from the shared AWS configuration (~/.aws/config),
// optionally overriding the profile and the region.
func newS3Client(ctx context.Context, profile string, region string) (*s3.Client, error) {
	var opts []func(*config.LoadOptions) error
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}

// newBucketClient creates an S3 client for the region the bucket lives in
// and returns that region.
func newBucketClient(ctx context.Context, profile string, region string, bucketName string) (s3API, string, error) {
	clt, err := newS3Client(ctx, profile, region)
	if err != nil {
		return nil, "", err
	}
	bucketRegion, err := getBucketLocation(ctx, clt, bucketName)
	if err != nil {
		return nil, "", err
	}
	clt, err = newS3Client(ctx, profile, bucketRegion)
	if err != nil {
		return nil, "", err
	}
	return clt, bucketRegion, nil
}
