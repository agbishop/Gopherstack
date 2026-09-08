// Command pgoload is a heavy, concurrent load generator for a running
// Gopherstack server. It exists to capture a representative CPU profile for
// Profile-Guided Optimization (PGO): it drives DynamoDB (on a table with both
// a GSI and an LSI) and S3 hard and concurrently, so the profile reflects the
// real hot paths — index maintenance, item marshaling, lockmetrics map
// operations, and S3 storage/listing.
//
// Usage:
//
//	pgoload -endpoint http://localhost:8000 -duration 90s -concurrency 8
//
// Flags fall back to environment variables (PGOLOAD_ENDPOINT,
// PGOLOAD_DURATION, PGOLOAD_CONCURRENCY) when unset.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

const (
	defaultEndpoint    = "http://localhost:8000"
	defaultDuration    = 90 * time.Second
	defaultConcurrency = 8

	envEndpoint    = "PGOLOAD_ENDPOINT"
	envDuration    = "PGOLOAD_DURATION"
	envConcurrency = "PGOLOAD_CONCURRENCY"

	awsRegion = "us-east-1"

	// setupTimeout bounds table/bucket provisioning, kept separate from the
	// load duration so a short -duration still gets a full run.
	setupTimeout = 60 * time.Second
	pollInterval = 500 * time.Millisecond
)

const (
	ddbTableName = "pgoload-ddb"
	ddbGSIName   = "GSI1"
	ddbLSIName   = "LSI1"

	attrPK     = "PK"
	attrSK     = "SK"
	attrGSI1PK = "GSI1PK"
	attrLSISK  = "LSISK"
	attrWorker = "WorkerID"
	attrData   = "Data"

	// itemsPerPartition items share a PK before the partition rolls over,
	// giving Query/LSI-range operations a non-trivial result set.
	itemsPerPartition = 20
	// gsiBucketCount is how many distinct GSI1PK values are in rotation, so
	// GSI1 queries also return multiple items per key.
	gsiBucketCount = 6
	lsiRange       = 40
	batchWriteSize = 25

	transactMinItems = 2
	transactMaxItems = 4
	transactRange    = transactMaxItems - transactMinItems + 1

	ddbSmallDataSize  = 128
	ddbMediumDataSize = 4096
	// ddbDataSizeMod: every ddbDataSizeMod'th item gets the larger payload.
	ddbDataSizeMod = 3

	ddbQueryLimit = 25
	ddbScanLimit  = 50
)

const (
	s3BucketCount        = 3
	s3VersionedBucketIdx = 0
	s3KeysPerPrefix      = 30

	s3SmallBodySize  = 512
	s3MediumBodySize = 64 * 1024
	// s3BodySizeMod: every s3BodySizeMod'th object gets the larger payload.
	s3BodySizeMod = 4

	s3ListBaseKeys = 10
	s3ListKeyRange = 20

	// multipartFirstPartSize must be >= 5 MiB: S3 rejects non-final parts
	// smaller than that with EntityTooSmall.
	multipartFirstPartSize  = 5 * 1024 * 1024
	multipartSecondPartSize = 1024

	multipartPartOne = 1
	multipartPartTwo = 2
)

// errTableNotActive is returned when the DynamoDB table fails to reach the
// ACTIVE status within setupTimeout.
var errTableNotActive = errors.New("table did not become active in time")

// config holds the resolved CLI flags for a pgoload run.
type config struct {
	endpoint    string
	duration    time.Duration
	concurrency int
}

// counters tracks operation and error totals across every worker goroutine.
// ddb/s3 keep their original flat fields untouched; every service added for
// breadth uses the shared opCounter type instead of four more fields each.
type counters struct {
	ddbOps    atomic.Int64
	ddbErrors atomic.Int64
	s3Ops     atomic.Int64
	s3Errors  atomic.Int64

	sqs            opCounter
	sns            opCounter
	kinesis        opCounter
	iam            opCounter
	sts            opCounter
	ssm            opCounter
	secretsmanager opCounter
	cloudwatch     opCounter
	logs           opCounter
	ec2            opCounter
	lambda         opCounter
	kms            opCounter
	eventbridge    opCounter
	stepfunctions  opCounter
}

// namedCounter pairs a scenario name with its counter, for summary printing.
type namedCounter struct {
	c    *opCounter
	name string
}

