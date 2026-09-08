package firehose

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// redshiftRetryDuration is the default retry window for Redshift delivery.
const redshiftRetryDuration = 7200 * time.Second

const (
	redshiftBackoffInitial = 2 * time.Second
	redshiftBackoffMax     = 60 * time.Second
)

// buildRedshiftCopySQL constructs the COPY command real Firehose synthesizes after staging
// records to S3: "COPY <table> (<columns>) FROM 's3://<bucket>/<key>' CREDENTIALS
// 'aws_iam_role=<roleARN>' <copyOptions>;" (matching CopyCommand's documented fields:
// DataTableName, DataTableColumns, CopyOptions). Returns ("", false) when key is empty
// (nothing was staged).
func buildRedshiftCopySQL(tableName, columns, bucket, key, roleARN, copyOptions string) (string, bool) {
	if key == "" {
		return "", false
	}

	var b strings.Builder

	fmt.Fprintf(&b, "COPY %s", tableName)

	if columns != "" {
		fmt.Fprintf(&b, " (%s)", columns)
	}

	fmt.Fprintf(&b, " FROM 's3://%s/%s'", bucket, key)

	if roleARN != "" {
		fmt.Fprintf(&b, " CREDENTIALS 'aws_iam_role=%s'", roleARN)
	}

	if copyOptions != "" {
		fmt.Fprintf(&b, " %s", copyOptions)
	}

	b.WriteByte(';')

	return b.String(), true
}

// parseRedshiftJDBCURL extracts the cluster identifier and database name from a
// Redshift JDBC connection string of the form
// jdbc:redshift://<cluster>.<suffix>.redshift.amazonaws.com:<port>/<database>.
// Returns an error when the URL cannot be parsed or is missing a cluster or database.
func parseRedshiftJDBCURL(clusterJDBCURL string) (string, string, error) {
	jdbcURL := strings.TrimPrefix(clusterJDBCURL, "jdbc:redshift://")
	parsed, parseErr := url.Parse("https://" + jdbcURL)
	if parseErr != nil {
		return "", "", parseErr
	}

	host := parsed.Hostname()
	database := strings.TrimPrefix(parsed.Path, "/")

	// Extract cluster identifier from the host: <cluster>.<suffix>.redshift.amazonaws.com
	clusterID, _, _ := strings.Cut(host, ".")

	if clusterID == "" || database == "" {
		return "", "", fmt.Errorf("%w: JDBC URL missing cluster or database", ErrValidation)
	}

	return clusterID, database, nil
}

// executeRedshiftCopyWithRetry runs copySQL via the wired Redshift Data executor, retrying
// with exponential back-off until maxRetry elapses or ctx is cancelled. A nil executor
// (never wired, see SetRedshiftDataBackend) is a documented no-op, not a silent failure
// dressed up as success -- logged once at the start rather than retried.
func (b *InMemoryBackend) executeRedshiftCopyWithRetry(
	ctx context.Context,
	clusterID, database, dbUser, copySQL, streamARN string,
	maxRetry time.Duration,
) {
	if b.redshiftData == nil {
		logger.Load(ctx).WarnContext(ctx, "firehose: Redshift delivery skipped, no Redshift Data backend wired",
			"cluster", clusterID, "database", database, "stream", streamARN)

		return
	}

	deadline := time.Now().Add(maxRetry)
	backoff := redshiftBackoffInitial

	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for {
		execErr := b.redshiftData.ExecuteStatement(ctx, copySQL, clusterID, database, dbUser)
		if execErr == nil {
			return
		}

		if time.Now().After(deadline) {
			logger.Load(ctx).WarnContext(ctx, "firehose: Redshift delivery failed after retries",
				"cluster", clusterID, "database", database, "stream", streamARN, "error", execErr)

			return
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(backoff)

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			backoff *= 2
			if backoff > redshiftBackoffMax {
				backoff = redshiftBackoffMax
			}
		}
	}
}

// deliverToRedshift models real Firehose's two-hop Redshift delivery: records are staged to
// the destination's required S3Configuration bucket (the same location a real Redshift COPY
// reads from), then a COPY command referencing that staged object and the configured
// CopyCommand is issued via the wired Redshift Data executor. See PARITY.md: without
// SetRedshiftDataBackend wired (a cli.go change, out of this pass's scope), the COPY step is a
// documented no-op rather than a silent failure -- staging still genuinely happens.
func (b *InMemoryBackend) deliverToRedshift(
	ctx context.Context,
	records [][]byte,
	dest *RedshiftDestinationDescription,
	streamARN, streamName string,
) {
	if dest.ClusterJDBCURL == "" || dest.CopyCommand == nil || dest.CopyCommand.DataTableName == "" {
		return
	}

	if dest.S3Destination == nil || dest.S3Destination.BucketARN == "" {
		logger.Load(ctx).WarnContext(ctx, "firehose: Redshift destination missing required S3 staging configuration",
			"stream", streamARN)

		return
	}

	clusterID, database, parseErr := parseRedshiftJDBCURL(dest.ClusterJDBCURL)
	if parseErr != nil {
		logger.Load(ctx).WarnContext(ctx, "firehose: cannot parse Redshift JDBC URL",
			"url", dest.ClusterJDBCURL, "stream", streamARN, "error", parseErr)

		return
	}

	key, stageErr := b.writeRecordsToBucket(ctx, records, dest.S3Destination.BucketARN,
		dest.S3Destination.Prefix, "", dest.S3Destination.CompressionFormat, streamName)
	if stageErr != nil {
		logger.Load(ctx).WarnContext(ctx, "firehose: Redshift S3 staging failed",
			"stream", streamARN, "error", stageErr)

		return
	}

	copySQL, ok := buildRedshiftCopySQL(
		dest.CopyCommand.DataTableName, dest.CopyCommand.DataTableColumns,
		bucketFromARN(dest.S3Destination.BucketARN), key, dest.RoleARN, dest.CopyCommand.CopyOptions,
	)
	if !ok {
		return
	}

	maxRetry := redshiftRetryDuration
	if dest.RetryOptions != nil && dest.RetryOptions.DurationInSeconds > 0 {
		maxRetry = time.Duration(dest.RetryOptions.DurationInSeconds) * time.Second
	}

	b.executeRedshiftCopyWithRetry(ctx, clusterID, database, dest.Username, copySQL, streamARN, maxRetry)
}
