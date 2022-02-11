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
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func getS3Slient(profile string, region string) (client s3.Client) {
	// Load the Shared AWS Configuration (~/.aws/config)
	var cfg aws.Config
	var err error
	if len(profile) > 0 && len(region) > 0 {
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithSharedConfigProfile(profile),
			config.WithRegion(region),
		)
	} else if len(profile) > 0 && len(region) == 0 {
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithSharedConfigProfile(profile),
		)
	} else if len(profile) == 0 && len(region) > 0 {
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(region),
		)
	}
	if err != nil {
		log.Fatal(err)
	}

	// Create an Amazon S3 service client
	clt := s3.NewFromConfig(cfg)
	return *clt

}
