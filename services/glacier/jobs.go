package glacier

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// jobStatusInProgress and jobStatusSucceeded are the retrieval-job status codes.
	jobStatusInProgress = "InProgress"
	jobStatusSucceeded  = "Succeeded"

	// jobIDLength is the length of the random job ID.
	jobIDLength = 60
	// jobTypeArchiveRetrieval is the Glacier job action name for archive retrieval (response).
	jobTypeArchiveRetrieval = "ArchiveRetrieval"
	// jobTypeInventoryRetrieval is the Glacier job action name for inventory retrieval (response).
	jobTypeInventoryRetrieval = "InventoryRetrieval"
	// jobTypeSelect is the Glacier job action name for a select job (response), matching
	// the real SDK's ActionCodeSelect ("Select").
	jobTypeSelect = "Select"
	// jobInputArchiveRetrieval is the type value sent by SDK/clients for archive retrieval (request).
	jobInputArchiveRetrieval = "archive-retrieval"
	// jobInputInventoryRetrieval is the type value sent by SDK/clients for inventory retrieval (request).
	jobInputInventoryRetrieval = "inventory-retrieval"
	// jobInputSelect is the type value sent by SDK/clients for a select job (request).
	jobInputSelect = "select"
)

// cloneJob returns a shallow copy of a Job.
func cloneJob(j *Job) *Job {
	cp := *j

	return &cp
}

// normalizeJobType converts the SDK request type (kebab-case) to the canonical
// action name (PascalCase) used in job responses. Returns empty string if unknown.
func normalizeJobType(t string) string {
	switch t {
	case jobTypeArchiveRetrieval, jobInputArchiveRetrieval:
		return jobTypeArchiveRetrieval
	case jobTypeInventoryRetrieval, jobInputInventoryRetrieval:
		return jobTypeInventoryRetrieval
	case jobTypeSelect, jobInputSelect:
		return jobTypeSelect
	default:
		return ""
	}
}

// isValidInventoryFormat reports whether format is a valid InitiateJob
// Format value ("CSV" or "JSON", case-insensitive) for an inventory-retrieval job.
func isValidInventoryFormat(format string) bool {
	return strings.EqualFold(format, "CSV") || strings.EqualFold(format, "JSON")
}

// isValidTier reports whether tier is one of the allowed retrieval tier values.
func isValidTier(tier string) bool {
	return tier == "Bulk" || tier == "Standard" || tier == "Expedited"
}

