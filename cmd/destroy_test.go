package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"exact match", "my-bucket\n", false},
		{"surrounding whitespace", "  my-bucket  \n", false},
		{"no trailing newline", "my-bucket", false},
		{"windows line ending", "my-bucket\r\n", false},
		{"different name", "other-bucket\n", true},
		{"prefix only", "my-buck\n", true},
		{"yes is not enough", "yes\n", true},
		{"empty input", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := confirm(strings.NewReader(tt.input), io.Discard, "my-bucket")
			if (err != nil) != tt.wantErr {
				t.Fatalf("confirm(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestDestroyCommand(t *testing.T) {
	tests := []struct {
		name        string
		flags       []string
		input       string
		wantError   string
		wantDeleted bool
		wantPrompt  bool
	}{
		{name: "dry run takes precedence over yes", flags: []string{"--dry-run", "--yes"}},
		{name: "wrong confirmation", input: "other-bucket\n", wantError: "confirmation did not match", wantPrompt: true},
		{name: "correct confirmation", input: testBucket + "\n", wantDeleted: true, wantPrompt: true},
		{name: "yes skips confirmation", flags: []string{"--yes"}, wantDeleted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeS3(false)
			f.put("example")
			cmd := newDestroyCommand(func(_ context.Context, profile, region, name string) (s3API, string, error) {
				if profile != "test-profile" || region != "eu-west-1" || name != testBucket {
					t.Fatalf("unexpected client arguments: %q, %q, %q", profile, region, name)
				}
				return f, region, nil
			})
			var out, prompts bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&prompts)
			cmd.SetIn(strings.NewReader(tt.input))
			cmd.SetArgs(append([]string{"--name", testBucket, "--profile", "test-profile", "--region", "eu-west-1"}, tt.flags...))

			err := cmd.Execute()
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("got %v, want error containing %q", err, tt.wantError)
			}
			if !strings.Contains(out.String(), "contains 1 object version(s) and 0 delete marker(s)") {
				t.Errorf("missing preview: %q", out.String())
			}
			if got := strings.Contains(prompts.String(), "Type the bucket name"); got != tt.wantPrompt {
				t.Errorf("prompt shown = %v, want %v", got, tt.wantPrompt)
			}
			if f.bucketDeleted != tt.wantDeleted {
				t.Errorf("bucket deleted = %v, want %v", f.bucketDeleted, tt.wantDeleted)
			}
			if !tt.wantDeleted && (len(f.deleteBatches) != 0 || f.deleteBucketCalls != 0) {
				t.Error("unexpected deletion request")
			}
		})
	}
}
