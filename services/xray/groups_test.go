package xray_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/xray"
)

func TestInMemoryBackend_CreateGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs   error
		name        string
		groupName   string
		filterExpr  string
		createFirst bool
		wantErr     bool
	}{
		{
			name:      "creates group",
			groupName: "my-group",
		},
		{
			name:       "creates group with filter",
			groupName:  "filtered-group",
			filterExpr: `service("my-service")`,
		},
		{
			name:        "duplicate group returns conflict",
			groupName:   "dup-group",
			createFirst: true,
			wantErr:     true,
			wantErrIs:   awserr.ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			if tt.createFirst {
				_, err := b.CreateGroup(tt.groupName, "")
				require.NoError(t, err)
			}

			g, err := b.CreateGroup(tt.groupName, tt.filterExpr)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.groupName, g.GroupName)
			assert.Equal(t, tt.filterExpr, g.FilterExpression)
			assert.NotEmpty(t, g.GroupARN)
		})
	}
}

func TestInMemoryBackend_GetGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		groupName string
		create    bool
		wantErr   bool
	}{
		{
			name:      "gets existing group",
			groupName: "my-group",
			create:    true,
		},
		{
			name:      "not found",
			groupName: "missing-group",
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			if tt.create {
				_, err := b.CreateGroup(tt.groupName, "")
				require.NoError(t, err)
			}

			g, err := b.GetGroup(tt.groupName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.groupName, g.GroupName)
		})
	}
}

func TestInMemoryBackend_GetGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		groupNames []string
		wantCount  int
	}{
		{
			name:      "empty",
			wantCount: 0,
		},
		{
			name:       "multiple groups sorted by name",
			groupNames: []string{"beta-group", "alpha-group", "gamma-group"},
			wantCount:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			for _, name := range tt.groupNames {
				_, err := b.CreateGroup(name, "")
				require.NoError(t, err)
			}

			groups := b.GetGroups()
			assert.Len(t, groups, tt.wantCount)

			if len(groups) > 1 {
				assert.Less(t, groups[0].GroupName, groups[1].GroupName)
			}
		})
	}
}

func TestInMemoryBackend_UpdateGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		groupName string
		newFilter string
		create    bool
		wantErr   bool
	}{
		{
			name:      "updates filter expression",
			groupName: "my-group",
			newFilter: `service("updated-svc")`,
			create:    true,
		},
		{
			name:      "not found",
			groupName: "missing-group",
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			if tt.create {
				_, err := b.CreateGroup(tt.groupName, "old-filter")
				require.NoError(t, err)
			}

			g, err := b.UpdateGroup(tt.groupName, tt.newFilter)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.newFilter, g.FilterExpression)
		})
	}
}

func TestInMemoryBackend_DeleteGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		groupName string
		create    bool
		wantErr   bool
	}{
		{
			name:      "deletes existing group",
			groupName: "my-group",
			create:    true,
		},
		{
			name:      "not found",
			groupName: "missing-group",
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			if tt.create {
				_, err := b.CreateGroup(tt.groupName, "")
				require.NoError(t, err)
			}

			err := b.DeleteGroup(tt.groupName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)

			_, getErr := b.GetGroup(tt.groupName)
			require.Error(t, getErr)
		})
	}
}

func TestDeleteGroup_ClearsResourceTagsOnRecreate(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	g, err := b.CreateGroup("reused-group", "")
	require.NoError(t, err)
	require.NoError(t, b.TagResource(g.GroupARN, map[string]string{"env": "prod"}))

	require.NoError(t, b.DeleteGroup("reused-group"))

	recreated, err := b.CreateGroup("reused-group", "")
	require.NoError(t, err)
	require.Equal(t, g.GroupARN, recreated.GroupARN)

	tags, err := b.ListTagsForResource(recreated.GroupARN)
	require.NoError(t, err)
	assert.Empty(t, tags)
}

// DeleteGroupByARN is the path the wire handler actually calls, so it needs the
// same cleanup as DeleteGroup.
func TestDeleteGroupByARN_ClearsResourceTagsOnRecreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tagKey string
		byARN  bool
	}{
		{name: "by_arn", tagKey: "env", byARN: true},
		{name: "by_name", tagKey: "owner", byARN: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			g, err := b.CreateGroup("reused-group", "")
			require.NoError(t, err)
			require.NoError(t, b.TagResource(g.GroupARN, map[string]string{tt.tagKey: "prod"}))

			if tt.byARN {
				require.NoError(t, b.DeleteGroupByARN("", g.GroupARN))
			} else {
				require.NoError(t, b.DeleteGroupByARN("reused-group", ""))
			}

			recreated, err := b.CreateGroup("reused-group", "")
			require.NoError(t, err)

			tags, err := b.ListTagsForResource(recreated.GroupARN)
			require.NoError(t, err)
			assert.Empty(t, tags)
		})
	}
}

// TestGetGroupByARN_UsesIndex verifies that GetGroupByARN works correctly
// when groups are created with custom regions and uses the O(1) ARN index.
func TestGetGroupByARN_UsesIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
		region    string
		groupName string
		wantFound bool
	}{
		{
			name:      "found by ARN",
			accountID: "123456789012",
			region:    "eu-central-1",
			groupName: "test-group",
			wantFound: true,
		},
		{
			name:      "not found returns error",
			accountID: "123456789012",
			region:    "eu-central-1",
			groupName: "", // will try to look up a nonexistent ARN
			wantFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := xray.NewInMemoryBackend(tc.accountID, tc.region)

			if tc.groupName != "" {
				g, err := b.CreateGroup(tc.groupName, "service(\"MyService\")")
				require.NoError(t, err)

				// Look up by the ARN that was returned from Create.
				found, err := b.GetGroupByARN(g.GroupARN)
				require.NoError(t, err)
				assert.Equal(t, g.GroupARN, found.GroupARN)
				assert.Equal(t, tc.groupName, found.GroupName)
			} else {
				_, err := b.GetGroupByARN("arn:aws:xray:eu-central-1:123456789012:group/default/nonexistent")
				assert.Error(t, err)
			}
		})
	}
}
