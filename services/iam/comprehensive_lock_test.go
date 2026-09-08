package iam_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// TestComprehensiveBackend_NoDataRace exercises comprehensiveBackend state
// (SSH keys, MFA device links, access advisor jobs, org access report jobs)
// concurrently with regular backend state (users, credential reports) that
// reads comprehensive fields inline (GetCredentialReport, DeleteUser). Both
// used to be guarded by two separate locks (b.mu and comprehensiveBackend's
// own sync.Mutex); this only proves anything under `-race`, since a
// single-threaded call sequence never exercised the two locks concurrently.
func TestComprehensiveBackend_NoDataRace(t *testing.T) {
	t.Parallel()

	b := iam.NewInMemoryBackend()

	const workers = 8

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			userName := fmt.Sprintf("user-%d", i)
			_, err := b.CreateUser(userName, "/", "")
			assert.NoError(t, err)

			_, err = b.UploadSSHPublicKey(userName, "ssh-rsa AAAAB3NzaC1yc2E fixture-key-body")
			assert.NoError(t, err)

			dev, err := b.CreateVirtualMFADeviceFull(fmt.Sprintf("mfa-%d", i), "/")
			assert.NoError(t, err)

			err = b.EnableMFADevice(userName, dev.SerialNumber, "123456", "654321")
			assert.NoError(t, err)

			_ = b.GetMFADeviceOwner(dev.SerialNumber)
			_, err = b.ListMFADevicesForUser(userName, "", 0)
			assert.NoError(t, err)

			_ = b.GenerateServiceLastAccessedDetailsForEntity("arn:aws:iam::123456789012:user/" + userName)
			b.RecordServiceAccess("arn:aws:iam::123456789012:user/"+userName, "s3", "Amazon S3")
			_ = b.GenerateOrganizationsAccessReport("/")

			_ = b.GetCredentialReport()
			_, _ = b.GetAccountAuthorizationDetails("", 0, nil)

			// DeleteUser must see its own SSH key under the same lock that
			// UploadSSHPublicKey wrote it under -- expect DeleteConflict, not
			// a stale "no dependents" read.
			delErr := b.DeleteUser(userName)
			assert.Error(t, delErr)
		}(i)
	}

	wg.Wait()
}

// TestDeleteUser_SSHKeyConflictIsAtomic locks in that DeleteUser's dependency
// check and the delete itself observe SSH-key/MFA state under one atomic
// critical section (both now guarded by the same b.mu), rather than the old
// two-lock sequence (check under comprehensiveBackend's private mutex, then
// separately take b.mu to delete) that left a window for the checked state to
// go stale before the delete committed.
func TestDeleteUser_SSHKeyConflictIsAtomic(t *testing.T) {
	t.Parallel()

	b := iam.NewInMemoryBackend()
	_, err := b.CreateUser("carol", "/", "")
	require.NoError(t, err)

	_, err = b.UploadSSHPublicKey("carol", "ssh-rsa AAAAB3NzaC1yc2E fixture-key-body")
	require.NoError(t, err)

	err = b.DeleteUser("carol")
	require.Error(t, err)

	key, err := b.ListSSHPublicKeys("carol", "", 0)
	require.NoError(t, err)
	require.Len(t, key.Data, 1)

	require.NoError(t, b.DeleteSSHPublicKey("carol", key.Data[0].SSHPublicKeyID))
	require.NoError(t, b.DeleteUser("carol"))
}
