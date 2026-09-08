package dlm

import (
	"context"
	"time"
)

// StorageBackend is the interface for DLM storage operations.
type StorageBackend interface {
	CreateLifecyclePolicy(
		description, executionRoleARN, state string, tags map[string]string, policyDetails map[string]any,
	) (*Policy, error)
	DeleteLifecyclePolicy(policyID string) error
	GetLifecyclePolicies(filter PolicyFilter) ([]*PolicySummary, error)
	GetLifecyclePolicy(policyID string) (*Policy, error)
	// description and executionRoleARN are *string, not string, because the
	// real UpdateLifecyclePolicyInput carries them as pointers: a nil pointer
	// means the field was omitted from the request body (no change), while a
	// non-nil pointer to "" means the caller explicitly sent an empty string
	// (clear the field). A plain string parameter cannot distinguish those
	// two wire states. State stays a plain string because
	// SettablePolicyStateValues is a non-pointer value type on the wire (the
	// real SDK's serializer only ever emits it `if len(State) > 0`, so an
	// explicit empty State is not constructible even by the real SDK).
	//
	// defaultPolicyOverrides carries the top-level [Default policies only]
	// request fields (see defaultPolicyFields), PolicyDetails-keyed. It must
	// be applied under the same lock as the rest of the update: when
	// policyDetails is nil the overrides target the policy's EXISTING stored
	// PolicyDetails, so merging in the handler instead would race a
	// concurrent update.
	UpdateLifecyclePolicy(
		policyID string, description, executionRoleARN *string, state string,
		policyDetails map[string]any, defaultPolicyOverrides map[string]any,
	) error

	TagResource(resourceARN string, tags map[string]string) error
	UntagResource(resourceARN string, tagKeys []string) error
	ListTagsForResource(resourceARN string) (map[string]string, error)

	AccountID() string
	Region() string
	Reset()
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// Policy holds full lifecycle policy details.
// time.Time fields are first so their non-pointer prefix reduces GC pointer bytes.
type Policy struct {
	DateCreated      time.Time
	DateModified     time.Time
	Tags             map[string]string
	PolicyDetails    map[string]any
	Description      string
	ExecutionRoleARN string
	PolicyArn        string
	PolicyID         string
	State            string
	StatusMessage    string
	DefaultPolicy    bool
}

// PolicySummary holds summary lifecycle policy info.
type PolicySummary struct {
	Tags          map[string]string
	PolicyID      string
	Description   string
	State         string
	PolicyType    string
	DefaultPolicy bool
}

// PolicyFilter narrows the results of GetLifecyclePolicies. A zero-value
// field (nil slice or empty string) imposes no restriction along that
// dimension. Matching AWS's documented semantics for the
// GetLifecyclePolicies query parameters, PolicyIDs/ResourceTypes/TargetTags/
// TagsToAdd each apply an ANY-of match within the list, and the dimensions
// are ANDed together.
type PolicyFilter struct {
	State string
	// DefaultPolicyType narrows results to default policies only: "VOLUME"
	// (EBS snapshot default policy), "INSTANCE" (EBS-backed AMI default
	// policy), or "ALL" (any default policy). Empty imposes no restriction
	// (both custom and default policies match).
	DefaultPolicyType string
	PolicyIDs         []string
	// ResourceTypes matches against PolicyDetails.ResourceTypes.
	ResourceTypes []string
	// TargetTags holds "key=value" pairs matched against
	// PolicyDetails.TargetTags.
	TargetTags []string
	// TagsToAdd holds "key=value" pairs matched against the TagsToAdd of any
	// of PolicyDetails.Schedules.
	TagsToAdd []string
}

var _ StorageBackend = (*InMemoryBackend)(nil)
