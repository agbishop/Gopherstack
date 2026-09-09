package azurearm

import "errors"

// Sentinel errors for Azure Resource Manager (ARM) emulation. Mapped to ARM
// error codes/HTTP statuses in the errorDetails table (handler.go), modeled
// on services/sqs's errorDetails pattern.
var (
	ErrInvalidResourceID      = errors.New("azurearm: invalid resource ID")
	ErrResourceGroupNotFound  = errors.New("azurearm: resource group not found")
	ErrResourceNotFound       = errors.New("azurearm: resource not found")
	ErrSubscriptionNotFound   = errors.New("azurearm: subscription not found")
	ErrProviderNotFound       = errors.New("azurearm: resource provider not found")
	ErrInvalidRequestBody     = errors.New("azurearm: invalid request body")
	ErrStorageAccountNotFound = errors.New("azurearm: storage account not found")

	// ErrSnapshotResourceGroupNull/ErrSnapshotResourceNull are returned by
	// Restore when a persisted snapshot's map holds a JSON null entry, which
	// decodes to a nil pointer that would panic on first dereference if
	// stored as-is (same class of bug services/cosmosdb's persistence.go
	// guards against).
	ErrSnapshotResourceGroupNull = errors.New("azurearm: restore snapshot: resource group is null")
	ErrSnapshotResourceNull      = errors.New("azurearm: restore snapshot: resource is null")
)

// errorEntry maps a sentinel error to its ARM JSON error code, message, and
// HTTP status.
type errorEntry struct {
	code    string
	message string
	status  int
}