func (c *counters) breadthCounters() []namedCounter {
	return []namedCounter{
		{name: "sqs", c: &c.sqs},
		{name: "sns", c: &c.sns},
		{name: "kinesis", c: &c.kinesis},
		{name: "iam", c: &c.iam},
		{name: "sts", c: &c.sts},
		{name: "ssm", c: &c.ssm},
		{name: "secretsmanager", c: &c.secretsmanager},
		{name: "cloudwatch", c: &c.cloudwatch},
		{name: "logs", c: &c.logs},
		{name: "ec2", c: &c.ec2},
		{name: "lambda", c: &c.lambda},
		{name: "kms", c: &c.kms},
		{name: "eventbridge", c: &c.eventbridge},
		{name: "stepfunctions", c: &c.stepfunctions},
	}
}

func main() {
	os.Exit(run())
}

func run() int {
	cfg := parseFlags()
	log := logger.NewLogger(slog.LevelInfo)

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cls, err := buildClients(rootCtx, cfg)
	if err != nil {
		log.ErrorContext(rootCtx, "failed to build aws clients", "error", err)

		return 1
	}

	res, err := setupResources(rootCtx, cls, log)
	if err != nil {
		log.ErrorContext(rootCtx, "failed to prepare load resources", "error", err)

		return 1
	}

	return runLoad(rootCtx, cfg, cls, res, log)
}

// parseFlags resolves CLI flags, falling back to environment variables and
// then hard-coded defaults.
func parseFlags() config {
	endpoint := flag.String(
		"endpoint",
		envOrDefault(envEndpoint, defaultEndpoint),
		"gopherstack server endpoint URL",
	)
	duration := flag.Duration(
		"duration",
		envDurationOrDefault(envDuration, defaultDuration),
		"how long to run the load",
	)
	concurrency := flag.Int(
		"concurrency",
		envIntOrDefault(envConcurrency, defaultConcurrency),
		"worker goroutines per scenario",
	)

	flag.Parse()

	return config{endpoint: *endpoint, duration: *duration, concurrency: *concurrency}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func envDurationOrDefault(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}

	return def
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}

	return def
}

// runLoad fans out cfg.concurrency workers for each scenario and lets them
// run until cfg.duration elapses (or ctx is canceled), then prints a summary.
func runLoad(ctx context.Context, cfg config, cls *clients, res *resources, log *slog.Logger) int {
	loadCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()

	c := &counters{}

	var wg sync.WaitGroup

	spawn := func(n int, worker func(id int)) {
		for w := range n {
			wg.Add(1)

			go func(id int) {
				defer wg.Done()
				worker(id)
			}(w)
		}
	}

	spawn(cfg.concurrency, func(id int) { ddbWorker(loadCtx, cls.ddb, id, c, log) })
	spawn(cfg.concurrency, func(id int) { s3Worker(loadCtx, cls.s3, res.s3Buckets, id, c, log) })
	spawn(cfg.concurrency, func(id int) { sqsWorker(loadCtx, cls.sqs, res, id, &c.sqs, log) })
	spawn(cfg.concurrency, func(id int) { snsWorker(loadCtx, cls.sns, res, id, &c.sns, log) })
	spawn(cfg.concurrency, func(id int) { kinesisWorker(loadCtx, cls.kinesis, res, id, &c.kinesis, log) })
	spawn(cfg.concurrency, func(id int) { iamWorker(loadCtx, cls.iam, id, &c.iam, log) })
	spawn(cfg.concurrency, func(id int) { stsWorker(loadCtx, cls.sts, res, id, &c.sts, log) })
	spawn(cfg.concurrency, func(id int) { ssmWorker(loadCtx, cls.ssm, id, &c.ssm, log) })
	spawn(cfg.concurrency, func(id int) {
		secretsManagerWorker(loadCtx, cls.secretsmanager, id, &c.secretsmanager, log)
	})
	spawn(cfg.concurrency, func(id int) { cloudwatchWorker(loadCtx, cls.cloudwatch, id, &c.cloudwatch, log) })
	spawn(cfg.concurrency, func(id int) { logsWorker(loadCtx, cls.logs, id, &c.logs, log) })
	spawn(cfg.concurrency, func(id int) { ec2Worker(loadCtx, cls.ec2, id, &c.ec2, log) })
	spawn(cfg.concurrency, func(id int) { lambdaWorker(loadCtx, cls.lambda, res, id, &c.lambda, log) })
	spawn(cfg.concurrency, func(id int) { kmsWorker(loadCtx, cls.kms, res, id, &c.kms, log) })
	spawn(cfg.concurrency, func(id int) { eventBridgeWorker(loadCtx, cls.eventbridge, id, &c.eventbridge, log) })
	spawn(cfg.concurrency, func(id int) {
		stepFunctionsWorker(loadCtx, cls.stepfunctions, res, id, &c.stepfunctions, log)
	})

	wg.Wait()
	printSummary(c)

	return 0
}

