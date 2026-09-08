package neptune_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_DBSubnetGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_subnet_group",
			vals: url.Values{
				"Action":                       {"CreateDBSubnetGroup"},
				"Version":                      {"2014-10-31"},
				"DBSubnetGroupName":            {"test-sg"},
				"DBSubnetGroupDescription":     {"test subnet group"},
				"SubnetIds.SubnetIdentifier.1": {"subnet-00000000"},
				"SubnetIds.SubnetIdentifier.2": {"subnet-11111111"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "test-sg",
		},
		{
			name: "describe_subnet_groups",
			vals: url.Values{
				"Action":  {"DescribeDBSubnetGroups"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeDBSubnetGroupsResponse",
		},
		{
			name: "delete_subnet_group",
			vals: url.Values{
				"Action":            {"DeleteDBSubnetGroup"},
				"Version":           {"2014-10-31"},
				"DBSubnetGroupName": {"test-sg"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DeleteDBSubnetGroupResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tt.name != "create_subnet_group" {
				doRequest(t, h, url.Values{
					"Action":                   {"CreateDBSubnetGroup"},
					"Version":                  {"2014-10-31"},
					"DBSubnetGroupName":        {"test-sg"},
					"DBSubnetGroupDescription": {"test subnet group"},
				})
			}
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestHandler_DeleteDBSubnetGroup_InUse asserts DeleteDBSubnetGroup rejects a
// subnet group still associated with a DB cluster, matching docdb's sibling
// guard (api_op_DeleteDBSubnetGroup.go: "The specified database subnet group
// must not be associated with any DB instances.").
func TestHandler_DeleteDBSubnetGroup_InUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, url.Values{
		"Action":                   {"CreateDBSubnetGroup"},
		"Version":                  {"2014-10-31"},
		"DBSubnetGroupName":        {"used-sg"},
		"DBSubnetGroupDescription": {"in use"},
	})
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"sg-user-cluster"},
		"DBSubnetGroupName":   {"used-sg"},
	})

	recEarly := doRequest(t, h, url.Values{
		"Action":            {"DeleteDBSubnetGroup"},
		"Version":           {"2014-10-31"},
		"DBSubnetGroupName": {"used-sg"},
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)
	assert.Contains(t, recEarly.Body.String(), "InvalidDBSubnetGroupStateFault")

	recDeleteCluster := doRequest(t, h, url.Values{
		"Action":              {"DeleteDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"sg-user-cluster"},
		"SkipFinalSnapshot":   {"true"},
	})
	require.Equal(t, http.StatusOK, recDeleteCluster.Code)

	recDelete := doRequest(t, h, url.Values{
		"Action":            {"DeleteDBSubnetGroup"},
		"Version":           {"2014-10-31"},
		"DBSubnetGroupName": {"used-sg"},
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_DescribeSubnetGroupsPagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, name := range []string{"sg-1", "sg-2", "sg-3"} {
		doRequest(t, h, url.Values{
			"Action":                   {"CreateDBSubnetGroup"},
			"Version":                  {"2014-10-31"},
			"DBSubnetGroupName":        {name},
			"DBSubnetGroupDescription": {"test"},
		})
	}

	rr := doRequest(t, h, url.Values{
		"Action":     {"DescribeDBSubnetGroups"},
		"Version":    {"2014-10-31"},
		"MaxRecords": {"1"},
	})
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "<Marker>")
}

// ---- DBSubnetGroup comprehensive coverage ----

func TestDBSubnetGroup_CreateAllFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"full-sg"},
		"DBSubnetGroupDescription":     {"Full test subnet group"},
		"VpcId":                        {"vpc-12345678"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-aaaa0001"},
		"SubnetIds.SubnetIdentifier.2": {"subnet-bbbb0002"},
		"SubnetIds.SubnetIdentifier.3": {"subnet-cccc0003"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "full-sg")
	assert.Contains(t, body, "Full test subnet group")
	assert.Contains(t, body, "vpc-12345678")
	assert.Contains(t, body, "subnet-aaaa0001")
	assert.Contains(t, body, "subnet-bbbb0002")
	assert.Contains(t, body, "subnet-cccc0003")
}

func TestDBSubnetGroup_ModifyDescription(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"mod-sg"},
		"DBSubnetGroupDescription":     {"original desc"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-0001"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                   {"ModifyDBSubnetGroup"},
		"Version":                  {"2014-10-31"},
		"DBSubnetGroupName":        {"mod-sg"},
		"DBSubnetGroupDescription": {"updated desc"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "updated desc")
}

func TestDBSubnetGroup_DescribeByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"desc-by-name-sg"},
		"DBSubnetGroupDescription":     {"test"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-xyz"},
	})
	doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"other-sg"},
		"DBSubnetGroupDescription":     {"other"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-abc"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":            {"DescribeDBSubnetGroups"},
		"Version":           {"2014-10-31"},
		"DBSubnetGroupName": {"desc-by-name-sg"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "desc-by-name-sg")
	assert.NotContains(t, body, "other-sg")
}