// InitiateJob creates a new retrieval or inventory job.
func (b *InMemoryBackend) InitiateJob(accountID, region, vaultName string, req *initiateJobRequest) (*Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Normalize the job type: the SDK sends kebab-case ("archive-retrieval",
	// "inventory-retrieval") but the job action is stored in PascalCase
	// ("ArchiveRetrieval", "InventoryRetrieval") per the AWS API spec.
	action := normalizeJobType(req.Type)
	if action == "" {
		return nil, ErrValidation
	}

	v, vaultExists := b.vaults.Get(vaultARN(accountID, region, vaultName))
	if !vaultExists {
		return nil, ErrVaultNotFound
	}

	invParams, err := validateInitiateJobFields(v, action, req)
	if err != nil {
		return nil, err
	}

	tier, err := resolveJobTier(req.Tier)
	if err != nil {
		return nil, err
	}

	inventoryFormat, err := resolveInventoryFormat(action, req.InventoryFormat)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	// AWS retrievals are asynchronous: a freshly initiated job starts InProgress and
	// only completes after a retrieval window elapses. We simulate that window with
	// retrievalDelay; reads promote the job to Succeeded once it has passed. A zero
	// delay means jobs complete immediately.
	readyAt := now.Add(b.retrievalDelay)
	ready := !now.Before(readyAt)

	statusCode := jobStatusInProgress
	if ready {
		statusCode = jobStatusSucceeded
	}

	j := &Job{
		JobID:              generateID(jobIDLength),
		VaultARN:           v.VaultARN,
		VaultName:          vaultName,
		Action:             action,
		ArchiveID:          req.ArchiveID,
		InventoryFormat:    inventoryFormat,
		JobDescription:     req.Description,
		StatusCode:         statusCode,
		StatusMessage:      statusCode,
		CreationDate:       formatDate(now),
		Completed:          ready,
		Tier:               tier,
		SNSTopic:           req.SNSTopic,
		RetrievalByteRange: req.RetrievalByteRange,
		readyAt:            readyAt,
	}

	if ready {
		j.CompletionDate = formatDate(now)
	}

	applyJobTypeSpecifics(j, v, req, action, invParams, now, ready)

	b.jobs.Put(j)

	return j, nil
}

// validateInitiateJobFields validates action-specific InitiateJob request fields:
// ArchiveId existence for archive-retrieval/select, SelectParameters/OutputLocation
// for select, and InventoryRetrievalParameters for inventory-retrieval (returning the
// parsed form, or nil if none were supplied).
func validateInitiateJobFields(
	v *Vault, action string, req *initiateJobRequest,
) (*parsedInventoryRetrievalParams, error) {
	// ArchiveId is required for both archive-retrieval and select jobs (per the real
	// JobParameters.ArchiveId doc: "required only if Type is set to select or
	// archive-retrieval"); it is invalid to specify for inventory-retrieval.
	if action == jobTypeArchiveRetrieval || action == jobTypeSelect {
		if req.ArchiveID == "" {
			return nil, ErrValidation
		}

		if _, archiveExists := v.Archives[req.ArchiveID]; !archiveExists {
			return nil, ErrArchiveNotFound
		}
	}

	if action == jobTypeSelect {
		if err := validateSelectParameters(req.SelectParameters); err != nil {
			return nil, err
		}

		if err := validateOutputLocation(req.OutputLocation); err != nil {
			return nil, err
		}
	}

	if action == jobTypeInventoryRetrieval && req.InventoryRetrievalParameters != nil {
		return parseInventoryRetrievalParams(req.InventoryRetrievalParameters)
	}

	return nil, nil //nolint:nilnil // absence of InventoryRetrievalParameters is not an error condition
}

// resolveJobTier defaults an empty Tier to "Standard" and validates the result.
func resolveJobTier(tier string) (string, error) {
	if tier == "" {
		tier = "Standard"
	}

	if !isValidTier(tier) {
		return "", fmt.Errorf("%w: tier must be Bulk, Standard, or Expedited", ErrValidation)
	}

	return tier, nil
}

// resolveInventoryFormat defaults an empty Format to "JSON" and, for
// inventory-retrieval jobs only, validates the result is CSV or JSON.
func resolveInventoryFormat(action, format string) (string, error) {
	if format == "" {
		format = "JSON"
	}

	if action == jobTypeInventoryRetrieval && !isValidInventoryFormat(format) {
		return "", fmt.Errorf("%w: Format must be CSV or JSON", ErrValidation)
	}

	return format, nil
}

// applyJobTypeSpecifics populates the action-specific fields on a freshly constructed
// Job: archive metadata for archive-retrieval, vault inventory bookkeeping + range
// parameters for inventory-retrieval, and select/output-location parameters for select.
func applyJobTypeSpecifics(
	j *Job, v *Vault, req *initiateJobRequest, action string,
	invParams *parsedInventoryRetrievalParams, now time.Time, ready bool,
) {
	switch action {
	case jobTypeArchiveRetrieval:
		applyArchiveRetrievalFields(j, v, req.ArchiveID, ready)
	case jobTypeInventoryRetrieval:
		v.LastInventoryDate = formatDate(now)
		v.NumberOfArchivesAtLastInventory = v.NumberOfArchives
		v.SizeInBytesAtLastInventory = v.SizeInBytes
		v.WriteSinceLastInventory = false

		if invParams != nil {
			j.InventoryRetrievalStartDate = invParams.StartDate
			j.InventoryRetrievalEndDate = invParams.EndDate
			j.InventoryRetrievalLimit = invParams.Limit
			j.InventoryRetrievalMarker = invParams.Marker
		}
	case jobTypeSelect:
		j.SelectParameters = req.SelectParameters
		j.OutputLocation = req.OutputLocation
		j.JobOutputPath = computeJobOutputPath(req.OutputLocation, j.JobID)
	}
}

// applyArchiveRetrievalFields copies archive metadata onto a freshly constructed
// archive-retrieval Job. ArchiveSHA256TreeHash is archive metadata: available
// immediately, regardless of job completion (matches AWS). SHA256TreeHash is the
// retrieved-range hash and must stay unset until the job completes --
// promoteJobIfReady sets it when the job transitions to Succeeded.
func applyArchiveRetrievalFields(j *Job, v *Vault, archiveID string, ready bool) {
	a, archiveFound := v.Archives[archiveID]
	if !archiveFound {
		return
	}

	j.ArchiveSizeInBytes = a.Size
	j.ArchiveDescription = a.Description
	j.ArchiveSHA256TreeHash = a.SHA256TreeHash

	if ready {
		j.SHA256TreeHash = a.SHA256TreeHash
	}
}

// DescribeJob returns metadata for a job.
func (b *InMemoryBackend) DescribeJob(accountID, region, vaultName, jobID string) (*Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)

	if !b.vaults.Has(vArn) {
		return nil, ErrVaultNotFound
	}

	j, ok := b.jobs.Get(jobKey(vArn, jobID))
	if !ok {
		return nil, ErrJobNotFound
	}

	promoteJobIfReady(j)

	return cloneJob(j), nil
}

