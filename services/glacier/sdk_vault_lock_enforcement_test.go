package glacier_test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	glaciersdk "github.com/aws/aws-sdk-go-v2/service/glacier"
	glaciertypes "github.com/aws/aws-sdk-go-v2/service/glacier/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glacier"
)

// newVaultLockTestClient stands up a fresh in-memory glacier backend and a
// real aws-sdk-go-v2 client against an httptest server running its Handler.
// Round-tripping through the genuine SDK serializer/deserializer is what
// proves wire-compatibility a direct handler call would miss.
func newVaultLockTestClient(t *testing.T) *glaciersdk.Client {
	t.Helper()

	bk := glacier.NewInMemoryBackend()
	h := glacier.NewHandler(bk)
	h.AccountID = testAccountID
	h.DefaultRegion = testRegion

	e := echo.New()
	e.Any("/*", h.Handler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion(testRegion),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return glaciersdk.NewFromConfig(cfg, func(o *glaciersdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

func createLockTestVault(t *testing.T, client *glaciersdk.Client, vaultName string) {
	t.Helper()

	_, err := client.CreateVault(t.Context(), &glaciersdk.CreateVaultInput{
		AccountId: aws.String("-"),
		VaultName: aws.String(vaultName),
	})
	require.NoError(t, err)
}

// uploadLockTestArchive uploads a tiny archive and returns its ID. The
// archive's age at deletion time is effectively 0 days, which the
// archive-age subtests below rely on.
func uploadLockTestArchive(t *testing.T, client *glaciersdk.Client, vaultName string) string {
	t.Helper()

	out, err := client.UploadArchive(t.Context(), &glaciersdk.UploadArchiveInput{
		AccountId: aws.String("-"),
		VaultName: aws.String(vaultName),
		Body:      bytes.NewReader([]byte("archive-data")),
	})
	require.NoError(t, err)

	return aws.ToString(out.ArchiveId)
}

func initiateLockTestPolicy(t *testing.T, client *glaciersdk.Client, vaultName, policy string) string {
	t.Helper()

	out, err := client.InitiateVaultLock(t.Context(), &glaciersdk.InitiateVaultLockInput{
		AccountId: aws.String("-"),
		VaultName: aws.String(vaultName),
		Policy:    &glaciertypes.VaultLockPolicy{Policy: aws.String(policy)},
	})
	require.NoError(t, err)

	return aws.ToString(out.LockId)
}

// vaultArchiveCount returns DescribeVault's NumberOfArchives, which only
// reports the count as of the last inventory (gopherstack-zpo5) -- callers
// must run an inventory-retrieval job first or this reads back the SDK's
// int64 zero value for the "no inventory yet" null, not a live count.
func vaultArchiveCount(t *testing.T, client *glaciersdk.Client, vaultName string) int64 {
	t.Helper()

	out, err := client.DescribeVault(t.Context(), &glaciersdk.DescribeVaultInput{
		AccountId: aws.String("-"),
		VaultName: aws.String(vaultName),
	})
	require.NoError(t, err)

	return out.NumberOfArchives
}

// runInventory drives an inventory-retrieval InitiateJob, which snapshots
// NumberOfArchivesAtLastInventory synchronously (jobs.go's
// applyJobTypeSpecifics), before the caller reads it back via
// vaultArchiveCount.
func runInventory(t *testing.T, client *glaciersdk.Client, vaultName string) {
	t.Helper()

	_, err := client.InitiateJob(t.Context(), &glaciersdk.InitiateJobInput{
		AccountId:     aws.String("-"),
		VaultName:     aws.String(vaultName),
		JobParameters: &glaciertypes.JobParameters{Type: aws.String("inventory-retrieval")},
	})
	require.NoError(t, err)
}

func TestVaultLockPolicy_DeleteEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("blanket deny blocks delete archive and archive survives", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "blanket-deny-archive")
		archiveID := uploadLockTestArchive(t, client, "blanket-deny-archive")

		policy := `{"Statement":[` +
			`{"Effect":"Deny","Principal":"*","Action":"glacier:DeleteArchive",` +
			`"Resource":"arn:aws:glacier:` + testRegion + `:000000000000:vaults/blanket-deny-archive"}` +
			`]}`
		lockID := initiateLockTestPolicy(t, client, "blanket-deny-archive", policy)
		_, err := client.CompleteVaultLock(t.Context(), &glaciersdk.CompleteVaultLockInput{
			AccountId: aws.String("-"), VaultName: aws.String("blanket-deny-archive"), LockId: aws.String(lockID),
		})
		require.NoError(t, err)

		_, err = client.DeleteArchive(t.Context(), &glaciersdk.DeleteArchiveInput{
			AccountId: aws.String("-"), VaultName: aws.String("blanket-deny-archive"), ArchiveId: aws.String(archiveID),
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "AccessDenied")

		runInventory(t, client, "blanket-deny-archive")
		require.EqualValues(t, 1, vaultArchiveCount(t, client, "blanket-deny-archive"),
			"the denied delete must not have removed the archive")
	})

	t.Run("blanket deny blocks delete vault and vault survives", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "blanket-deny-vault")

		policy := `{"Statement":[` +
			`{"Effect":"Deny","Principal":"*","Action":"glacier:DeleteVault",` +
			`"Resource":"arn:aws:glacier:` + testRegion + `:000000000000:vaults/blanket-deny-vault"}` +
			`]}`
		lockID := initiateLockTestPolicy(t, client, "blanket-deny-vault", policy)
		_, err := client.CompleteVaultLock(t.Context(), &glaciersdk.CompleteVaultLockInput{
			AccountId: aws.String("-"), VaultName: aws.String("blanket-deny-vault"), LockId: aws.String(lockID),
		})
		require.NoError(t, err)

		_, err = client.DeleteVault(t.Context(), &glaciersdk.DeleteVaultInput{
			AccountId: aws.String("-"), VaultName: aws.String("blanket-deny-vault"),
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "AccessDenied")

		_, err = client.DescribeVault(t.Context(), &glaciersdk.DescribeVaultInput{
			AccountId: aws.String("-"), VaultName: aws.String("blanket-deny-vault"),
		})
		require.NoError(t, err, "the denied delete must not have removed the vault")
	})

	t.Run("deny scoped to a different vault does not block delete", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "other-vault-deny")
		archiveID := uploadLockTestArchive(t, client, "other-vault-deny")

		policy := `{"Statement":[` +
			`{"Effect":"Deny","Principal":"*","Action":"glacier:DeleteArchive",` +
			`"Resource":"arn:aws:glacier:` + testRegion + `:000000000000:vaults/some-unrelated-vault"}` +
			`]}`
		lockID := initiateLockTestPolicy(t, client, "other-vault-deny", policy)
		_, err := client.CompleteVaultLock(t.Context(), &glaciersdk.CompleteVaultLockInput{
			AccountId: aws.String("-"), VaultName: aws.String("other-vault-deny"), LockId: aws.String(lockID),
		})
		require.NoError(t, err)

		_, err = client.DeleteArchive(t.Context(), &glaciersdk.DeleteArchiveInput{
			AccountId: aws.String("-"), VaultName: aws.String("other-vault-deny"), ArchiveId: aws.String(archiveID),
		})
		require.NoError(t, err, "a deny on a different vault's resource must not block this vault")
	})

	t.Run("deny scoped to a different action does not block delete", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "other-action-deny")
		archiveID := uploadLockTestArchive(t, client, "other-action-deny")

		policy := `{"Statement":[` +
			`{"Effect":"Deny","Principal":"*","Action":"glacier:UploadArchive",` +
			`"Resource":"arn:aws:glacier:` + testRegion + `:000000000000:vaults/other-action-deny"}` +
			`]}`
		lockID := initiateLockTestPolicy(t, client, "other-action-deny", policy)
		_, err := client.CompleteVaultLock(t.Context(), &glaciersdk.CompleteVaultLockInput{
			AccountId: aws.String("-"), VaultName: aws.String("other-action-deny"), LockId: aws.String(lockID),
		})
		require.NoError(t, err)

		_, err = client.DeleteArchive(t.Context(), &glaciersdk.DeleteArchiveInput{
			AccountId: aws.String("-"), VaultName: aws.String("other-action-deny"), ArchiveId: aws.String(archiveID),
		})
		require.NoError(t, err, "a deny on a different action must not block DeleteArchive")
	})

	t.Run("archive age condition blocks deletion of a too young archive", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "young-archive-deny")
		archiveID := uploadLockTestArchive(t, client, "young-archive-deny")

		// The archive is seconds old, so "age <= 36500 days" always matches --
		// this is the canonical "deny deletion permissions for archives less
		// than N days old" WORM retention policy.
		policy := `{"Statement":[` +
			`{"Effect":"Deny","Principal":"*","Action":"glacier:DeleteArchive",` +
			`"Resource":"arn:aws:glacier:` + testRegion + `:000000000000:vaults/young-archive-deny",` +
			`"Condition":{"NumericLessThanEquals":{"glacier:ArchiveAgeInDays":"36500"}}}` +
			`]}`
		lockID := initiateLockTestPolicy(t, client, "young-archive-deny", policy)
		_, err := client.CompleteVaultLock(t.Context(), &glaciersdk.CompleteVaultLockInput{
			AccountId: aws.String("-"), VaultName: aws.String("young-archive-deny"), LockId: aws.String(lockID),
		})
		require.NoError(t, err)

		_, err = client.DeleteArchive(t.Context(), &glaciersdk.DeleteArchiveInput{
			AccountId: aws.String("-"), VaultName: aws.String("young-archive-deny"), ArchiveId: aws.String(archiveID),
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "AccessDenied")
	})

	t.Run("archive age condition permits deletion once the threshold is not met", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "old-enough-archive")
		archiveID := uploadLockTestArchive(t, client, "old-enough-archive")

		// The archive is seconds old, so "age >= 36500 days" never matches --
		// the Deny statement is present but inapplicable, and the delete must
		// go through.
		policy := `{"Statement":[` +
			`{"Effect":"Deny","Principal":"*","Action":"glacier:DeleteArchive",` +
			`"Resource":"arn:aws:glacier:` + testRegion + `:000000000000:vaults/old-enough-archive",` +
			`"Condition":{"NumericGreaterThanEquals":{"glacier:ArchiveAgeInDays":"36500"}}}` +
			`]}`
		lockID := initiateLockTestPolicy(t, client, "old-enough-archive", policy)
		_, err := client.CompleteVaultLock(t.Context(), &glaciersdk.CompleteVaultLockInput{
			AccountId: aws.String("-"), VaultName: aws.String("old-enough-archive"), LockId: aws.String(lockID),
		})
		require.NoError(t, err)

		_, err = client.DeleteArchive(t.Context(), &glaciersdk.DeleteArchiveInput{
			AccountId: aws.String("-"), VaultName: aws.String("old-enough-archive"), ArchiveId: aws.String(archiveID),
		})
		require.NoError(t, err, "an age condition the archive doesn't satisfy must not block the delete")
	})

	t.Run("policy is enforced while the lock is still in progress", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "inprogress-deny")
		archiveID := uploadLockTestArchive(t, client, "inprogress-deny")

		policy := `{"Statement":[` +
			`{"Effect":"Deny","Principal":"*","Action":"glacier:DeleteArchive",` +
			`"Resource":"arn:aws:glacier:` + testRegion + `:000000000000:vaults/inprogress-deny"}` +
			`]}`
		// Deliberately no CompleteVaultLock: AWS's documented "test your
		// policy before locking it down" workflow evaluates requests against
		// an InProgress lock too.
		initiateLockTestPolicy(t, client, "inprogress-deny", policy)

		_, err := client.DeleteArchive(t.Context(), &glaciersdk.DeleteArchiveInput{
			AccountId: aws.String("-"), VaultName: aws.String("inprogress-deny"), ArchiveId: aws.String(archiveID),
		})
		require.Error(t, err, "an InProgress lock policy must already be enforced, not only once Locked")
	})

	t.Run("no lock ever initiated allows deletion", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "no-lock-vault")
		archiveID := uploadLockTestArchive(t, client, "no-lock-vault")

		_, err := client.DeleteArchive(t.Context(), &glaciersdk.DeleteArchiveInput{
			AccountId: aws.String("-"), VaultName: aws.String("no-lock-vault"), ArchiveId: aws.String(archiveID),
		})
		require.NoError(t, err, "with no vault lock ever initiated, deletion remains allowed")
	})

	t.Run("initiate vault lock rejects malformed policy json", func(t *testing.T) {
		t.Parallel()

		client := newVaultLockTestClient(t)
		createLockTestVault(t, client, "malformed-policy-vault")

		_, err := client.InitiateVaultLock(t.Context(), &glaciersdk.InitiateVaultLockInput{
			AccountId: aws.String("-"),
			VaultName: aws.String("malformed-policy-vault"),
			Policy:    &glaciertypes.VaultLockPolicy{Policy: aws.String(`{"Statement":[{"Effect":`)},
		})
		require.Error(t, err)
	})
}
