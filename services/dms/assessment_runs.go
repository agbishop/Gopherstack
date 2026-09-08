package dms

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// defaultApplicableIndividualAssessments is the static catalog of individual
// premigration assessment names DMS supports, returned by
// DescribeApplicableIndividualAssessments and used as the default set run by
// StartReplicationTaskAssessmentRun when neither IncludeOnly nor Exclude is
// specified. The SDK does not model these names as an enum (they are plain
// strings returned by the service), so this is a representative static
// reference catalog -- matching the convention already used for
// DescribeEndpointTypes/DescribeEngineVersions (rule: a reference-data op
// with no mutable backend state behind it is not a stub).
func defaultApplicableIndividualAssessments() []string {
	return []string{
		"test-connection-source",
		"test-connection-target",
		"test-selection-rules",
		"test-table-mappings",
		"test-primary-key",
		"test-supported-data-types",
	}
}

// CancelReplicationTaskAssessmentRun cancels a single premigration assessment run.
func (b *InMemoryBackend) CancelReplicationTaskAssessmentRun(
	ctx context.Context,
	replicationTaskAssessmentRunArn string,
) error {
	if replicationTaskAssessmentRunArn == "" {
		return fmt.Errorf("%w: ReplicationTaskAssessmentRunArn is required", ErrValidation)
	}

	b.mu.Lock("CancelReplicationTaskAssessmentRun")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	run, ok := b.assessmentRuns.Get(regionKey(region, replicationTaskAssessmentRunArn))
	if !ok {
		return fmt.Errorf(
			"%w: assessment run %s not found",
			ErrNotFound,
			replicationTaskAssessmentRunArn,
		)
	}

	run.Status = statusCancelling

	return nil
}

// StartReplicationTaskAssessment validates and returns the replication task
// for a premigration data-type assessment. Real AWS requires the task to be
// stopped with successful prior connection tests to both endpoints, else it
// throws InvalidResourceStateFault (databasemigrationservice@v1.66.4
// api_op_StartReplicationTaskAssessment.go:16-23).
func (b *InMemoryBackend) StartReplicationTaskAssessment(
	ctx context.Context,
	taskArn string,
) (*ReplicationTask, error) {
	b.mu.RLock("StartReplicationTaskAssessment")
	defer b.mu.RUnlock()

	rt := b.findTask(ctx, taskArn)
	if rt == nil {
		return nil, fmt.Errorf("%w: replication task %s not found", ErrNotFound, taskArn)
	}

	if rt.Status != statusStopped {
		return nil, fmt.Errorf(
			"%w: replication task %s must be stopped to start an assessment; current status is %s",
			ErrInvalidState,
			taskArn,
			rt.Status,
		)
	}

	region := getRegion(ctx, b.region)
	if !b.hasSuccessfulConnection(region, rt.ReplicationInstanceArn, rt.SourceEndpointArn) ||
		!b.hasSuccessfulConnection(region, rt.ReplicationInstanceArn, rt.TargetEndpointArn) {
		return nil, fmt.Errorf(
			"%w: replication task %s requires successful TestConnection results for its source and target endpoints",
			ErrInvalidState,
			taskArn,
		)
	}

	cp := *rt

	return &cp, nil
}

// hasSuccessfulConnection reports whether a prior TestConnection between the
// given replication instance and endpoint recorded status "successful".
func (b *InMemoryBackend) hasSuccessfulConnection(region, replicationInstanceArn, endpointArn string) bool {
	conn, ok := b.connections.Get(regionKey(region, replicationInstanceArn+":"+endpointArn))

	return ok && conn.Status == statusSuccessful
}

// resolveAssessmentNames computes the set of individual assessment names an
// assessment run should execute: includeOnly if given, else the default
// catalog minus exclude, else the full default catalog.
func resolveAssessmentNames(includeOnly, exclude []string) []string {
	if len(includeOnly) > 0 {
		return includeOnly
	}

	base := defaultApplicableIndividualAssessments()
	if len(exclude) == 0 {
		return base
	}

	excluded := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		excluded[e] = true
	}

	names := make([]string, 0, len(base))
	for _, n := range base {
		if !excluded[n] {
			names = append(names, n)
		}
	}

	return names
}

