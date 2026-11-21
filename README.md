# s3destroyer

A simple command line application that destroys Amazon S3 buckets even if they're not empty. Before deleting a bucket, you can list buckets and objects.

## Installation

Build from source (requires Go 1.25 or newer):
```
git clone https://github.com/PrzemekMalak/s3destroyer.git
cd s3destroyer
go build -o s3destroyer .
```

## Usage

To list buckets:
```
s3destroyer ls
```

To list objects inside a bucket:
```
s3destroyer ls --name bucket-name
```

To destroy a bucket:
```
s3destroyer destroy --name bucket-name
```

Before deleting anything, `destroy` shows how many object versions and delete markers the bucket contains and asks you to type the bucket name to confirm.

To only see what would be deleted:
```
s3destroyer destroy --name bucket-name --dry-run
```

To skip the confirmation (e.g. in scripts), use `--yes` or `-y`:
```
s3destroyer destroy --name bucket-name --yes
```

You can also provide a region and a profile name using `--region` and `--profile` flags.

`s3destroyer help` can also be helpful.

## Required permissions

These permissions apply to general purpose S3 buckets. The resource examples use
the standard AWS partition.

| Permission | Used by | IAM resource |
|---|---|---|
| `s3:ListAllMyBuckets` | `ls` without `--name` | `*` |
| `s3:GetBucketLocation` | `ls --name`, `destroy`, `destroy --dry-run` | `arn:aws:s3:::bucket-name` |
| `s3:ListBucket` | `ls --name` | `arn:aws:s3:::bucket-name` |
| `s3:ListBucketVersions` | `destroy`, `destroy --dry-run` | `arn:aws:s3:::bucket-name` |
| `s3:DeleteObject` | `destroy` | `arn:aws:s3:::bucket-name/*` |
| `s3:DeleteObjectVersion` | `destroy` | `arn:aws:s3:::bucket-name/*` |
| `s3:DeleteBucket` | `destroy` | `arn:aws:s3:::bucket-name` |

`destroy` lists the bucket only with `ListObjectVersions`, the same call
`--dry-run` uses, and deletes every object version and delete marker by its
version ID. A successful dry run therefore confirms the listing access that
`destroy` needs. On top of that, `destroy` needs the delete permissions:
`DeleteObjects` always requires `s3:DeleteObject`, and deleting specific
versions also requires `s3:DeleteObjectVersion`.

[Required permissions for S3 API operations](https://docs.aws.amazon.com/AmazonS3/latest/userguide/using-with-s3-policy-actions.html)

## Limitations

Objects protected by S3 Object Lock (retention or legal hold) and buckets with MFA delete enabled cannot be emptied by s3destroyer. If S3 refuses to delete any object, s3destroyer stops, lists the objects it could not delete with the reason, and leaves the bucket in place.
