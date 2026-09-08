package dms

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/ptrconv"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// assessmentRunProgressJSON mirrors types.ReplicationTaskAssessmentRunProgress.
type assessmentRunProgressJSON struct {
	IndividualAssessmentCompletedCount int32 `json:"IndividualAssessmentCompletedCount"`
	IndividualAssessmentCount          int32 `json:"IndividualAssessmentCount"`
}

// assessmentRunResultStatisticJSON mirrors types.ReplicationTaskAssessmentRunResultStatistic.
type assessmentRunResultStatisticJSON struct {
	Cancelled int32 `json:"Cancelled"`
	Error     int32 `json:"Error"`
	Failed    int32 `json:"Failed"`
	Passed    int32 `json:"Passed"`
	Skipped   int32 `json:"Skipped"`
	Warning   int32 `json:"Warning"`
}

// assessmentRunJSON mirrors types.ReplicationTaskAssessmentRun.
type assessmentRunJSON struct {
	AssessmentRunName               string `json:"AssessmentRunName,omitempty"`
	ReplicationTaskArn              string `json:"ReplicationTaskArn,omitempty"`
	ReplicationTaskAssessmentRunArn string `json:"ReplicationTaskAssessmentRunArn,omitempty"`
	ResultLocationBucket            string `json:"ResultLocationBucket,omitempty"`
	ResultLocationFolder            string `json:"ResultLocationFolder,omitempty"`
	ServiceAccessRoleArn            string `json:"ServiceAccessRoleArn,omitempty"`
	Status                          string `json:"Status,omitempty"`

	// ReplicationTaskAssessmentRunCreationDate is wire-encoded as epoch
	// seconds (awsjson1.1 unixTimestamp format) -- see pkgs/awstime.Epoch.
	ReplicationTaskAssessmentRunCreationDate float64 `json:"ReplicationTaskAssessmentRunCreationDate,omitempty"`

	ResultStatistic    assessmentRunResultStatisticJSON `json:"ResultStatistic"`
	AssessmentProgress assessmentRunProgressJSON        `json:"AssessmentProgress"`

	IsLatestTaskAssessmentRun bool `json:"IsLatestTaskAssessmentRun"`
}

func runToJSON(run *AssessmentRun) assessmentRunJSON {
	individualCount := int32(len(run.IndividualAssessments)) //nolint:gosec // bounded by request input

	return assessmentRunJSON{
		AssessmentProgress: assessmentRunProgressJSON{
			IndividualAssessmentCompletedCount: individualCount,
			IndividualAssessmentCount:          individualCount,
		},
		AssessmentRunName:                        run.AssessmentRunName,
		IsLatestTaskAssessmentRun:                run.IsLatestTaskAssessmentRun,
		ReplicationTaskArn:                       run.ReplicationTaskArn,
		ReplicationTaskAssessmentRunArn:          run.ReplicationTaskAssessmentRunArn,
		ReplicationTaskAssessmentRunCreationDate: awstime.Epoch(run.CreationDate),
		ResultLocationBucket:                     run.ResultLocationBucket,
		ResultLocationFolder:                     run.ResultLocationFolder,
		ResultStatistic: assessmentRunResultStatisticJSON{
			Cancelled: run.ResultStatistic.Cancelled,
			Error:     run.ResultStatistic.Error,
			Failed:    run.ResultStatistic.Failed,
			Passed:    run.ResultStatistic.Passed,
			Skipped:   run.ResultStatistic.Skipped,
			Warning:   run.ResultStatistic.Warning,
		},
		ServiceAccessRoleArn: run.ServiceAccessRoleArn,
		Status:               run.Status,
	}
}