// StartAssessmentRun creates and stores a new premigration assessment run.
// Real AWS executes assessment runs asynchronously; this emulation
// synchronously completes the run (no goroutines/tickers -- matching the
// service's leak-free convention) with every individual assessment passing,
// since there is no real source/target connectivity to actually check.
func (b *InMemoryBackend) StartAssessmentRun(
	ctx context.Context,
	taskArn, serviceAccessRoleArn, resultLocationBucket, assessmentRunName string,
) (*AssessmentRun, error) {
	return b.startAssessmentRunWithSelection(
		ctx, taskArn, serviceAccessRoleArn, resultLocationBucket, assessmentRunName, nil, nil,
	)
}

// startAssessmentRunWithSelection is the full StartAssessmentRun
// implementation, including IncludeOnly/Exclude support. Split out so the
// simpler StartAssessmentRun signature (used by persistence_test.go's
// programmatic seeding) stays stable.
func (b *InMemoryBackend) startAssessmentRunWithSelection(
	ctx context.Context,
	taskArn, serviceAccessRoleArn, resultLocationBucket, assessmentRunName string,
	includeOnly, exclude []string,
) (*AssessmentRun, error) {
	b.mu.Lock("StartAssessmentRun")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	rt, ok := lookupUnique(b.replicationTasksByARN, regionKey(region, taskArn))
	if !ok {
		return nil, fmt.Errorf("%w: replication task %s not found", ErrNotFound, taskArn)
	}

	// Real AWS tracks a single "latest" assessment run per task; unset the
	// flag on any prior runs for this task.
	for _, existing := range b.assessmentRunsByRegion.Get(region) {
		if existing.ReplicationTaskArn == rt.ReplicationTaskArn {
			existing.IsLatestTaskAssessmentRun = false
		}
	}

	names := resolveAssessmentNames(includeOnly, exclude)
	now := time.Now().UTC()
	runARN := arn.Build("dms", region, b.accountID, "assessment-run:"+uuid.NewString())

	individual := make([]*IndividualAssessment, 0, len(names))
	for _, name := range names {
		individual = append(individual, &IndividualAssessment{
			ReplicationTaskIndividualAssessmentArn: arn.Build(
				"dms", region, b.accountID, "individual-assessment:"+uuid.NewString(),
			),
			IndividualAssessmentName:        name,
			ReplicationTaskAssessmentRunArn: runARN,
			ReplicationTaskArn:              rt.ReplicationTaskArn,
			Status:                          statusPassed,
			StartDate:                       now,
		})
	}

	passedCount := int32(len(individual)) //nolint:gosec // bounded by request input

	run := &AssessmentRun{
		ReplicationTaskAssessmentRunArn: runARN,
		ReplicationTaskArn:              rt.ReplicationTaskArn,
		AssessmentRunName:               assessmentRunName,
		Status:                          statusPassed,
		ServiceAccessRoleArn:            serviceAccessRoleArn,
		ResultLocationBucket:            resultLocationBucket,
		ResultLocationFolder:            assessmentRunName,
		CreationDate:                    now,
		Region:                          region,
		IndividualAssessments:           individual,
		ResultStatistic:                 AssessmentRunResultStatistic{Passed: passedCount},
		IsLatestTaskAssessmentRun:       true,
	}
	b.assessmentRuns.Put(run)
	cp := *run

	return &cp, nil
}

// DeleteAssessmentRun removes a stored assessment run.
func (b *InMemoryBackend) DeleteAssessmentRun(ctx context.Context, runArn string) (*AssessmentRun, error) {
	b.mu.Lock("DeleteAssessmentRun")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	run, ok := b.assessmentRuns.Get(regionKey(region, runArn))
	if !ok {
		return nil, fmt.Errorf("%w: assessment run %s not found", ErrNotFound, runArn)
	}

	cp := *run
	b.assessmentRuns.Delete(regionKey(region, runArn))

	return &cp, nil
}