// promoteJobIfReady transitions an InProgress retrieval job to Succeeded once its
// simulated retrieval window (readyAt) has elapsed. Caller must hold b.mu for writing.
func promoteJobIfReady(j *Job) {
	if j.Completed {
		return
	}

	if j.readyAt.IsZero() || time.Now().Before(j.readyAt) {
		return
	}

	j.Completed = true
	j.StatusCode = jobStatusSucceeded
	j.StatusMessage = jobStatusSucceeded
	j.CompletionDate = formatDate(time.Now())

	// SHA256TreeHash (the retrieved-range hash) only becomes available once the
	// job completes; ArchiveSHA256TreeHash (the whole-archive hash) was already
	// set at InitiateJob time. For whole-archive retrievals they are equal.
	if j.Action == jobTypeArchiveRetrieval {
		j.SHA256TreeHash = j.ArchiveSHA256TreeHash
	}
}

// ListJobs returns all jobs for the given vault.
// Returns ErrVaultNotFound if the vault does not exist.
func (b *InMemoryBackend) ListJobs(accountID, region, vaultName string) ([]*Job, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	vArn := vaultARN(accountID, region, vaultName)

	if !b.vaults.Has(vArn) {
		return nil, ErrVaultNotFound
	}

	jobs := b.jobsByVault.Get(vArn)
	result := make([]*Job, 0, len(jobs))

	for _, j := range jobs {
		cj := cloneJob(j)
		promoteJobIfReady(cj)
		result = append(result, cj)
	}

	// Real ListJobs sorts by job initiation time (CreationDate), ascending -- confirmed
	// via the real API's ListJobs example responses, which show JobList entries in
	// ascending CreationDate order. JobID (used previously) is a crypto/rand string with
	// no relationship to creation order. CreationDate is a fixed-width ISO-8601 string
	// (formatDate), so lexical string comparison is equivalent to chronological order.
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreationDate < result[j].CreationDate })

	return result, nil
}

// SetJobInventorySize stores the computed inventory size on the job.
// No-op if the job does not exist.
func (b *InMemoryBackend) SetJobInventorySize(accountID, region, vaultName, jobID string, size int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)

	if j, ok := b.jobs.Get(jobKey(vArn, jobID)); ok {
		j.InventorySizeInBytes = size
	}
}

// AddJobInternal adds a job directly to the backend for testing. VaultARN is
// always recomputed from the accountID/region/vaultName parameters -- see
// the AddVaultInternal doc comment for why.
func (b *InMemoryBackend) AddJobInternal(accountID, region, vaultName string, j *Job) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cp := *j
	cp.VaultARN = vaultARN(accountID, region, vaultName)
	b.jobs.Put(&cp)
}