// individualAssessmentJSON mirrors types.ReplicationTaskIndividualAssessment.
type individualAssessmentJSON struct {
	IndividualAssessmentName                     string  `json:"IndividualAssessmentName,omitempty"`
	ReplicationTaskAssessmentRunArn              string  `json:"ReplicationTaskAssessmentRunArn,omitempty"`
	ReplicationTaskIndividualAssessmentArn       string  `json:"ReplicationTaskIndividualAssessmentArn,omitempty"`
	Status                                       string  `json:"Status,omitempty"`
	ReplicationTaskIndividualAssessmentStartDate float64 `json:"ReplicationTaskIndividualAssessmentStartDate,omitempty"`
}

func individualToJSON(ia *IndividualAssessment) individualAssessmentJSON {
	return individualAssessmentJSON{
		IndividualAssessmentName:                     ia.IndividualAssessmentName,
		ReplicationTaskAssessmentRunArn:              ia.ReplicationTaskAssessmentRunArn,
		ReplicationTaskIndividualAssessmentArn:       ia.ReplicationTaskIndividualAssessmentArn,
		ReplicationTaskIndividualAssessmentStartDate: awstime.Epoch(ia.StartDate),
		Status: ia.Status,
	}
}

type cancelReplicationTaskAssessmentRunInput struct {
	ReplicationTaskAssessmentRunArn *string `json:"ReplicationTaskAssessmentRunArn"`
}

type cancelReplicationTaskAssessmentRunOutput struct {
	ReplicationTaskAssessmentRun assessmentRunJSON `json:"ReplicationTaskAssessmentRun"`
}

func (h *Handler) handleCancelReplicationTaskAssessmentRun(
	ctx context.Context, in *cancelReplicationTaskAssessmentRunInput,
) (*cancelReplicationTaskAssessmentRunOutput, error) {
	runArn := ptrconv.String(in.ReplicationTaskAssessmentRunArn)
	if err := h.Backend.CancelReplicationTaskAssessmentRun(ctx, runArn); err != nil {
		return nil, err
	}

	runs, err := h.Backend.DescribeAssessmentRunsFiltered(ctx, assessmentRunFilters{runArn: runArn})
	if err != nil || len(runs) == 0 {
		return nil, fmt.Errorf("%w: assessment run %s not found", ErrNotFound, runArn)
	}

	return &cancelReplicationTaskAssessmentRunOutput{ReplicationTaskAssessmentRun: runToJSON(runs[0])}, nil
}

type deleteReplicationTaskAssessmentRunInput struct {
	ReplicationTaskAssessmentRunArn *string `json:"ReplicationTaskAssessmentRunArn"`
}

type deleteReplicationTaskAssessmentRunOutput struct {
	ReplicationTaskAssessmentRun assessmentRunJSON `json:"ReplicationTaskAssessmentRun"`
}

func (h *Handler) handleDeleteReplicationTaskAssessmentRun(
	ctx context.Context, in *deleteReplicationTaskAssessmentRunInput,
) (*deleteReplicationTaskAssessmentRunOutput, error) {
	run, err := h.Backend.DeleteAssessmentRun(ctx, ptrconv.String(in.ReplicationTaskAssessmentRunArn))
	if err != nil {
		return nil, err
	}

	return &deleteReplicationTaskAssessmentRunOutput{ReplicationTaskAssessmentRun: runToJSON(run)}, nil
}

type describeApplicableIndividualAssessmentsInput struct {
	Marker     *string `json:"Marker"`
	MaxRecords *int32  `json:"MaxRecords"`
}

type describeApplicableIndividualAssessmentsOutput struct {
	Marker                    *string  `json:"Marker,omitempty"`
	IndividualAssessmentNames []string `json:"IndividualAssessmentNames"`
}

func (h *Handler) handleDescribeApplicableIndividualAssessments(
	_ context.Context, in *describeApplicableIndividualAssessmentsInput,
) (*describeApplicableIndividualAssessmentsOutput, error) {
	data, marker := dmsPaginate(defaultApplicableIndividualAssessments(), in.Marker, in.MaxRecords)

	return &describeApplicableIndividualAssessmentsOutput{
		IndividualAssessmentNames: data,
		Marker:                    marker,
	}, nil
}

