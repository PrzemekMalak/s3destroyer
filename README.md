# s3destroyer

A simple command line application which destroys Amazon S3 Buckets even if they're not empty. Before deleting a bucket, you can list buckets and objects.

To list buskets:
```
s3destroyer ls
```

To list objects inside a bucket:
```
s3destroyer ls --name bucket-name
```

To destroy a bucket
```
s3destroyer destroy --name bucket-name
```

TYou can also provide a region and a profile name using ``--region`` and ``--profile`` flags.

``s3destroyer help`` can also be helpful 