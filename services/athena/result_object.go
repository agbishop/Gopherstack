package athena

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	sdk_s3_types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	encryptionOptionSSES3  = "SSE_S3"
	encryptionOptionSSEKMS = "SSE_KMS"
)

// writeResultObject writes a succeeded query execution's result to
// ResultConfiguration.OutputLocation as a CSV object, when S3 is wired and an
// output location is set. A no-op otherwise, matching this repo's
// unwired-hook-stays-permissive convention: results remain stored/echoed via
// GetQueryExecution/GetQueryResults regardless.
//
// Real AWS Athena writes a result object per query execution under
// OutputLocation, keyed by QueryExecutionId; the pinned SDK
// (types.ResultConfiguration.OutputLocation) documents only the directory,
// not an object key or file format. gopherstack approximates both: the key
// is "<prefix>/<QueryExecutionId>.csv", and the body is a plain CSV (header
// row of column names, then one row per result row) via encoding/csv --
// this is a disclosed approximation, not a verified wire shape.
func (b *InMemoryBackend) writeResultObject(id string, rc ResultConfiguration, result *sqlResult) {
	if b.s3 == nil || rc.OutputLocation == "" {
		return
	}

	bucket, key, ok := resultObjectKey(rc.OutputLocation, id)
	if !ok {
		return
	}

	input := &sdk_s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(resultCSV(result)),
	}

	switch rc.EncryptionConfiguration.EncryptionOption {
	case encryptionOptionSSES3:
		input.ServerSideEncryption = sdk_s3_types.ServerSideEncryptionAes256
	case encryptionOptionSSEKMS:
		input.ServerSideEncryption = sdk_s3_types.ServerSideEncryptionAwsKms
		if rc.EncryptionConfiguration.KmsKey != "" {
			input.SSEKMSKeyId = aws.String(rc.EncryptionConfiguration.KmsKey)
		}
	}

	_, _ = b.s3.PutObject(context.Background(), input)
}

// resultObjectKey splits an "s3://bucket/prefix" OutputLocation into a bucket
// and the object key "<prefix>/<id>.csv" (no prefix: just "<id>.csv").
func resultObjectKey(outputLocation, id string) (string, string, bool) {
	const s3Scheme = "s3://"
	if !strings.HasPrefix(outputLocation, s3Scheme) {
		return "", "", false
	}

	bucket, prefix, _ := strings.Cut(strings.TrimPrefix(outputLocation, s3Scheme), "/")
	if bucket == "" {
		return "", "", false
	}

	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}

	return bucket, prefix + id + ".csv", true
}

// resultCSV serializes a query result to CSV: a header row of column names,
// then one row per result row. A nil result (e.g. a DDL statement with no
// result set) still produces a valid, empty CSV.
func resultCSV(result *sqlResult) []byte {
	var buf bytes.Buffer

	w := csv.NewWriter(&buf)

	if result != nil {
		header := make([]string, len(result.columns))
		for i, c := range result.columns {
			header[i] = c.name
		}

		_ = w.Write(header)

		for _, row := range result.rows {
			_ = w.Write(row)
		}
	}

	w.Flush()

	return buf.Bytes()
}