func printSummary(c *counters) {
	fmt.Fprintf(
		os.Stdout,
		"ddb ops=%d, s3 ops=%d, errors=%d\n",
		c.ddbOps.Load(),
		c.s3Ops.Load(),
		c.ddbErrors.Load()+c.s3Errors.Load(),
	)

	for _, nc := range c.breadthCounters() {
		fmt.Fprintf(os.Stdout, "%s ops=%d, errors=%d\n", nc.name, nc.c.ops.Load(), nc.c.errors.Load())
	}
}

// ---------------------------------------------------------------------------
// DynamoDB scenario
// ---------------------------------------------------------------------------

// ensureDDBTable creates ddbTableName with a GSI and an LSI, tolerating a
// table that already exists, then waits for it to become ACTIVE.
func ensureDDBTable(ctx context.Context, cl *dynamodb.Client, log *slog.Logger) error {
	_, err := cl.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(ddbTableName),
		BillingMode: ddbtypes.BillingModePayPerRequest,
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String(attrPK), KeyType: ddbtypes.KeyTypeHash},
			{AttributeName: aws.String(attrSK), KeyType: ddbtypes.KeyTypeRange},
		},
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String(attrPK), AttributeType: ddbtypes.ScalarAttributeTypeS},
			{AttributeName: aws.String(attrSK), AttributeType: ddbtypes.ScalarAttributeTypeS},
			{AttributeName: aws.String(attrGSI1PK), AttributeType: ddbtypes.ScalarAttributeTypeS},
			{AttributeName: aws.String(attrLSISK), AttributeType: ddbtypes.ScalarAttributeTypeN},
		},
		GlobalSecondaryIndexes: []ddbtypes.GlobalSecondaryIndex{
			{
				IndexName: aws.String(ddbGSIName),
				KeySchema: []ddbtypes.KeySchemaElement{
					{AttributeName: aws.String(attrGSI1PK), KeyType: ddbtypes.KeyTypeHash},
				},
				Projection: &ddbtypes.Projection{ProjectionType: ddbtypes.ProjectionTypeAll},
			},
		},
		LocalSecondaryIndexes: []ddbtypes.LocalSecondaryIndex{
			{
				IndexName: aws.String(ddbLSIName),
				KeySchema: []ddbtypes.KeySchemaElement{
					{AttributeName: aws.String(attrPK), KeyType: ddbtypes.KeyTypeHash},
					{AttributeName: aws.String(attrLSISK), KeyType: ddbtypes.KeyTypeRange},
				},
				Projection: &ddbtypes.Projection{ProjectionType: ddbtypes.ProjectionTypeAll},
			},
		},
	})
	if err != nil {
		if _, ok := errors.AsType[*ddbtypes.ResourceInUseException](err); !ok {
			return fmt.Errorf("create table %s: %w", ddbTableName, err)
		}

		log.InfoContext(ctx, "ddb table already exists", "table", ddbTableName)
	}

	return waitTableActive(ctx, cl, log)
}

func waitTableActive(ctx context.Context, cl *dynamodb.Client, log *slog.Logger) error {
	deadline := time.Now().Add(setupTimeout)

	for time.Now().Before(deadline) {
		out, err := cl.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(ddbTableName)})
		if err == nil && out.Table != nil && out.Table.TableStatus == ddbtypes.TableStatusActive {
			return nil
		}

		t := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			t.Stop()

			return fmt.Errorf("waiting for table %s: %w", ddbTableName, ctx.Err())
		case <-t.C:
		}
	}

	log.WarnContext(ctx, "gave up waiting for table to become active", "table", ddbTableName)

	return fmt.Errorf("%w: table=%s timeout=%s", errTableNotActive, ddbTableName, setupTimeout)
}

// ddbWorker repeatedly runs a mix of DynamoDB operations, staggered by
// workerID so concurrent workers hit different operations on each tick, until
// ctx is done.
func ddbWorker(ctx context.Context, cl *dynamodb.Client, workerID int, c *counters, log *slog.Logger) {
	ops := ddbOpTable()

	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		op := ops[(workerID+i)%len(ops)]
		if err := op(ctx, cl, workerID, i); err != nil {
			c.ddbErrors.Add(1)
			log.WarnContext(ctx, "ddb operation failed", "worker", workerID, "iter", i, "error", err)

			continue
		}

		c.ddbOps.Add(1)
	}
}

