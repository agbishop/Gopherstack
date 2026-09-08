package cloudtrail

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// logFileRecords is the top-level shape of a CloudTrail log file: a single
// "Records" array of individual event details. Documentation-sourced (AWS
// User Guide "CloudTrail log file examples"), not part of the pinned SDK --
// see PARITY.md.
type logFileRecords struct {
	Records []json.RawMessage `json:"Records"`
}

// deliverLogFileLocked writes ev as a CloudTrail log file to every trail
// that is currently logging and has S3BucketName set, when S3 is wired
// (SetS3Backend). A no-op when S3 is unwired or ev carries no
// CloudTrailEvent detail (e.g. a directly seeded test event bypassing
// RecordManagementEvent) -- matching this repo's unwired-hook-stays-
// permissive convention. Callers must hold b.mu.
//
// Real AWS batches multiple events per file roughly every 5 minutes; this
// backend delivers one file per recorded event instead of buffering, a
// disclosed simplification (see PARITY.md).
func (b *InMemoryBackend) deliverLogFileLocked(ev Event) {
	if b.s3 == nil || ev.CloudTrailEvent == "" {
		return
	}

	body, err := logFileBody(ev)
	if err != nil {
		return
	}

	for _, t := range b.trails.All() {
		if !t.IsLogging || t.S3BucketName == "" {
			continue
		}

		input := &sdk_s3.PutObjectInput{
			Bucket: aws.String(t.S3BucketName),
			Key:    aws.String(logFileKey(t, b.accountID, b.region, ev.EventTime)),
			Body:   bytes.NewReader(body),
		}

		if _, putErr := b.s3.PutObject(context.Background(), input); putErr == nil {
			now := time.Now().UTC()
			t.LatestDeliveryTime = &now
		}
	}
}

// logFileBody gzip-compresses a single-record CloudTrail log file body.
func logFileBody(ev Event) ([]byte, error) {
	encoded, err := json.Marshal(
		logFileRecords{Records: []json.RawMessage{json.RawMessage(ev.CloudTrailEvent)}},
	)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)

	if _, writeErr := gz.Write(encoded); writeErr != nil {
		return nil, writeErr
	}
	if closeErr := gz.Close(); closeErr != nil {
		return nil, closeErr
	}

	return buf.Bytes(), nil
}

// logFileKey builds a CloudTrail log file object key. Documentation-sourced
// (AWS User Guide "CloudTrail log file name format":
// "AWSLogs/AccountID/CloudTrail/Region/YYYY/MM/DD/AccountID_CloudTrail_
// Region_YYYYMMDDTHHmmZ_UniqueString.json.gz"), not part of the pinned SDK --
// see PARITY.md.
func logFileKey(t *Trail, accountID, region string, eventTime time.Time) string {
	ts := eventTime.UTC()
	unique := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	key := fmt.Sprintf(
		"AWSLogs/%s/CloudTrail/%s/%04d/%02d/%02d/%s_CloudTrail_%s_%s_%s.json.gz",
		accountID, region, ts.Year(), ts.Month(), ts.Day(),
		accountID, region, ts.Format("20060102T1504Z"), unique,
	)

	if t.S3KeyPrefix != "" {
		key = strings.TrimSuffix(t.S3KeyPrefix, "/") + "/" + key
	}

	return key
}
