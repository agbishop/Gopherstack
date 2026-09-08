package rds_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestReset_ClearsAccountAndInstanceState verifies Reset clears the readiness
// timers, log content, and account-level CA certificate override that
// TestReset (db_instances_test.go) doesn't cover.
func TestReset_ClearsAccountAndInstanceState(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	t.Cleanup(b.Close)

	_, err := b.CreateDBInstance("inst-1", "mysql", "", "", "", "", 0, rds.DBInstanceOptions{})
	require.NoError(t, err)
	b.AddClusterInternal("cluster-1", "aurora-mysql")

	_, err = b.DescribeDBLogFiles("inst-1", rds.LogFileFilter{})
	require.NoError(t, err)
	require.Equal(t, 1, rds.InstanceLogFilesCountForTest(b), "setup: log files not seeded")
	require.Equal(t, 1, rds.InstanceLogContentCountForTest(b), "setup: log content not seeded")

	b.SetPerformanceInsightsData("inst-1", "db.CPU.Total.Avg", []rds.PIDataPoint{{Timestamp: "t", Value: 1}})
	require.Equal(t, 1, rds.PIMetricsCountForTest(b), "setup: PI metrics not stored")

	_, err = b.RebootDBCluster("cluster-1")
	require.NoError(t, err)
	require.Equal(t, 1, rds.ClusterReadyAtCountForTest(b), "setup: reboot didn't schedule a ready-at deadline")

	cert, err := b.ModifyCertificates("rds-ca-2019")
	require.NoError(t, err)
	require.Equal(t, "rds-ca-2019", cert.CertificateIdentifier, "setup: ModifyCertificates didn't take")

	b.Reset()

	assert.Zero(t, rds.InstanceLogFilesCountForTest(b), "Reset must clear instanceLogFiles")
	assert.Zero(t, rds.InstanceLogContentCountForTest(b), "Reset must clear instanceLogContent")
	assert.Zero(t, rds.PIMetricsCountForTest(b), "Reset must clear piMetrics")
	assert.Zero(t, rds.ClusterReadyAtCountForTest(b), "Reset must clear clusterReadyAt")

	certs, err := b.DescribeCertificates("")
	require.NoError(t, err)

	var defaultCert, overrideCert *rds.Certificate
	for i := range certs {
		switch certs[i].CertificateIdentifier {
		case "rds-ca-rsa2048-g1":
			defaultCert = &certs[i]
		case "rds-ca-2019":
			overrideCert = &certs[i]
		}
	}
	require.NotNil(t, defaultCert)
	require.NotNil(t, overrideCert)

	// defaultCACertificateID's clean-state value is the const default
	// ("rds-ca-rsa2048-g1"), not the Go zero value "" -- Reset must restore
	// it, not merely blank it, or DescribeCertificates would report no
	// account default at all.
	assert.True(t, defaultCert.CustomerOverride,
		"Reset must restore defaultCACertificateID to the const default, not zero it")
	assert.False(t, overrideCert.CustomerOverride,
		"Reset must clear the ModifyCertificates override")
}
