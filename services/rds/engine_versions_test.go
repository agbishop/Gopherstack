package rds_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestCreateCustomDBEngineVersionCRUD verifies full CRUD for custom engine versions.
func TestCreateCustomDBEngineVersionCRUD(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	// Create.
	rec := doAccuracyRDS(t, h, url.Values{
		"Action":        {"CreateCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0.0.ru-2023-01.rur-2023-01.r1"},
	})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	// CreateCustomDBEngineVersionOutput is a flat shape in the real RDS API — the
	// Engine/EngineVersion/Status members sit directly under
	// <CreateCustomDBEngineVersionResult>, with no nested <CustomDBEngineVersion>
	// wrapper (unlike e.g. CreateDBInstanceOutput, which nests under <DBInstance>).
	var createResp struct {
		Result struct {
			Engine        string `xml:"Engine"`
			EngineVersion string `xml:"EngineVersion"`
			Status        string `xml:"Status"`
		} `xml:"CreateCustomDBEngineVersionResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &createResp))
	assert.Equal(t, "custom-oracle-ee", createResp.Result.Engine)
	assert.Equal(t, "available", createResp.Result.Status)

	// Modify.
	modRec := doAccuracyRDS(t, h, url.Values{
		"Action":        {"ModifyCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0.0.ru-2023-01.rur-2023-01.r1"},
		"Status":        {"inactive"},
	})
	require.Equal(t, http.StatusOK, modRec.Code)

	var modResp struct {
		Result struct {
			Status string `xml:"Status"`
		} `xml:"ModifyCustomDBEngineVersionResult"`
	}
	require.NoError(t, xml.Unmarshal(modRec.Body.Bytes(), &modResp))
	assert.Equal(t, "inactive", modResp.Result.Status)

	// Delete.
	delRec := doAccuracyRDS(t, h, url.Values{
		"Action":        {"DeleteCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0.0.ru-2023-01.rur-2023-01.r1"},
	})
	require.Equal(t, http.StatusOK, delRec.Code)

	var delResp struct {
		Result struct {
			Status string `xml:"Status"`
		} `xml:"DeleteCustomDBEngineVersionResult"`
	}
	require.NoError(t, xml.Unmarshal(delRec.Body.Bytes(), &delResp))
	assert.Equal(t, "deleting", delResp.Result.Status)

	// Modify after delete should fail.
	modRec2 := doAccuracyRDS(t, h, url.Values{
		"Action":        {"ModifyCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0.0.ru-2023-01.rur-2023-01.r1"},
		"Status":        {"available"},
	})
	assert.Equal(t, http.StatusBadRequest, modRec2.Code)
}

// TestCreateCustomDBEngineVersionDuplicateRejected verifies duplicates are rejected.
func TestCreateCustomDBEngineVersionDuplicateRejected(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	doAccuracyRDS(t, h, url.Values{
		"Action":        {"CreateCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0"},
	})

	// Second create should fail.
	rec := doAccuracyRDS(t, h, url.Values{
		"Action":        {"CreateCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCustomDBEV_CRUD(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()

	cev, err := b.CreateCustomDBEngineVersion("custom-oracle-ee", "19.0.0.0", "Oracle 19c")
	require.NoError(t, err)
	assert.Equal(t, "custom-oracle-ee", cev.Engine)
	assert.Equal(t, "available", cev.Status)

	_, err = b.ModifyCustomDBEngineVersion("custom-oracle-ee", "19.0.0.0", "Oracle 19c updated", "")
	require.NoError(t, err)

	_, err = b.DeleteCustomDBEngineVersion("custom-oracle-ee", "19.0.0.0")
	require.NoError(t, err)

	_, err = b.DeleteCustomDBEngineVersion("custom-oracle-ee", "19.0.0.0")
	require.Error(t, err)
}

func TestCustomDBEV_ModifyStatus(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateCustomDBEngineVersion("custom-oracle-ee-cdb", "19.0.1.0", "Oracle 19c CDB")
	require.NoError(t, err)

	updated, err := b.ModifyCustomDBEngineVersion(
		"custom-oracle-ee-cdb",
		"19.0.1.0",
		"",
		"inactive-except-restore",
	)
	require.NoError(t, err)
	assert.Equal(t, "inactive-except-restore", updated.Status)
}

func TestCustomDBEV_Duplicate(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateCustomDBEngineVersion("custom-oracle-ee-se2", "19.0.2.0", "test")
	require.NoError(t, err)

	_, err = b.CreateCustomDBEngineVersion("custom-oracle-ee-se2", "19.0.2.0", "duplicate")
	require.Error(t, err)
}

func TestCustomDBEV_Concurrent(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	n := 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = b.CreateCustomDBEngineVersion(
				"custom-oracle-ee",
				fmt.Sprintf("19.0.%d.0", idx),
				fmt.Sprintf("Oracle 19.0.%d", idx),
			)
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, e := range errs {
		if e == nil {
			successCount++
		}
	}
	assert.Equal(t, n, successCount)
}

func TestCustomDBEV_HTTP(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()

	rec := postRDSForm(t, h, url.Values{
		"Action":        {"CreateCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0.0"},
		"Description":   {"Oracle EE 19c custom"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":        {"ModifyCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0.0"},
		"Status":        {"inactive"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":        {"DeleteCustomDBEngineVersion"},
		"Version":       {"2014-10-31"},
		"Engine":        {"custom-oracle-ee"},
		"EngineVersion": {"19.0.0.0"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOrderableOptions_AllEngines(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	opts := b.DescribeOrderableDBInstanceOptions("", "")
	assert.NotEmpty(t, opts)
}

func TestOrderableOptions_FilterByEngine(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	opts := b.DescribeOrderableDBInstanceOptions("postgres", "")
	require.NotEmpty(t, opts)
	for _, o := range opts {
		assert.Equal(t, "postgres", o.Engine)
	}
}

func TestOrderableOptions_ContainsExpectedClasses(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	opts := b.DescribeOrderableDBInstanceOptions("mysql", "")

	classSet := make(map[string]bool)
	for _, o := range opts {
		classSet[o.DBInstanceClass] = true
	}
	assert.True(t, classSet["db.t3.micro"], "expected db.t3.micro in orderable classes")
	assert.True(t, classSet["db.r5.large"], "expected db.r5.large in orderable classes")
}

func TestOrderableOptions_HTTP(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()

	rec := postRDSForm(t, h, url.Values{
		"Action":  {"DescribeOrderableDBInstanceOptions"},
		"Version": {"2014-10-31"},
		"Engine":  {"postgres"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "postgres")
}

func TestPersistence_CustomEngineVersions(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()

	_, err := b.CreateCustomDBEngineVersion("custom-oracle-ee", "19.0.0.0", "Oracle 19c")
	require.NoError(t, err)
	_, err = b.CreateCustomDBEngineVersion("custom-oracle-ee", "21.0.0.0", "Oracle 21c")
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := rds.NewInMemoryBackend("000000000000", "us-east-1")
	err = b2.Restore(t.Context(), snap)
	require.NoError(t, err)

	// Verify: creating the same engine again should fail (already exists)
	_, err = b2.CreateCustomDBEngineVersion("custom-oracle-ee", "19.0.0.0", "Oracle 19c again")
	require.Error(t, err, "duplicate should fail after restore")

	// New version should succeed
	_, err = b2.CreateCustomDBEngineVersion("custom-oracle-ee", "23.0.0.0", "Oracle 23c")
	require.NoError(t, err)
}

func TestDescribeCustomDBEngineVersions_Empty(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()

	versions := b.DescribeCustomDBEngineVersions("", "")
	assert.Empty(t, versions)
}

func TestDescribeCustomDBEngineVersions_AfterCreate(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()

	_, err := b.CreateCustomDBEngineVersion("oracle-ee", "19.0.0.0.ru-2024-04.rur-2024-04.r1", "test cev")
	require.NoError(t, err)

	_, err = b.CreateCustomDBEngineVersion("oracle-ee", "19.0.0.0.ru-2023-10.rur-2023-10.r1", "older cev")
	require.NoError(t, err)

	all := b.DescribeCustomDBEngineVersions("", "")
	require.Len(t, all, 2)

	filtered := b.DescribeCustomDBEngineVersions("oracle-ee", "19.0.0.0.ru-2024-04.rur-2024-04.r1")
	require.Len(t, filtered, 1)
	assert.Equal(t, "19.0.0.0.ru-2024-04.rur-2024-04.r1", filtered[0].EngineVersion)
}

func TestDescribeCustomDBEngineVersions_EngineFilter(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()

	_, err := b.CreateCustomDBEngineVersion("oracle-ee", "19.v1", "oracle")
	require.NoError(t, err)

	_, err = b.CreateCustomDBEngineVersion("oracle-se2", "19.v2", "oracle se2")
	require.NoError(t, err)

	filtered := b.DescribeCustomDBEngineVersions("oracle-ee", "")
	require.Len(t, filtered, 1)
	assert.Equal(t, "oracle-ee", filtered[0].Engine)
}

func TestDescribeCustomDBEngineVersions_SortedResults(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()

	_, err := b.CreateCustomDBEngineVersion("oracle-ee", "19.v2", "v2")
	require.NoError(t, err)

	_, err = b.CreateCustomDBEngineVersion("oracle-ee", "19.v1", "v1")
	require.NoError(t, err)

	all := b.DescribeCustomDBEngineVersions("oracle-ee", "")
	require.Len(t, all, 2)
	assert.Equal(t, "19.v1", all[0].EngineVersion, "results should be sorted by version")
	assert.Equal(t, "19.v2", all[1].EngineVersion)
}

// TestDescribeCustomDBEngineVersions_ViaHandler verifies that a created custom engine
// version is visible via DescribeDBEngineVersions -- the real AWS RDS operation. There
// is no separate "DescribeCustomDBEngineVersions" wire operation on the real RDS SDK
// client (custom engine versions are just DBEngineVersion entries distinguished by
// their Engine value); an earlier pass advertised and dispatched that name as if it
// were real, which this test used to exercise. See DescribeDBEngineVersions's doc
// comment in engine_versions.go.
func TestDescribeCustomDBEngineVersions_ViaHandler(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()
	h := rds.NewHandler(b)

	_, err := b.CreateCustomDBEngineVersion("oracle-ee", "19.test.v1", "test cev")
	require.NoError(t, err)

	rec := postRDSForm(t, h,
		"Action=DescribeDBEngineVersions&Version=2014-10-31&Engine=oracle-ee")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "oracle-ee")
	assert.Contains(t, rec.Body.String(), "19.test.v1")
	assert.Contains(t, rec.Body.String(), "DescribeDBEngineVersionsResponse")
}

// TestDescribeCustomDBEngineVersions_NotAdvertised documents that
// "DescribeCustomDBEngineVersions" is not a real RDS operation and must not be
// advertised via GetSupportedOperations() -- see the ViaHandler test above for the
// real (DescribeDBEngineVersions) equivalent.
func TestDescribeCustomDBEngineVersions_NotAdvertised(t *testing.T) {
	t.Parallel()

	h := newBatch3Handler()
	ops := h.GetSupportedOperations()

	assert.False(
		t,
		slices.Contains(ops, "DescribeCustomDBEngineVersions"),
		"DescribeCustomDBEngineVersions is not a real RDS operation and should not be advertised",
	)
	assert.True(
		t,
		slices.Contains(ops, "DescribeDBEngineVersions"),
		"DescribeDBEngineVersions (the real operation) should be advertised",
	)
}

func TestDBInstance_EngineLifecycleSupport(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()

	inst, err := b.CreateDBInstance("els-inst", "postgres", "db.t3.micro", "", "admin", "", 20,
		rds.DBInstanceOptions{
			EngineLifecycleSupport: "open-source-rds-extended-support-disabled",
		})
	require.NoError(t, err)

	assert.Equal(t, "open-source-rds-extended-support-disabled", inst.EngineLifecycleSupport)
}

func TestValidateEngineLifecycleSupport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		val     string
		wantErr bool
	}{
		{"empty", "", false},
		{"open-source", "open-source-rds-extended-support", false},
		{"disabled", "open-source-rds-extended-support-disabled", false},
		{"invalid", "unsupported-value", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := rds.ValidateEngineLifecycleSupport(tt.val)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDescribeCustomDBEngineVersions_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()

	const goroutines = 10
	done := make(chan struct{}, goroutines)

	for i := range goroutines {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			ver := "19.v" + string(rune('a'+n))
			_, err := b.CreateCustomDBEngineVersion("oracle-ee", ver, "concurrent")
			if err != nil {
				return
			}
			b.DescribeCustomDBEngineVersions("oracle-ee", "")
		}(i)
	}

	for range goroutines {
		<-done
	}

	versions := b.DescribeCustomDBEngineVersions("", "")
	assert.NotEmpty(t, versions)
}

func TestCreateDBCluster_And_CreateDBInstance_RejectInvalidEngineLifecycleSupport(t *testing.T) {
	t.Parallel()

	t.Run("cluster", func(t *testing.T) {
		t.Parallel()

		b := newBatch2Backend()
		_, err := b.CreateDBCluster(
			"bad-els-cluster", "aurora-postgresql", "admin", "", "", 5432, nil,
			rds.DBClusterOptions{EngineLifecycleSupport: "bogus-lifecycle"},
		)
		require.ErrorIs(t, err, rds.ErrInvalidParameter)

		_, descErr := b.DescribeDBClusters("bad-els-cluster")
		assert.Error(t, descErr)
	})

	t.Run("instance", func(t *testing.T) {
		t.Parallel()

		b := newBatch3Backend()
		_, err := b.CreateDBInstance("bad-els-inst", "postgres", "db.t3.micro", "", "admin", "", 20,
			rds.DBInstanceOptions{EngineLifecycleSupport: "bogus-lifecycle"})
		require.ErrorIs(t, err, rds.ErrInvalidParameter)

		_, descErr := b.DescribeDBInstances("bad-els-inst")
		assert.Error(t, descErr)
	})
}