type describeReplicationTaskAssessmentRunsInput struct {
	Marker     *string       `json:"Marker"`
	MaxRecords *int32        `json:"MaxRecords"`
	Filters    []filterEntry `json:"Filters"`
}

type describeReplicationTaskAssessmentRunsOutput struct {
	Marker                        *string             `json:"Marker,omitempty"`
	ReplicationTaskAssessmentRuns []assessmentRunJSON `json:"ReplicationTaskAssessmentRuns"`
}

func (h *Handler) handleDescribeReplicationTaskAssessmentRuns(
	ctx context.Context, in *describeReplicationTaskAssessmentRunsInput,
) (*describeReplicationTaskAssessmentRunsOutput, error) {
	f := assessmentRunFilters{
		taskArn:                extractFilterValue(in.Filters, "replication-task-arn"),
		runArn:                 extractFilterValue(in.Filters, "replication-task-assessment-run-arn"),
		replicationInstanceArn: extractFilterValue(in.Filters, "replication-instance-arn"),
		status:                 extractFilterValue(in.Filters, "status"),
	}

	runs, err := h.Backend.DescribeAssessmentRunsFiltered(ctx, f)
	if err != nil {
		return nil, err
	}

	all := make([]assessmentRunJSON, 0, len(runs))
	for _, run := range runs {
		all = append(all, runToJSON(run))
	}

	data, marker := dmsPaginate(all, in.Marker, in.MaxRecords)

	return &describeReplicationTaskAssessmentRunsOutput{ReplicationTaskAssessmentRuns: data, Marker: marker}, nil
}

type describeReplicationTaskIndividualAssessmentsInput struct {
	Marker     *string       `json:"Marker"`
	MaxRecords *int32        `json:"MaxRecords"`
	Filters    []filterEntry `json:"Filters"`
}

type describeReplicationTaskIndividualAssessmentsOutput struct {
	Marker                               *string                    `json:"Marker,omitempty"`
	ReplicationTaskIndividualAssessments []individualAssessmentJSON `json:"ReplicationTaskIndividualAssessments"`
}

func (h *Handler) handleDescribeReplicationTaskIndividualAssessments(
	ctx context.Context, in *describeReplicationTaskIndividualAssessmentsInput,
) (*describeReplicationTaskIndividualAssessmentsOutput, error) {
	f := assessmentRunFilters{
		taskArn: extractFilterValue(in.Filters, "replication-task-arn"),
		runArn:  extractFilterValue(in.Filters, "replication-task-assessment-run-arn"),
		status:  extractFilterValue(in.Filters, "status"),
	}

	items, err := h.Backend.DescribeIndividualAssessments(ctx, f)
	if err != nil {
		return nil, err
	}

	all := make([]individualAssessmentJSON, 0, len(items))
	for _, ia := range items {
		all = append(all, individualToJSON(ia))
	}

	data, marker := dmsPaginate(all, in.Marker, in.MaxRecords)

	return &describeReplicationTaskIndividualAssessmentsOutput{
		ReplicationTaskIndividualAssessments: data,
		Marker:                               marker,
	}, nil
}

// assessmentResultJSON mirrors types.ReplicationTaskAssessmentResult.
type assessmentResultJSON struct {
	AssessmentResults                 string  `json:"AssessmentResults,omitempty"`
	AssessmentResultsFile             string  `json:"AssessmentResultsFile,omitempty"`
	AssessmentStatus                  string  `json:"AssessmentStatus,omitempty"`
	ReplicationTaskArn                string  `json:"ReplicationTaskArn,omitempty"`
	ReplicationTaskIdentifier         string  `json:"ReplicationTaskIdentifier,omitempty"`
	ReplicationTaskLastAssessmentDate float64 `json:"ReplicationTaskLastAssessmentDate,omitempty"`
}

