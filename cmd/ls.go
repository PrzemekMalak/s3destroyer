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
	"github.com/spf13/cobra"
)

// lsCmd represents the ls command
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "Generates a list of buckets or files inside a bucket",
	Long: `Generates a list of buckets or files inside a bucket.

To generate the list of files inside a bucket add --name flag followed by name of the bucket:
s3destroyer ls --name bucketname

To generate the list of buckets: 
s3destroyer ls`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		profile, _ := cmd.Flags().GetString("profile")
		region, _ := cmd.Flags().GetString("region")

		ctx := cmd.Context()
		if len(name) > 0 {
			clt, _, err := newBucketClient(ctx, profile, region, name)
			if err != nil {
				return err
			}
			return listObjects(ctx, clt, name, cmd.OutOrStdout())
		}
		clt, err := newS3Client(ctx, profile, region)
		if err != nil {
			return err
		}
		return listBuckets(ctx, clt, cmd.OutOrStdout())
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
	lsCmd.Flags().String("name", "", "Name of the bucket.")
	lsCmd.Flags().String("profile", "", "AWS Profile.")
	lsCmd.Flags().String("region", "", "AWS Region.")
}
