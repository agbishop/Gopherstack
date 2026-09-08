package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPurgeContainer starts a Gopherstack container with AUTO_PURGE_TTL set.
func startPurgeContainer(t *testing.T, ttl string) (testcontainers.Container, string) {
	t.Helper()
	ctx := t.Context()

	dockerfile, err := dockerfileFor()
	require.NoError(t, err)

	req := testcontainers.ContainerRequest{
		Context:    "../../",
		Dockerfile: dockerfile,
		Env: map[string]string{
			"AUTO_PURGE_TTL": ttl,
		},
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor: wait.ForHTTP("/_gopherstack/health").
			WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_ = container.Terminate(cleanupCtx)
	})

	mappedPort, err := container.MappedPort(ctx, "8000")
	require.NoError(t, err)

	return container, "http://localhost:" + mappedPort.Port()
}

// purgeClients bundles the per-service SDK clients used by the auto-purge test.
type purgeClients struct {
	s3     *s3.Client
	ddb    *dynamodb.Client
	sqs    *sqs.Client
	sns    *sns.Client
	iam    *iam.Client
	lambda *lambda.Client
}

// newPurgeClients builds SDK clients for every service exercised by the
// auto-purge test, all pointed at the given Gopherstack endpoint.
func newPurgeClients(t *testing.T, ep string) purgeClients {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(
		t.Context(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	return purgeClients{
		s3: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(ep)
			o.UsePathStyle = true
		}),
		ddb: dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(ep)
		}),
		sqs: sqs.NewFromConfig(cfg, func(o *sqs.Options) {
			o.BaseEndpoint = aws.String(ep)
		}),
		sns: sns.NewFromConfig(cfg, func(o *sns.Options) {
			o.BaseEndpoint = aws.String(ep)
		}),
		iam: iam.NewFromConfig(cfg, func(o *iam.Options) {
			o.BaseEndpoint = aws.String(ep)
		}),
		lambda: lambda.NewFromConfig(cfg, func(o *lambda.Options) {
			o.BaseEndpoint = aws.String(ep)
		}),
	}
}

// purgeResourceNames holds the names of the one-of-each-service resource set
// created for a given prefix ("old" or "new").
type purgeResourceNames struct {
	bucket   string
	table    string
	queue    string
	topic    string
	user     string
	function string
}

func purgeNames(prefix string) purgeResourceNames {
	return purgeResourceNames{
		bucket:   prefix + "-bucket",
		table:    prefix + "-table",
		queue:    prefix + "-queue",
		topic:    prefix + "-topic",
		user:     prefix + "-user",
		function: prefix + "-func",
	}
}

// seedPurgeResources creates one S3 bucket, DynamoDB table, SQS queue, SNS
// topic, IAM user, and Lambda function, all named with the given prefix.
func seedPurgeResources(ctx context.Context, t *testing.T, c purgeClients, prefix string) purgeResourceNames {
	t.Helper()

	names := purgeNames(prefix)

	_, err := c.s3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &names.bucket})
	require.NoError(t, err)

	_, err = c.ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: &names.table,
		AttributeDefinitions: []ddbtypes.AttributeDefinition{{
			AttributeName: aws.String("id"),
			AttributeType: ddbtypes.ScalarAttributeTypeS,
		}},
		KeySchema: []ddbtypes.KeySchemaElement{{
			AttributeName: aws.String("id"),
			KeyType:       ddbtypes.KeyTypeHash,
		}},
		ProvisionedThroughput: &ddbtypes.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
	})
	require.NoError(t, err)

	_, err = c.sqs.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: &names.queue})
	require.NoError(t, err)

	_, err = c.sns.CreateTopic(ctx, &sns.CreateTopicInput{Name: &names.topic})
	require.NoError(t, err)

	_, err = c.iam.CreateUser(ctx, &iam.CreateUserInput{UserName: &names.user})
	require.NoError(t, err)

	_, err = c.lambda.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: &names.function,
		Role:         aws.String("arn:aws:iam::000000000000:role/dummy"),
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String("dummy")},
		PackageType:  lambdatypes.PackageTypeImage,
	})
	require.NoError(t, err)

	return names
}

// waitForOldBucketPurge polls ListBuckets until bucketOld is no longer present
// or the deadline is reached (the auto-purge TTL ticker fires every 20s).
func waitForOldBucketPurge(ctx context.Context, t *testing.T, c *s3.Client, bucketOld string) {
	t.Helper()

	require.Eventually(t, func() bool {
		result, listErr := c.ListBuckets(ctx, &s3.ListBucketsInput{})
		if listErr != nil {
			return false
		}

		for _, b := range result.Buckets {
			if *b.Name == bucketOld {
				return false
			}
		}

		return true
	}, 60*time.Second, 2*time.Second)
}

// assertS3Purged verifies the old bucket is gone and the new bucket remains.
func assertS3Purged(ctx context.Context, t *testing.T, c *s3.Client, bucketOld, bucketNew string) {
	t.Helper()

	buckets, err := c.ListBuckets(ctx, &s3.ListBucketsInput{})
	require.NoError(t, err)
	foundOldS3, foundNewS3 := false, false
	for _, b := range buckets.Buckets {
		if *b.Name == bucketOld {
			foundOldS3 = true
		}
		if *b.Name == bucketNew {
			foundNewS3 = true
		}
	}
	assert.False(t, foundOldS3, "old bucket should be purged")
	assert.True(t, foundNewS3, "new bucket should remain")
}

