package mediaconvert

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateResourceShare records a resource-share request for the given job ID.
// Sets ShareStatus = "SHARED" on the job and populates LastShareDetails.
// JobId and SupportCaseId are both "This member is required" on
// CreateResourceShareInput (aws-sdk-go-v2 mediaconvert
// api_op_CreateResourceShare.go), and the SDK's own client-side validator
// (validators.go validateOpCreateResourceShareInput) rejects a request
// missing either before it is ever sent.
func (b *InMemoryBackend) CreateResourceShare(jobID, supportCaseID string) (string, error) {
	b.mu.Lock("CreateResourceShare")
	defer b.mu.Unlock()

	if jobID == "" {
		return "", fmt.Errorf("%w: jobId is required", ErrValidation)
	}

	if supportCaseID == "" {
		return "", fmt.Errorf("%w: supportCaseId is required", ErrValidation)
	}

	j, ok := b.jobs.Get(jobID)
	if !ok {
		return "", fmt.Errorf("%w: job %s not found", ErrNotFound, jobID)
	}

	token := uuid.NewString()
	j.ShareStatus = "SHARED"

	details, err := json.Marshal(struct {
		ShareToken string  `json:"shareToken"`
		SharedAt   float64 `json:"sharedAt"`
	}{ShareToken: token, SharedAt: epochSeconds(time.Now())})
	if err != nil {
		return "", fmt.Errorf("encode share details: %w", err)
	}
	detailsStr := string(details)
	j.LastShareDetails = &detailsStr

	return jobID, nil
}