// assessmentRunFilters holds the filter values DescribeReplicationTaskAssessmentRuns
// supports (matches the SDK's documented valid filter names).
type assessmentRunFilters struct {
	runArn                 string
	taskArn                string
	replicationInstanceArn string
	status                 string
}

// DescribeAssessmentRuns returns stored assessment runs, optionally filtered by task ARN.
func (b *InMemoryBackend) DescribeAssessmentRuns(ctx context.Context, taskArn string) ([]*AssessmentRun, error) {
	return b.DescribeAssessmentRunsFiltered(ctx, assessmentRunFilters{taskArn: taskArn})
}

// DescribeAssessmentRunsFiltered returns stored assessment runs matching all
// non-empty filter fields (valid filter names per the SDK: replication-task-
// assessment-run-arn, replication-task-arn, replication-instance-arn, status).
func (b *InMemoryBackend) DescribeAssessmentRunsFiltered(
	ctx context.Context, f assessmentRunFilters,
) ([]*AssessmentRun, error) {
	b.mu.RLock("DescribeAssessmentRuns")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	items := b.assessmentRunsByRegion.Get(region)
	list := make([]*AssessmentRun, 0, len(items))

	for _, run := range items {
		if f.taskArn != "" && run.ReplicationTaskArn != f.taskArn {
			continue
		}

		if f.runArn != "" && run.ReplicationTaskAssessmentRunArn != f.runArn {
			continue
		}

		if f.status != "" && run.Status != f.status {
			continue
		}

		if f.replicationInstanceArn != "" {
			rt, ok := lookupUnique(b.replicationTasksByARN, regionKey(region, run.ReplicationTaskArn))
			if !ok || rt.ReplicationInstanceArn != f.replicationInstanceArn {
				continue
			}
		}

		cp := *run
		list = append(list, &cp)
	}

	return list, nil
}

// DescribeIndividualAssessments returns individual assessments across all
// stored assessment runs, optionally filtered (valid filter names per the
// SDK: replication-task-assessment-run-arn, replication-task-arn, status).
func (b *InMemoryBackend) DescribeIndividualAssessments(
	ctx context.Context, f assessmentRunFilters,
) ([]*IndividualAssessment, error) {
	b.mu.RLock("DescribeIndividualAssessments")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	items := b.assessmentRunsByRegion.Get(region)
	list := make([]*IndividualAssessment, 0)

	for _, run := range items {
		if f.runArn != "" && run.ReplicationTaskAssessmentRunArn != f.runArn {
			continue
		}

		if f.taskArn != "" && run.ReplicationTaskArn != f.taskArn {
			continue
		}

		for _, ia := range run.IndividualAssessments {
			if f.status != "" && ia.Status != f.status {
				continue
			}

			cp := *ia
			list = append(list, &cp)
		}
	}

	return list, nil
}

// DescribeAssessmentResult returns the assessment result for a single
// replication task's most recent assessment run (real AWS: "When this input
// parameter [ReplicationTaskArn] is specified, the API returns only one
// result"). Returns nil if the task has never had an assessment run.
func (b *InMemoryBackend) DescribeAssessmentResult(ctx context.Context, taskArn string) (*AssessmentRun, error) {
	b.mu.RLock("DescribeAssessmentResult")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	for _, run := range b.assessmentRunsByRegion.Get(region) {
		if run.ReplicationTaskArn == taskArn && run.IsLatestTaskAssessmentRun {
			cp := *run

			return &cp, nil
		}
	}

	return nil, nil //nolint:nilnil // absence is a valid "never assessed" state, not an error
}

// DescribeAllAssessmentResults returns one AssessmentRun per task that has
// ever had an assessment run (the latest one), for the no-ReplicationTaskArn
// list form of DescribeReplicationTaskAssessmentResults.
func (b *InMemoryBackend) DescribeAllAssessmentResults(ctx context.Context) ([]*AssessmentRun, error) {
	b.mu.RLock("DescribeAllAssessmentResults")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	items := b.assessmentRunsByRegion.Get(region)
	list := make([]*AssessmentRun, 0, len(items))

	for _, run := range items {
		if run.IsLatestTaskAssessmentRun {
			cp := *run
			list = append(list, &cp)
		}
	}

	return list, nil
}
