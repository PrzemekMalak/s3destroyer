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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newDestroyCommand(bucketClient func(context.Context, string, string, string) (s3API, string, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Emptying a bucket and deleting it",
		Long: `Emptying a bucket and deleting it.

To destroy a bucket you must provide a name of the bucket:
s3destroyer destroy --name bucketname

You will be asked to type the bucket name to confirm. To skip the confirmation
(e.g. in scripts) add --yes. To only see what would be deleted add --dry-run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			profile, _ := cmd.Flags().GetString("profile")
			region, _ := cmd.Flags().GetString("region")
			yes, _ := cmd.Flags().GetBool("yes")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			ctx := cmd.Context()
			clt, bucketRegion, err := bucketClient(ctx, profile, region, name)
			if err != nil {
				return err
			}

			stats, err := countBucketContents(ctx, clt, name)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Bucket %s (%s) contains %d object version(s) and %d delete marker(s).\n",
				name, bucketRegion, stats.Versions, stats.DeleteMarkers)

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run: nothing was deleted.")
				return nil
			}

			if !yes {
				if err := confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), name); err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Destroying bucket: %s\n", name)
			if err := removeBucket(ctx, clt, name, cmd.OutOrStdout()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Bucket %s destroyed.\n", name)
			return nil
		},
	}

	cmd.Flags().String("name", "", "Name of the bucket.")
	cmd.Flags().String("profile", "", "AWS Profile.")
	cmd.Flags().String("region", "", "AWS Region.")
	cmd.Flags().BoolP("yes", "y", false, "Skip the confirmation prompt.")
	cmd.Flags().Bool("dry-run", false, "Only show what would be deleted.")
	cmd.MarkFlagRequired("name")
	return cmd
}

// confirm asks the user to type the bucket name and fails unless it matches exactly.
func confirm(in io.Reader, prompt io.Writer, bucketName string) error {
	fmt.Fprintf(prompt, "This will permanently delete bucket %q and ALL of its contents.\n", bucketName)
	fmt.Fprint(prompt, "Type the bucket name to confirm: ")

	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	if strings.TrimSpace(answer) != bucketName {
		return errors.New("confirmation did not match the bucket name, aborting")
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newDestroyCommand(newBucketClient))
}
