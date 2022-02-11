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
	"fmt"

	"github.com/spf13/cobra"
)

// destroyCmd represents the destroy command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Emptying a bucket and delteing it",
	Long: `Emptying a bucket and delteing it.

To destroy a bucket you must provide a name of the bucket:
s3destroyer destroy --name bucketname`,
	Run: func(cmd *cobra.Command, args []string) {

		name, _ := cmd.Flags().GetString("name")
		profile, _ := cmd.Flags().GetString("profile")
		region, _ := cmd.Flags().GetString("region")

		if len(name) > 0 {
			fmt.Printf("Destroying bucket: %s\n", name)
			destroyBucket(name, profile, region)
		} else {
			fmt.Printf("Please provide name of the bucket")
		}
	},
}

func destroyBucket(bucketName string, profile string, region string) {
	removeBucket(profile, region, bucketName)
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.Flags().String("name", "", "Name of the bucket.")
	destroyCmd.Flags().String("profile", "", "AWS Profile.")
	destroyCmd.Flags().String("region", "", "AWS Region.")
}