func TestDBSubnetGroup_DeleteNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":            {"DeleteDBSubnetGroup"},
		"Version":           {"2014-10-31"},
		"DBSubnetGroupName": {"nonexistent-sg"},
	})
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBSubnetGroupNotFoundFault")
}

func TestDBSubnetGroup_CreateAlreadyExists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"dup-sg"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-001"},
	})
	rr := doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"dup-sg"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-001"},
	})
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBSubnetGroupAlreadyExists")
}

func TestDBSubnetGroup_SubnetGroupStatusComplete(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"status-sg"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-001"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "Complete")
}

// ---- Tags on all resource types ----

func TestTags_OnSubnetGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"tagged-sg"},
		"DBSubnetGroupDescription":     {"tagged"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-001"},
		"Tags.Tag.1.Key":               {"Env"},
		"Tags.Tag.1.Value":             {"test"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
}

// --- ModifyDBSubnetGroup ---

func TestModifyDBSubnetGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		groupName    string
		description  string
		wantContains string
		wantStatus   int
		setupGroup   bool
	}{
		{
			name:         "success",
			setupGroup:   true,
			groupName:    "sg-modify",
			description:  "modified description",
			wantStatus:   http.StatusOK,
			wantContains: "sg-modify",
		},
		{
			name:         "not_found",
			setupGroup:   false,
			groupName:    "no-such-sg",
			description:  "desc",
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBSubnetGroupNotFoundFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tt.setupGroup {
				doRequest(t, h, url.Values{
					"Action":                       {"CreateDBSubnetGroup"},
					"Version":                      {"2014-10-31"},
					"DBSubnetGroupName":            {tt.groupName},
					"DBSubnetGroupDescription":     {"initial description"},
					"SubnetIds.SubnetIdentifier.1": {"subnet-aaa"},
				})
			}
			rr := doRequest(t, h, url.Values{
				"Action":                   {"ModifyDBSubnetGroup"},
				"Version":                  {"2014-10-31"},
				"DBSubnetGroupName":        {tt.groupName},
				"DBSubnetGroupDescription": {tt.description},
			})
			assert.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestDescribeDBSubnetGroups_ByName tests single subnet group lookup.
func TestDescribeDBSubnetGroups_ByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"sg-byname"},
		"DBSubnetGroupDescription":     {"test"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-1"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":            {"DescribeDBSubnetGroups"},
		"Version":           {"2014-10-31"},
		"DBSubnetGroupName": {"sg-byname"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "sg-byname")
}

// TestDescribeDBSubnetGroups_NotFound tests not found error.
func TestDescribeDBSubnetGroups_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":            {"DescribeDBSubnetGroups"},
		"Version":           {"2014-10-31"},
		"DBSubnetGroupName": {"nonexistent-sg"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBSubnetGroupNotFoundFault")
}