// assertDDBPurged verifies the old table is gone and the new table remains.
func assertDDBPurged(ctx context.Context, t *testing.T, c *dynamodb.Client, tableOld, tableNew string) {
	t.Helper()

	tables, err := c.ListTables(ctx, &dynamodb.ListTablesInput{})
	require.NoError(t, err)
	foundOldDDB, foundNewDDB := false, false
	for _, tName := range tables.TableNames {
		if tName == tableOld {
			foundOldDDB = true
		}
		if tName == tableNew {
			foundNewDDB = true
		}
	}
	assert.False(t, foundOldDDB, "old table should be purged")
	assert.True(t, foundNewDDB, "new table should remain")
}

// assertSQSPurged verifies the old queue is gone and the new queue remains.
func assertSQSPurged(ctx context.Context, t *testing.T, c *sqs.Client, queueOld, queueNew string) {
	t.Helper()

	queues, err := c.ListQueues(ctx, &sqs.ListQueuesInput{})
	require.NoError(t, err)
	foundOldSQS, foundNewSQS := false, false
	for _, qURL := range queues.QueueUrls {
		if strings.Contains(qURL, queueOld) {
			foundOldSQS = true
		}
		if strings.Contains(qURL, queueNew) {
			foundNewSQS = true
		}
	}
	assert.False(t, foundOldSQS, "old queue should be purged")
	assert.True(t, foundNewSQS, "new queue should remain")
}

// assertSNSPurged verifies the old topic is gone and the new topic remains.
func assertSNSPurged(ctx context.Context, t *testing.T, c *sns.Client, topicOld, topicNew string) {
	t.Helper()

	snsTopics, err := c.ListTopics(ctx, &sns.ListTopicsInput{})
	require.NoError(t, err)
	foundOldSNS, foundNewSNS := false, false
	for _, tp := range snsTopics.Topics {
		if strings.Contains(*tp.TopicArn, topicOld) {
			foundOldSNS = true
		}
		if strings.Contains(*tp.TopicArn, topicNew) {
			foundNewSNS = true
		}
	}
	assert.False(t, foundOldSNS, "old topic should be purged")
	assert.True(t, foundNewSNS, "new topic should remain")
}

// assertIAMPurged verifies the old user is gone and the new user remains.
func assertIAMPurged(ctx context.Context, t *testing.T, c *iam.Client, userOld, userNew string) {
	t.Helper()

	iamUsers, err := c.ListUsers(ctx, &iam.ListUsersInput{})
	require.NoError(t, err)
	foundOldIAM, foundNewIAM := false, false
	for _, u := range iamUsers.Users {
		if *u.UserName == userOld {
			foundOldIAM = true
		}
		if *u.UserName == userNew {
			foundNewIAM = true
		}
	}
	assert.False(t, foundOldIAM, "old user should be purged")
	assert.True(t, foundNewIAM, "new user should remain")
}

// assertLambdaPurged verifies the old function is gone and the new function remains.
func assertLambdaPurged(ctx context.Context, t *testing.T, c *lambda.Client, funcOld, funcNew string) {
	t.Helper()

	lambdaFuncs, err := c.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	require.NoError(t, err)
	foundOldLambda, foundNewLambda := false, false
	for _, f := range lambdaFuncs.Functions {
		if *f.FunctionName == funcOld {
			foundOldLambda = true
		}
		if *f.FunctionName == funcNew {
			foundNewLambda = true
		}
	}
	assert.False(t, foundOldLambda, "old function should be purged")
	assert.True(t, foundNewLambda, "new function should remain")
}

func TestIntegration_AutoPurgeTTL_SupportsGranularPurge(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 1. Start Gopherstack with 20s TTL
	_, ep := startPurgeContainer(t, "20s")
	clients := newPurgeClients(t, ep)
	ctx := t.Context()

	// 2. Create "old" resources
	oldNames := seedPurgeResources(ctx, t, clients, "old")

	// 3. Wait for TTL to pass (20s + buffer). Real wall-clock wait: no API exposes
	// "has the TTL elapsed", so this can't be turned into a poll without just
	// busy-waiting on the clock — the elapsed time itself is what's under test.
	t.Log("Waiting for resources to expire...")
	time.Sleep(22 * time.Second)

	// 4. Create "new" resources
	newNames := seedPurgeResources(ctx, t, clients, "new")

	// 5. Poll until old S3 bucket is purged or deadline is reached (TTL ticker fires every 20s).
	t.Log("Waiting for purge cycle...")
	waitForOldBucketPurge(ctx, t, clients.s3, oldNames.bucket)

	// 6. Verify old are gone, new remain
	assertS3Purged(ctx, t, clients.s3, oldNames.bucket, newNames.bucket)
	assertDDBPurged(ctx, t, clients.ddb, oldNames.table, newNames.table)
	assertSQSPurged(ctx, t, clients.sqs, oldNames.queue, newNames.queue)
	assertSNSPurged(ctx, t, clients.sns, oldNames.topic, newNames.topic)
	assertIAMPurged(ctx, t, clients.iam, oldNames.user, newNames.user)
	assertLambdaPurged(ctx, t, clients.lambda, oldNames.function, newNames.function)
}
