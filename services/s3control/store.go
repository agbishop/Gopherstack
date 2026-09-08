package s3control

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	// defaultAccessGrantsInstanceID is the fixed ID for the single Access Grants instance per account.
	defaultAccessGrantsInstanceID = "default"

	// jobStatusNew is the initial status for a newly created batch job.
	jobStatusNew = "New"

	// aliasAccountIDMaxLen is the max characters of accountID used in access point aliases.
	aliasAccountIDMaxLen = 8

	// ARN format constants.
	arnFmtAccessGrantsInstance = "arn:aws:s3:%s:%s:access-grants/default"
	arnFmtAccessGrant          = "arn:aws:s3:%s:%s:access-grants/default/grant/%s"
	arnFmtAccessGrantsLocation = "arn:aws:s3:%s:%s:access-grants/default/location/%s"
	arnFmtAccessPoint          = "arn:aws:s3:%s:%s:accesspoint/%s"
	arnFmtObjectLambda         = "arn:aws:s3-object-lambda:%s:%s:accesspoint/%s"
	arnFmtOutpostsBucket       = "arn:aws:s3-outposts:%s:%s:outpost/op-00000000/bucket/%s"
	arnFmtJob                  = "arn:aws:s3:%s:%s:job/%s"
	// arnFmtMRAPToken is the ARN for MRAP async request tokens; gosec false positive (not a credential).
	arnFmtMRAPToken         = "arn:aws:s3::%s:async-request/mrap/create/%s" //nolint:gosec // ARN format, not a credential
	arnFmtStorageLensGroup  = "arn:aws:s3:%s:%s:storage-lens-group/%s"
	arnFmtStorageLensConfig = "arn:aws:s3:%s:%s:storage-lens/%s"
)

// InMemoryBackend is the in-memory store for S3 Control resources.
//
// Every map[string]*T resource field is backed by a *store.Table[T] (see
// pkgs/store and store_setup.go). "Clean" tables key off fields the value
// type already carries and are registered on registry, so Reset/Snapshot/
// Restore collapse to one registry call each. "Dirty" tables (mrapRequests,
// accessPointPABs) key off a field with no natural home on the value type
// and are NOT registered on registry — persistence.go round-trips them
// through an ephemeral DTO store.Registry instead.
type InMemoryBackend struct {
	// objectLambdaSink is wired by cli.go to the S3 backend so a completed
	// Object Lambda access point configuration reaches real GetObject
	// handling (see SetObjectLambdaConfigSink).
	objectLambdaSink             ObjectLambdaConfigSink
	accessGrantsInstancePolicies map[string]string
	objectLambdaAPPolicies       map[string]string
	accessGrants                 *store.Table[AccessGrant]
	accessGrantsLocations        *store.Table[AccessGrantsLocation]
	accessPoints                 *store.Table[AccessPoint]
	objectLambdaAccessPoints     *store.Table[ObjectLambdaAccessPoint]
	mraps                        *store.Table[MultiRegionAccessPoint]
	batchJobs                    *store.Table[BatchJob]
	accessPointScopes            map[string]string
	storageLensGroups            *store.Table[StorageLensGroup]
	configs                      *store.Table[PublicAccessBlock]
	// mrapRequests and accessPointPABs are "dirty" tables -- see the type
	// doc comment above and store_setup.go.
	mrapRequests          *store.Table[MultiRegionAccessPointRequest]
	accessPointPABs       *store.Table[PublicAccessBlock]
	accessPointPolicies   map[string]string
	outpostsBuckets       *store.Table[OutpostsBucket]
	jobTags               map[string]TagSet
	accessGrantsInstances *store.Table[AccessGrantsInstance]
	mu                    *lockmetrics.RWMutex
	objectLambdaAPConfigs map[string]string
	bucketPolicies        map[string]string
	bucketTagging         map[string]TagSet
	bucketLifecycle       map[string]string
	bucketVersioning      map[string]string
	mrapRoutes            map[string]string
	bucketReplication     map[string]string            // accountID:bucketName → replication config XML
	storageLensConfigs    map[string]string            // accountID:configName → config XML
	storageLensConfigTags map[string]TagSet            // accountID:configName → tags
	resourceTags          map[string]map[string]string // ARN → tag key → tag value
	registry              *store.Registry
	region                string
	accountID             string
	nextID                int64
}

// NewInMemoryBackend creates a new InMemoryBackend with default config values.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(config.DefaultAccountID, config.DefaultRegion)
}

// NewInMemoryBackendWithConfig creates a new InMemoryBackend with explicit config values.
func NewInMemoryBackendWithConfig(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:                     store.NewRegistry(),
		accessPointPolicies:          make(map[string]string),
		jobTags:                      make(map[string]TagSet),
		accessGrantsInstancePolicies: make(map[string]string),
		accessPointScopes:            make(map[string]string),
		objectLambdaAPPolicies:       make(map[string]string),
		objectLambdaAPConfigs:        make(map[string]string),
		bucketPolicies:               make(map[string]string),
		bucketTagging:                make(map[string]TagSet),
		bucketLifecycle:              make(map[string]string),
		bucketVersioning:             make(map[string]string),
		mrapRoutes:                   make(map[string]string),
		bucketReplication:            make(map[string]string),
		storageLensConfigs:           make(map[string]string),
		storageLensConfigTags:        make(map[string]TagSet),
		resourceTags:                 make(map[string]map[string]string),
		mu:                           lockmetrics.New("s3control"),
		accountID:                    accountID,
		region:                       region,
	}

	registerAllTables(b)

	return b
}

// AccountID returns the AWS account ID configured for this backend.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the AWS region configured for this backend.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all stored resources without recreating the backend.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.resetTablesLocked()

	b.accessPointPolicies = make(map[string]string)
	b.jobTags = make(map[string]TagSet)
	b.accessGrantsInstancePolicies = make(map[string]string)
	b.accessPointScopes = make(map[string]string)
	b.objectLambdaAPPolicies = make(map[string]string)
	b.objectLambdaAPConfigs = make(map[string]string)
	b.bucketPolicies = make(map[string]string)
	b.bucketTagging = make(map[string]TagSet)
	b.bucketLifecycle = make(map[string]string)
	b.bucketVersioning = make(map[string]string)
	b.mrapRoutes = make(map[string]string)
	b.bucketReplication = make(map[string]string)
	b.storageLensConfigs = make(map[string]string)
	b.storageLensConfigTags = make(map[string]TagSet)
	b.resourceTags = make(map[string]map[string]string)
	b.nextID = 0
}

// resetTablesLocked clears every store.Table-backed resource field --
// both the "clean" tables on b.registry and the "dirty" tables held outside
// it (mrapRequests, accessPointPABs; see the InMemoryBackend doc comment).
// The caller MUST hold b.mu for writing.
func (b *InMemoryBackend) resetTablesLocked() {
	b.registry.ResetAll()
	b.mrapRequests.Reset()
	b.accessPointPABs.Reset()
}

// newID generates a new unique ID string using an internal counter (must be called under lock).
func (b *InMemoryBackend) newID(prefix string) string {
	b.nextID++

	return fmt.Sprintf("%s-%d", prefix, b.nextID)
}

// nowRFC3339 returns the current UTC time formatted as RFC3339.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
