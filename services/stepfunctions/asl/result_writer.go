package asl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// ResultWriterDetails points to a Distributed Map's exported result
// manifest.
type ResultWriterDetails struct {
	Bucket string `json:"Bucket"`
	Key    string `json:"Key"`
}

// mapExportOutput is a Distributed Map state's own output when ResultWriter
// exports to S3: the Map Run ARN plus the manifest location, replacing the
// inline results array (AWS docs: input-output-resultwriter.html, example
// under "Exporting to Amazon S3").
type mapExportOutput struct {
	MapRunArn           string              `json:"MapRunArn,omitempty"`
	ResultWriterDetails ResultWriterDetails `json:"ResultWriterDetails"`
}

type resultManifest struct {
	MapRunArn         string              `json:"MapRunArn,omitempty"`
	DestinationBucket string              `json:"DestinationBucket"`
	ResultFiles       resultManifestFiles `json:"ResultFiles"`
}

// resultManifestFiles mirrors AWS's manifest.json ResultFiles section: one
// array per status, populated only for statuses that produced a file.
// gopherstack runs every resolved item synchronously, so PENDING (items
// never scheduled because the Map Run aborted) is always empty here.
type resultManifestFiles struct {
	Succeeded []resultManifestFile `json:"SUCCEEDED"`
	Failed    []resultManifestFile `json:"FAILED"`
	Pending   []resultManifestFile `json:"PENDING"`
}

type resultManifestFile struct {
	Key  string `json:"Key"`
	Size int    `json:"Size"`
}

// mapItemRecord is one entry in a SUCCEEDED_0.json/FAILED_0.json result
// file. Real AWS records also carry a per-item ExecutionArn/Name/StartDate/
// StopDate, since each Distributed Map iteration is a real child workflow
// execution there. gopherstack runs Map iterations as in-process
// sub-executors with no separate Execution resource to point to, so those
// identity fields are omitted rather than fabricated.
type mapItemRecord struct {
	Input  any    `json:"Input"`
	Output any    `json:"Output,omitempty"`
	Error  string `json:"Error,omitempty"`
	Cause  string `json:"Cause,omitempty"`
	Status string `json:"Status"`
	Index  int    `json:"Index"`
}

const resultWriterFileIndex = 0