// ddbOpFunc is one DynamoDB operation exercised by ddbWorker. workerID and i
// together select the key space touched by the call.
type ddbOpFunc func(ctx context.Context, cl *dynamodb.Client, workerID, i int) error

// ddbOpTable returns the operation mix for the DynamoDB scenario. Puts,
// queries and updates are weighted heaviest; batch writes and transactions
// are rarer but still exercised on every cycle through the table.
func ddbOpTable() []ddbOpFunc {
	return []ddbOpFunc{
		ddbPutItemOp, ddbPutItemOp, ddbPutItemOp,
		ddbBatchWriteOp,
		ddbQueryBaseOp, ddbQueryBaseOp,
		ddbQueryGSIOp, ddbQueryGSIOp,
		ddbQueryLSIOp,
		ddbScanOp,
		ddbUpdateItemOp, ddbUpdateItemOp,
		ddbDeleteItemOp,
		ddbTransactWriteOp,
	}
}

func ddbPK(workerID, i int) string {
	return fmt.Sprintf("W%d#P%d", workerID, i/itemsPerPartition)
}

func ddbSK(i int) string {
	return fmt.Sprintf("S%05d", i%itemsPerPartition)
}

func ddbGSI1PK(workerID, i int) string {
	return fmt.Sprintf("G%d", (workerID*itemsPerPartition+i)%gsiBucketCount)
}

func ddbLSISK(i int) int {
	return i % lsiRange
}

func ddbDataPayload(i int) string {
	size := ddbSmallDataSize
	if i%ddbDataSizeMod == 0 {
		size = ddbMediumDataSize
	}

	return strings.Repeat("d", size)
}

func ddbBuildItem(workerID, i int) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{
		attrPK:     &ddbtypes.AttributeValueMemberS{Value: ddbPK(workerID, i)},
		attrSK:     &ddbtypes.AttributeValueMemberS{Value: ddbSK(i)},
		attrGSI1PK: &ddbtypes.AttributeValueMemberS{Value: ddbGSI1PK(workerID, i)},
		attrLSISK:  &ddbtypes.AttributeValueMemberN{Value: strconv.Itoa(ddbLSISK(i))},
		attrWorker: &ddbtypes.AttributeValueMemberN{Value: strconv.Itoa(workerID)},
		attrData:   &ddbtypes.AttributeValueMemberS{Value: ddbDataPayload(i)},
	}
}

func ddbPutItemOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	_, err := cl.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(ddbTableName),
		Item:      ddbBuildItem(workerID, i),
	})

	return err
}

func ddbBatchWriteOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	base := i * batchWriteSize
	reqs := make([]ddbtypes.WriteRequest, 0, batchWriteSize)

	for j := range batchWriteSize {
		reqs = append(reqs, ddbtypes.WriteRequest{
			PutRequest: &ddbtypes.PutRequest{Item: ddbBuildItem(workerID, base+j)},
		})
	}

	_, err := cl.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]ddbtypes.WriteRequest{ddbTableName: reqs},
	})

	return err
}

func ddbQueryBaseOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	_, err := cl.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(ddbTableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":pk": &ddbtypes.AttributeValueMemberS{Value: ddbPK(workerID, i)},
		},
		Limit: aws.Int32(ddbQueryLimit),
	})

	return err
}

func ddbQueryGSIOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	_, err := cl.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(ddbTableName),
		IndexName:              aws.String(ddbGSIName),
		KeyConditionExpression: aws.String("GSI1PK = :g"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":g": &ddbtypes.AttributeValueMemberS{Value: ddbGSI1PK(workerID, i)},
		},
		Limit: aws.Int32(ddbQueryLimit),
	})

	return err
}

func ddbQueryLSIOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	_, err := cl.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(ddbTableName),
		IndexName:              aws.String(ddbLSIName),
		KeyConditionExpression: aws.String("PK = :pk AND LSISK BETWEEN :lo AND :hi"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":pk": &ddbtypes.AttributeValueMemberS{Value: ddbPK(workerID, i)},
			":lo": &ddbtypes.AttributeValueMemberN{Value: "0"},
			":hi": &ddbtypes.AttributeValueMemberN{Value: strconv.Itoa(lsiRange)},
		},
		Limit: aws.Int32(ddbQueryLimit),
	})

	return err
}

func ddbScanOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	limit := int32(ddbScanLimit + i%ddbQueryLimit)

	_, err := cl.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(ddbTableName),
		FilterExpression: aws.String("WorkerID = :w"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":w": &ddbtypes.AttributeValueMemberN{Value: strconv.Itoa(workerID)},
		},
		Limit: aws.Int32(limit),
	})

	return err
}

func ddbUpdateItemOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	_, err := cl.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(ddbTableName),
		Key: map[string]ddbtypes.AttributeValue{
			attrPK: &ddbtypes.AttributeValueMemberS{Value: ddbPK(workerID, i)},
			attrSK: &ddbtypes.AttributeValueMemberS{Value: ddbSK(i)},
		},
		UpdateExpression: aws.String("SET Data = :d ADD HitCount :inc"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":d":   &ddbtypes.AttributeValueMemberS{Value: ddbDataPayload(i)},
			":inc": &ddbtypes.AttributeValueMemberN{Value: "1"},
		},
	})

	return err
}

func ddbDeleteItemOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	_, err := cl.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(ddbTableName),
		Key: map[string]ddbtypes.AttributeValue{
			attrPK: &ddbtypes.AttributeValueMemberS{Value: ddbPK(workerID, i)},
			attrSK: &ddbtypes.AttributeValueMemberS{Value: ddbSK(i)},
		},
	})

	return err
}

func ddbTransactWriteOp(ctx context.Context, cl *dynamodb.Client, workerID, i int) error {
	n := transactMinItems + i%transactRange
	base := i * transactMaxItems
	items := make([]ddbtypes.TransactWriteItem, 0, n)

	for j := range n {
		items = append(items, ddbtypes.TransactWriteItem{
			Put: &ddbtypes.Put{
				TableName: aws.String(ddbTableName),
				Item:      ddbBuildItem(workerID, base+j),
			},
		})
	}

	_, err := cl.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})

	return err
}

// ---------------------------------------------------------------------------
// S3 scenario
// ---------------------------------------------------------------------------

func s3BucketNames() []string {
	names := make([]string, 0, s3BucketCount)
	for i := range s3BucketCount {
		names = append(names, fmt.Sprintf("pgoload-s3-%d", i))
	}

	return names
}

// ensureS3Buckets creates the load-test buckets, enabling versioning on the
// bucket at s3VersionedBucketIdx.
func ensureS3Buckets(ctx context.Context, cl *s3.Client, log *slog.Logger) ([]string, error) {
	buckets := s3BucketNames()

	for idx, b := range buckets {
		if err := ensureS3Bucket(ctx, cl, b); err != nil {
			return nil, err
		}

		if idx != s3VersionedBucketIdx {
			continue
		}

		if _, err := cl.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
			Bucket: aws.String(b),
			VersioningConfiguration: &s3types.VersioningConfiguration{
				Status: s3types.BucketVersioningStatusEnabled,
			},
		}); err != nil {
			log.WarnContext(ctx, "enable bucket versioning failed", "bucket", b, "error", err)
		}
	}

	return buckets, nil
}

func ensureS3Bucket(ctx context.Context, cl *s3.Client, bucket string) error {
	_, err := cl.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err == nil || isBucketAlreadyOwned(err) {
		return nil
	}

	return fmt.Errorf("create bucket %s: %w", bucket, err)
}

func isBucketAlreadyOwned(err error) bool {
	return strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") ||
		strings.Contains(err.Error(), "BucketAlreadyExists")
}

// s3Worker repeatedly runs a mix of S3 operations, staggered by workerID,
// until ctx is done.
func s3Worker(ctx context.Context, cl *s3.Client, buckets []string, workerID int, c *counters, log *slog.Logger) {
	ops := s3OpTable()

	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		op := ops[(workerID+i)%len(ops)]
		if err := op(ctx, cl, buckets, workerID, i); err != nil {
			c.s3Errors.Add(1)
			log.WarnContext(ctx, "s3 operation failed", "worker", workerID, "iter", i, "error", err)

			continue
		}

		c.s3Ops.Add(1)
	}
}

// s3OpFunc is one S3 operation exercised by s3Worker. workerID and i together
// select the bucket and key touched by the call.
type s3OpFunc func(ctx context.Context, cl *s3.Client, buckets []string, workerID, i int) error