type describeReplicationTaskAssessmentResultsInput struct {
	ReplicationTaskArn *string `json:"ReplicationTaskArn"`
	Marker             *string `json:"Marker"`
	MaxRecords         *int32  `json:"MaxRecords"`
}

type describeReplicationTaskAssessmentResultsOutput struct {
	BucketName                       string                 `json:"BucketName,omitempty"`
	Marker                           *string                `json:"Marker,omitempty"`
	ReplicationTaskAssessmentResults []assessmentResultJSON `json:"ReplicationTaskAssessmentResults"`
}

func (h *Handler) handleDescribeReplicationTaskAssessmentResults(
	ctx context.Context, in *describeReplicationTaskAssessmentResultsInput,
) (*describeReplicationTaskAssessmentResultsOutput, error) {
	taskArn := ptrconv.String(in.ReplicationTaskArn)

	// Real AWS: "When [ReplicationTaskArn] is specified, the API returns only
	// one result and ignores the values of MaxRecords and Marker."
	if taskArn != "" {
		run, err := h.Backend.DescribeAssessmentResult(ctx, taskArn)
		if err != nil {
			return nil, err
		}

		if run == nil {
			return &describeReplicationTaskAssessmentResultsOutput{
				ReplicationTaskAssessmentResults: []assessmentResultJSON{},
			}, nil
		}

		tasks, err := h.Backend.DescribeReplicationTasks(ctx, taskArn)
		if err != nil {
			return nil, err
		}

		result := assessmentResultToJSON(run, tasks)

		return &describeReplicationTaskAssessmentResultsOutput{
			BucketName:                       run.ResultLocationBucket,
			ReplicationTaskAssessmentResults: []assessmentResultJSON{result},
		}, nil
	}

	runs, err := h.Backend.DescribeAllAssessmentResults(ctx)
	if err != nil {
		return nil, err
	}

	all := make([]assessmentResultJSON, 0, len(runs))

	for _, run := range runs {
		tasks, taskErr := h.Backend.DescribeReplicationTasks(ctx, run.ReplicationTaskArn)
		if taskErr != nil {
			continue
		}
		// The AssessmentResults/S3ObjectUrl fields are documented as present
		// only when ReplicationTaskArn is provided in the request; omit them
		// in the unfiltered list form.
		all = append(all, assessmentResultJSON{
			AssessmentStatus:                  run.Status,
			ReplicationTaskArn:                run.ReplicationTaskArn,
			ReplicationTaskIdentifier:         taskIdentifierOrEmpty(tasks),
			ReplicationTaskLastAssessmentDate: awstime.Epoch(run.CreationDate),
		})
	}

	data, marker := dmsPaginate(all, in.Marker, in.MaxRecords)

	return &describeReplicationTaskAssessmentResultsOutput{ReplicationTaskAssessmentResults: data, Marker: marker}, nil
}

func taskIdentifierOrEmpty(tasks []*ReplicationTask) string {
	if len(tasks) == 0 {
		return ""
	}

	return tasks[0].ReplicationTaskIdentifier
}

func assessmentResultToJSON(run *AssessmentRun, tasks []*ReplicationTask) assessmentResultJSON {
	return assessmentResultJSON{
		AssessmentResultsFile:             run.ResultLocationFolder,
		AssessmentStatus:                  run.Status,
		ReplicationTaskArn:                run.ReplicationTaskArn,
		ReplicationTaskIdentifier:         taskIdentifierOrEmpty(tasks),
		ReplicationTaskLastAssessmentDate: awstime.Epoch(run.CreationDate),
	}
}

func (h *Handler) handleStartReplicationTaskAssessment(
	ctx context.Context, in *startReplicationTaskAssessmentInput,
) (*startReplicationTaskAssessmentOutput, error) {
	rt, err := h.Backend.StartReplicationTaskAssessment(ctx, ptrconv.String(in.ReplicationTaskArn))
	if err != nil {
		return nil, err
	}

	return &startReplicationTaskAssessmentOutput{ReplicationTask: rtToJSON(rt)}, nil
}

