package rds_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

func TestSubnetGroup_Modify(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBSubnetGroup("sg1", "original", "vpc-1", []string{"subnet-a", "subnet-b"})
	require.NoError(t, err)

	updated, err := b.ModifyDBSubnetGroup(
		"sg1",
		"updated description",
		[]string{"subnet-c", "subnet-d"},
	)
	require.NoError(t, err)
	assert.Equal(t, "updated description", updated.DBSubnetGroupDescription)
	assert.Len(t, updated.SubnetIDs, 2)
	assert.Equal(t, "subnet-c", updated.SubnetIDs[0])
}

func TestSubnetGroup_NotFound(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()

	err := b.DeleteDBSubnetGroup("noexist")
	require.ErrorIs(t, err, rds.ErrSubnetGroupNotFound)

	_, err = b.DescribeDBSubnetGroups("noexist")
	require.ErrorIs(t, err, rds.ErrSubnetGroupNotFound)
}

func TestSubnetGroup_DeleteInUse(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBSubnetGroup("sg-inuse", "desc", "vpc-1", []string{"subnet-a"})
	require.NoError(t, err)

	_, err = b.CreateDBInstance("inst-sg", "postgres", "db.t3.micro", "", "admin", "", 20,
		rds.DBInstanceOptions{DBSubnetGroupName: "sg-inuse"})
	require.NoError(t, err)

	err = b.DeleteDBSubnetGroup("sg-inuse")
	require.ErrorIs(t, err, rds.ErrSubnetGroupInUse)

	_, err = b.DeleteDBInstance("inst-sg")
	require.NoError(t, err)

	err = b.DeleteDBSubnetGroup("sg-inuse")
	require.NoError(t, err)
}

func TestSubnetGroup_Duplicate(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBSubnetGroup("dup-sg", "first", "vpc-1", []string{"subnet-a"})
	require.NoError(t, err)

	_, err = b.CreateDBSubnetGroup("dup-sg", "second", "vpc-1", []string{"subnet-b"})
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrSubnetGroupAlreadyExists)
}

func TestSubnetGroup_HTTP_Modify(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()

	rec := postRDSForm(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"http-sg"},
		"DBSubnetGroupDescription":     {"original"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-a"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":                       {"ModifyDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"http-sg"},
		"DBSubnetGroupDescription":     {"modified"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-b"},
		"SubnetIds.SubnetIdentifier.2": {"subnet-c"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "http-sg")
}

func TestCreateDBSubnetGroup_SubnetIDs_AreCaptured(t *testing.T) {
	t.Parallel()

	h := rds.NewHandler(rds.NewInMemoryBackend("000000000000", "us-east-1"))

	rec := postRDSForm(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"sg-capture"},
		"DBSubnetGroupDescription":     {"test"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-aaa111"},
		"SubnetIds.SubnetIdentifier.2": {"subnet-bbb222"},
	}.Encode())

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "subnet-aaa111", "first subnet ID must appear in response")
	assert.Contains(t, body, "subnet-bbb222", "second subnet ID must appear in response")
}

func TestModifyDBSubnetGroup_SubnetIDs_AreCaptured(t *testing.T) {
	t.Parallel()

	h := rds.NewHandler(rds.NewInMemoryBackend("000000000000", "us-east-1"))

	postRDSForm(t, h, url.Values{
		"Action":                       {"CreateDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"sg-modify"},
		"DBSubnetGroupDescription":     {"original"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-orig"},
	}.Encode())

	rec := postRDSForm(t, h, url.Values{
		"Action":                       {"ModifyDBSubnetGroup"},
		"Version":                      {"2014-10-31"},
		"DBSubnetGroupName":            {"sg-modify"},
		"DBSubnetGroupDescription":     {"updated"},
		"SubnetIds.SubnetIdentifier.1": {"subnet-new1"},
		"SubnetIds.SubnetIdentifier.2": {"subnet-new2"},
	}.Encode())

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "subnet-new1", "first new subnet ID must appear in response")
	assert.Contains(t, body, "subnet-new2", "second new subnet ID must appear in response")
}

func TestSubnetGroup_MultipleSubnets_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subnets []string
	}{
		{name: "single", subnets: []string{"subnet-111"}},
		{name: "three", subnets: []string{"subnet-aaa", "subnet-bbb", "subnet-ccc"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := rds.NewHandler(rds.NewInMemoryBackend("000000000000", "us-east-1"))

			vals := url.Values{
				"Action":                   {"CreateDBSubnetGroup"},
				"Version":                  {"2014-10-31"},
				"DBSubnetGroupName":        {"sg-" + tc.name},
				"DBSubnetGroupDescription": {"test"},
			}
			for i, id := range tc.subnets {
				key := "SubnetIds.SubnetIdentifier." + func(n int) string {
					switch n {
					case 0:
						return "1"
					case 1:
						return "2"
					default:
						return "3"
					}
				}(i)
				vals[key] = []string{id}
			}

			rec := postRDSForm(t, h, vals.Encode())
			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			for _, subnet := range tc.subnets {
				assert.Contains(t, body, subnet)
			}
		})
	}
}

func TestModifyDBSubnetGroup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs   error
		name        string
		sgName      string
		description string
		subnetIDs   []string
		wantErr     bool
	}{
		{
			name:        "success updates description and subnets",
			sgName:      "my-subnet-group",
			description: "updated description",
			subnetIDs:   []string{"subnet-1111", "subnet-2222"},
		},
		{
			name:      "not found",
			sgName:    "missing",
			wantErr:   true,
			wantErrIs: rds.ErrSubnetGroupNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if !tt.wantErr {
				_, err := b.CreateDBSubnetGroup(tt.sgName, "original desc", "", []string{"subnet-old"})
				require.NoError(t, err)
			}
			got, err := b.ModifyDBSubnetGroup(tt.sgName, tt.description, tt.subnetIDs)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			if tt.description != "" {
				assert.Equal(t, tt.description, got.DBSubnetGroupDescription)
			}
			if len(tt.subnetIDs) > 0 {
				assert.Equal(t, tt.subnetIDs, got.SubnetIDs)
			}
		})
	}
}