// s3OpTable returns the operation mix for the S3 scenario. Puts, gets and
// listings dominate; multipart uploads are exercised periodically.
func s3OpTable() []s3OpFunc {
	return []s3OpFunc{
		s3PutObjectOp, s3PutObjectOp, s3PutObjectOp,
		s3GetObjectOp, s3GetObjectOp,
		s3ListObjectsOp,
		s3ListVersionsOp,
		s3DeleteObjectOp,
		s3MultipartUploadOp,
	}
}

func s3Key(workerID, i int) string {
	return fmt.Sprintf("prefix-%d/obj-%05d.bin", workerID, i%s3KeysPerPrefix)
}

func s3Bucket(buckets []string, workerID, i int) string {
	return buckets[(workerID+i)%len(buckets)]
}

func s3Payload(i int) string {
	size := s3SmallBodySize
	if i%s3BodySizeMod == 0 {
		size = s3MediumBodySize
	}

	return strings.Repeat("s", size)
}

func s3PutObjectOp(ctx context.Context, cl *s3.Client, buckets []string, workerID, i int) error {
	_, err := cl.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s3Bucket(buckets, workerID, i)),
		Key:         aws.String(s3Key(workerID, i)),
		Body:        strings.NewReader(s3Payload(i)),
		ContentType: aws.String("application/octet-stream"),
	})

	return err
}

func s3GetObjectOp(ctx context.Context, cl *s3.Client, buckets []string, workerID, i int) error {
	out, err := cl.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3Bucket(buckets, workerID, i)),
		Key:    aws.String(s3Key(workerID, i)),
	})
	if err != nil {
		return err
	}
	defer func() { _ = out.Body.Close() }()

	if _, copyErr := io.Copy(io.Discard, out.Body); copyErr != nil {
		return fmt.Errorf("read object body: %w", copyErr)
	}

	return nil
}

func s3DeleteObjectOp(ctx context.Context, cl *s3.Client, buckets []string, workerID, i int) error {
	_, err := cl.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s3Bucket(buckets, workerID, i)),
		Key:    aws.String(s3Key(workerID, i)),
	})

	return err
}

func s3ListObjectsOp(ctx context.Context, cl *s3.Client, buckets []string, workerID, i int) error {
	maxKeys := int32(s3ListBaseKeys + i%s3ListKeyRange)

	_, err := cl.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s3Bucket(buckets, workerID, i)),
		Prefix:  aws.String(fmt.Sprintf("prefix-%d/", workerID)),
		MaxKeys: aws.Int32(maxKeys),
	})

	return err
}

func s3ListVersionsOp(ctx context.Context, cl *s3.Client, buckets []string, workerID, i int) error {
	maxKeys := int32(s3ListBaseKeys + i%s3ListKeyRange)

	_, err := cl.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket:  aws.String(buckets[s3VersionedBucketIdx]),
		Prefix:  aws.String(fmt.Sprintf("prefix-%d/", workerID)),
		MaxKeys: aws.Int32(maxKeys),
	})

	return err
}

// s3MultipartUploadOp drives CreateMultipartUpload -> UploadPart x2 ->
// CompleteMultipartUpload. The first part must be >= 5 MiB or S3 rejects the
// completion with EntityTooSmall, so it is sized accordingly.
func s3MultipartUploadOp(ctx context.Context, cl *s3.Client, buckets []string, workerID, i int) error {
	bucket := s3Bucket(buckets, workerID, i)
	key := fmt.Sprintf("multipart-%d/%05d.bin", workerID, i)

	created, err := cl.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("create multipart upload: %w", err)
	}

	part1, err := cl.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		PartNumber: aws.Int32(multipartPartOne),
		UploadId:   created.UploadId,
		Body:       strings.NewReader(strings.Repeat("a", multipartFirstPartSize)),
	})
	if err != nil {
		return fmt.Errorf("upload part 1: %w", err)
	}

	part2, err := cl.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		PartNumber: aws.Int32(multipartPartTwo),
		UploadId:   created.UploadId,
		Body:       strings.NewReader(strings.Repeat("b", multipartSecondPartSize)),
	})
	if err != nil {
		return fmt.Errorf("upload part 2: %w", err)
	}

	_, err = cl.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: created.UploadId,
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: []s3types.CompletedPart{
				{ETag: part1.ETag, PartNumber: aws.Int32(multipartPartOne)},
				{ETag: part2.ETag, PartNumber: aws.Int32(multipartPartTwo)},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}

	return nil
}
