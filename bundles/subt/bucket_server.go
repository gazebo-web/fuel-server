package subt

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"io"
	"time"
)

// S3Bucket is responsible of interacting with AWS S3.
// The following environment variables must be set:
// AWS_REGION
// AWS_ACCESS_KEY_ID
// AWS_SECRET_ACCESS_KEY
type S3Bucket struct {
	// From aws go documentation:
	// Sessions should be cached when possible, because creating a new Session
	// will load all configuration values from the environment, and config files
	// each time the Session is created. Sharing the Session value across all of
	// your service clients will ensure the configuration is loaded the fewest
	// number of times possible.
	sess *session.Session
	// s3 clients are safe to use concurrently.
	svc    *s3.S3
	prefix string
}

// NewS3Bucket initializes a new S3Bucket.
// arg pre is the prefix to use in Bucket names.
func NewS3Bucket(pre string) *S3Bucket {

	sess := session.Must(session.NewSession())
	// Create S3 service client (note: s3 clients are safe to use concurrently)
	svc := s3.New(sess)

	b := S3Bucket{
		sess:   sess,
		svc:    svc,
		prefix: pre,
	}

	return &b
}

// GetBucketName is an s3 implementation to get a bucket name in the cloud
func (s3b *S3Bucket) GetBucketName(bucket string) string {
	return s3b.prefix + bucket
}

// Upload is an s3 implementation to upload files to a bucket
func (s3b *S3Bucket) Upload(ctx context.Context, f io.Reader, bucket,
	fPath string) (*string, error) {
	return s3b.s3Upload(ctx, f, bucket, fPath)
}

// RemoveFile is an implementation to remove files from a bucket in S3
func (s3b *S3Bucket) RemoveFile(ctx context.Context, bucket, fPath string) error {
	return s3b.s3RemoveFile(ctx, bucket, fPath)
}

// GetPresignedURL returns presigned urls from S3 buckets.
func (s3b *S3Bucket) GetPresignedURL(ctx context.Context, bucket,
	fPath string) (*string, error) {
	return s3b.s3GetPresignedURL(ctx, bucket, fPath)
}

// s3Upload uploads a file to an S3 bucket. Creates the bucket if needed.
// Returns the URL where the object was uploaded to.
func (s3b *S3Bucket) s3Upload(ctx context.Context, f io.Reader, bucket, fPath string) (*string, error) {

	// processing form files into S3:
	// https://medium.com/@questhenkart/s3-image-uploads-via-aws-sdk-with-golang-63422857c548

	bucket = s3b.GetBucketName(bucket)

	if _, err := s3b.svc.CreateBucket(&s3.CreateBucketInput{Bucket: &bucket}); err != nil {
		if aerr, ok := err.(awserr.Error); !ok {
			return nil, err
		} else if aerr.Code() != s3.ErrCodeBucketAlreadyExists {
			return nil, err
		}
	}

	if err := s3b.svc.WaitUntilBucketExists(&s3.HeadBucketInput{Bucket: &bucket}); err != nil {
		return nil, err
	}

	// Configure CORS
	rule := s3.CORSRule{
		AllowedHeaders: aws.StringSlice([]string{"Authorization"}),
		AllowedOrigins: aws.StringSlice([]string{"*"}),
		MaxAgeSeconds:  aws.Int64(3000),
		AllowedMethods: aws.StringSlice([]string{"GET"}),
	}
	params := s3.PutBucketCorsInput{
		Bucket: aws.String(bucket),
		CORSConfiguration: &s3.CORSConfiguration{
			CORSRules: []*s3.CORSRule{&rule},
		},
	}
	if _, err := s3b.svc.PutBucketCors(&params); err != nil {
		return nil, err
	}

	// Create an uploader with S3 client and default options
	uploader := s3manager.NewUploaderWithClient(s3b.svc)
	result, err := uploader.Upload(&s3manager.UploadInput{
		Body:   f,
		Bucket: aws.String(bucket),
		Key:    aws.String(fPath),
	})
	if err != nil {
		return nil, err
	}

	return &result.Location, nil
}

// s3RemoveFile removes a file from an S3 bucket.
// Returns the URL where the object was uploaded to.
func (s3b *S3Bucket) s3RemoveFile(ctx context.Context, bucket, fPath string) error {
	bucket = s3b.GetBucketName(bucket)

	// Delete the item
	_, err := s3b.svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fPath),
	})
	if err != nil {
		return err
	}

	err = s3b.svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fPath),
	})
	if err != nil {
		return err
	}

	return nil
}

// s3GetPresignedURL returns a presign'ed URL to download a file from an S3 bucket.
func (s3b *S3Bucket) s3GetPresignedURL(ctx context.Context, bucket, fPath string) (*string, error) {

	// https://stackoverflow.com/questions/35245649/aws-s3-large-file-reverse-proxying-with-golangs-http-responsewriter
	bucket = s3b.GetBucketName(bucket)
	req, _ := s3b.svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fPath),
	})

	url, err := req.Presign(5 * time.Minute)
	if err != nil {
		return nil, err
	}
	return &url, nil
}
