package backup_test

import (
	"testing"

	backupsdk "github.com/aws/aws-sdk-go-v2/service/backup"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/backup"
)

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// backup client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.  The test fails when the upstream
// SDK adds a new operation that gopherstack has not yet handled.
func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	backend := backup.NewInMemoryBackend("000000000000", "us-east-1")
	h := backup.NewHandler(backend)

	// Added by the aws-sdk-go-v2/service/backup v1.64.0 bump; unimplemented.
	notImplemented := []string{
		"CreateBackupAccessPoint",
		"DeleteBackupAccessPoint",
		"DescribeBackupAccessPoint",
		"ListBackupAccessPoints",
		"ListBackupAccessPointsByRecoveryPoint",
		"ListBackupAccessPointsByResource",
	}
	sdkcheck.CheckCompleteness(t, &backupsdk.Client{}, h.GetSupportedOperations(), notImplemented)
}