type startReplicationTaskAssessmentRunInput struct {
	ReplicationTaskArn   *string  `json:"ReplicationTaskArn"`
	ServiceAccessRoleArn *string  `json:"ServiceAccessRoleArn"`
	ResultLocationBucket *string  `json:"ResultLocationBucket"`
	AssessmentRunName    *string  `json:"AssessmentRunName"`
	IncludeOnly          []string `json:"IncludeOnly"`
	Exclude              []string `json:"Exclude"`
}

type startReplicationTaskAssessmentRunOutput struct {
	ReplicationTaskAssessmentRun assessmentRunJSON `json:"ReplicationTaskAssessmentRun"`
}

func (h *Handler) handleStartReplicationTaskAssessmentRun(
	ctx context.Context, in *startReplicationTaskAssessmentRunInput,
) (*startReplicationTaskAssessmentRunOutput, error) {
	if err := validateStartAssessmentRunInput(in); err != nil {
		return nil, err
	}

	run, err := h.Backend.startAssessmentRunWithSelection(
		ctx,
		ptrconv.String(in.ReplicationTaskArn),
		ptrconv.String(in.ServiceAccessRoleArn),
		ptrconv.String(in.ResultLocationBucket),
		ptrconv.String(in.AssessmentRunName),
		in.IncludeOnly,
		in.Exclude,
	)
	if err != nil {
		return nil, err
	}

	return &startReplicationTaskAssessmentRunOutput{ReplicationTaskAssessmentRun: runToJSON(run)}, nil
}

// validateStartAssessmentRunInput checks StartReplicationTaskAssessmentRunInput's
// four required members and the mutual exclusion of IncludeOnly/Exclude.
func validateStartAssessmentRunInput(in *startReplicationTaskAssessmentRunInput) error {
	if ptrconv.String(in.AssessmentRunName) == "" {
		return fmt.Errorf("%w: AssessmentRunName is required", ErrValidation)
	}

	if ptrconv.String(in.ReplicationTaskArn) == "" {
		return fmt.Errorf("%w: ReplicationTaskArn is required", ErrValidation)
	}

	if ptrconv.String(in.ResultLocationBucket) == "" {
		return fmt.Errorf("%w: ResultLocationBucket is required", ErrValidation)
	}

	if ptrconv.String(in.ServiceAccessRoleArn) == "" {
		return fmt.Errorf("%w: ServiceAccessRoleArn is required", ErrValidation)
	}

	if len(in.IncludeOnly) > 0 && len(in.Exclude) > 0 {
		return fmt.Errorf("%w: cannot set both IncludeOnly and Exclude", ErrValidation)
	}

	return nil
}

// opsAssessmentRuns returns the dispatch-table entries for the assessment_runs operation family.
func (h *Handler) opsAssessmentRuns() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"CancelReplicationTaskAssessmentRun": service.WrapOp(
			h.handleCancelReplicationTaskAssessmentRun,
		),
		opDeleteReplicationTaskAssessmentRun: service.WrapOp(
			h.handleDeleteReplicationTaskAssessmentRun,
		),
		opDescribeApplicableIndividualAssessments: service.WrapOp(
			h.handleDescribeApplicableIndividualAssessments,
		),
		opDescribeReplicationTaskAssessmentResults: service.WrapOp(
			h.handleDescribeReplicationTaskAssessmentResults,
		),
		opDescribeReplicationTaskAssessmentRuns: service.WrapOp(
			h.handleDescribeReplicationTaskAssessmentRuns,
		),
		opDescribeReplicationTaskIndividualAssessments: service.WrapOp(
			h.handleDescribeReplicationTaskIndividualAssessments,
		),
		opStartReplicationTaskAssessment: service.WrapOp(
			h.handleStartReplicationTaskAssessment,
		),
		opStartReplicationTaskAssessmentRun: service.WrapOp(
			h.handleStartReplicationTaskAssessmentRun,
		),
	}
}