// exportMapResults writes a Distributed Map's per-item results plus a
// manifest to the wired S3Writer and returns ResultWriterDetails in place
// of inline results (AWS docs: input-output-resultwriter.html). It also
// returns the number of successful results actually written, for
// MapRunItemCounts.ResultsWritten.
//
// Only the Resource+Parameters(Bucket,Prefix) S3-export combination is
// supported; WriterConfig's Transformation/OutputType are parsed but not
// applied. When no S3Writer is wired, or Parameters.Bucket is unset,
// results are returned inline unchanged instead of failing the Map state --
// the computation already succeeded, only its export is unavailable. Both
// that fallback and an unapplied WriterConfig log a warning naming the
// state, so the degradation is diagnosable instead of silent.
func (e *Executor) exportMapResults(
	ctx context.Context, state *State, stateName, mapRunARN string, items, results []any, errs []error,
) (any, int, error) {
	if wc := state.ResultWriter.WriterConfig; wc != nil && writerConfigUnsupported(wc) {
		logger.Load(ctx).WarnContext(ctx,
			"stepfunctions: ResultWriter WriterConfig not applied, writing default JSON array result files",
			"state", stateName, "transformation", wc.Transformation, "outputType", wc.OutputType)
	}

	bucket, _ := state.ResultWriter.Parameters["Bucket"].(string)
	if e.s3w == nil || bucket == "" {
		logger.Load(ctx).WarnContext(ctx,
			"stepfunctions: ResultWriter configured but export unavailable, returning inline results",
			"state", stateName, "bucket", bucket)

		return results, 0, nil
	}

	prefix, _ := state.ResultWriter.Parameters["Prefix"].(string)
	folder := resultFolderKey(prefix, mapRunARN)

	succeeded, failed := partitionMapResults(items, results, errs)

	files, err := e.writeMapResultFiles(ctx, bucket, folder, succeeded, failed)
	if err != nil {
		return nil, 0, fmt.Errorf("ResultWriter: %w", err)
	}

	manifestKey := folder + "manifest.json"

	manifestBytes, err := json.Marshal(resultManifest{
		DestinationBucket: bucket,
		MapRunArn:         mapRunARN,
		ResultFiles:       files,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("ResultWriter: marshal manifest: %w", err)
	}

	if putErr := e.s3w.PutObjectBytes(ctx, bucket, manifestKey, manifestBytes); putErr != nil {
		return nil, 0, fmt.Errorf("ResultWriter: write manifest: %w", putErr)
	}

	out := mapExportOutput{
		MapRunArn:           mapRunARN,
		ResultWriterDetails: ResultWriterDetails{Bucket: bucket, Key: manifestKey},
	}

	return out, len(succeeded), nil
}

// writerConfigUnsupported reports whether wc requests a Transformation or
// OutputType other than the defaults, neither of which exportMapResults
// applies.
func writerConfigUnsupported(wc *ResultWriterConfig) bool {
	transform := wc.Transformation == "COMPACT" || wc.Transformation == "FLATTEN"
	outputType := wc.OutputType == "JSONL"

	return transform || outputType
}

// resultFolderKey builds the slash-terminated S3 key prefix holding one Map
// Run's exported result files. Real AWS uses Prefix/<map-run-uuid>/, the
// UUID taken from the "Map:<uuid>" suffix of the MapRunArn. gopherstack's
// MapRunArn instead ends in "/<execName>/<stateName>" (see
// map_runs.go:mapRunARNFor) with no UUID component, so that suffix is used
// as the folder segment instead -- still unique per Map Run and traceable
// back to the same MapRunArn DescribeMapRun returns.
func resultFolderKey(prefix, mapRunARN string) string {
	const marker = ":mapRun:"

	id := "unknown"
	if idx := strings.LastIndex(mapRunARN, marker); idx >= 0 {
		id = mapRunARN[idx+len(marker):]
	}

	if prefix == "" {
		return id + "/"
	}

	return strings.TrimSuffix(prefix, "/") + "/" + id + "/"
}

// partitionMapResults splits Map iteration results into AWS's SUCCEEDED/
// FAILED result-file buckets by per-item error.
func partitionMapResults(items, results []any, errs []error) ([]mapItemRecord, []mapItemRecord) {
	var succeeded, failed []mapItemRecord

	for i, item := range items {
		if errs[i] != nil {
			failed = append(failed, mapItemRecord{
				Index:  i,
				Input:  item,
				Status: "FAILED",
				Error:  mapResultErrorCode(errs[i]),
				Cause:  errs[i].Error(),
			})

			continue
		}

		succeeded = append(succeeded, mapItemRecord{
			Index:  i,
			Input:  item,
			Output: results[i],
			Status: "SUCCEEDED",
		})
	}

	return succeeded, failed
}

func mapResultErrorCode(err error) string {
	if failErr, ok := errors.AsType[*FailError](err); ok {
		return failErr.ErrCode
	}

	return errCodeStatesTaskFailed
}

// writeMapResultFiles writes SUCCEEDED_0.json/FAILED_0.json, each only when
// non-empty -- AWS's own manifest only references files that were actually
// created.
func (e *Executor) writeMapResultFiles(
	ctx context.Context, bucket, folder string, succeeded, failed []mapItemRecord,
) (resultManifestFiles, error) {
	var files resultManifestFiles

	if len(succeeded) > 0 {
		entry, err := e.writeMapResultFile(ctx, bucket, folder, "SUCCEEDED", succeeded)
		if err != nil {
			return files, err
		}

		files.Succeeded = []resultManifestFile{entry}
	}

	if len(failed) > 0 {
		entry, err := e.writeMapResultFile(ctx, bucket, folder, "FAILED", failed)
		if err != nil {
			return files, err
		}

		files.Failed = []resultManifestFile{entry}
	}

	return files, nil
}

func (e *Executor) writeMapResultFile(
	ctx context.Context, bucket, folder, status string, recs []mapItemRecord,
) (resultManifestFile, error) {
	data, err := json.Marshal(recs)
	if err != nil {
		return resultManifestFile{}, fmt.Errorf("marshal %s results: %w", status, err)
	}

	key := fmt.Sprintf("%s%s_%d.json", folder, status, resultWriterFileIndex)
	if putErr := e.s3w.PutObjectBytes(ctx, bucket, key, data); putErr != nil {
		return resultManifestFile{}, fmt.Errorf("write %s results: %w", status, putErr)
	}

	return resultManifestFile{Key: key, Size: len(data)}, nil
}
