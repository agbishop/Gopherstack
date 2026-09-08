package swf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/swf"
)

// TestRegisterActivityType verifies creation and retrieval.
func TestRegisterActivityType(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterActivityType("dom", "act", "2.0", "activity desc", swf.ActivityTypeDefaults{}))

	at, err := b.DescribeActivityType("dom", "act", "2.0")
	require.NoError(t, err)
	assert.Equal(t, "act", at.Name)
	assert.Equal(t, "REGISTERED", at.Status)
	assert.Equal(t, "activity desc", at.Description)
}

// TestRegisterActivityType_Duplicate verifies ErrTypeAlreadyExists.
func TestRegisterActivityType_Duplicate(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterActivityType("dom", "act", "1.0", "", swf.ActivityTypeDefaults{}))

	err := b.RegisterActivityType("dom", "act", "1.0", "", swf.ActivityTypeDefaults{})

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrTypeAlreadyExists)
}

// TestRegisterActivityType_DomainChecks verifies ErrNotFound (UnknownResourceFault)
// both when the domain does not exist and when it has been deprecated -- per
// DeprecateDomain's doc ("it cannot be used to create new workflow executions or
// register new types"). RegisterActivityType's modelled error set has
// UnknownResourceFault but no DomainDeprecatedFault.
func TestRegisterActivityType_DomainChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		setupDeprecatedDomain bool
	}{
		{name: "domain_missing"},
		{name: "domain_deprecated", setupDeprecatedDomain: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			if tt.setupDeprecatedDomain {
				require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
				require.NoError(t, b.DeprecateDomain("dom"))
			}

			err := b.RegisterActivityType("dom", "act", "1.0", "", swf.ActivityTypeDefaults{})

			require.Error(t, err)
			assert.ErrorIs(t, err, swf.ErrNotFound)
		})
	}
}

func TestActivityTypeDefaults(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	defaults := swf.ActivityTypeDefaults{
		DefaultTaskList:                   "act-list",
		DefaultTaskHeartbeatTimeout:       "300",
		DefaultTaskScheduleToCloseTimeout: "3600",
		DefaultTaskScheduleToStartTimeout: "600",
		DefaultTaskStartToCloseTimeout:    "3000",
	}
	require.NoError(t, b.RegisterActivityType("dom", "act1", "1.0", "activity", defaults))

	at, err := b.DescribeActivityType("dom", "act1", "1.0")
	require.NoError(t, err)
	assert.Equal(t, defaults.DefaultTaskHeartbeatTimeout, at.Defaults.DefaultTaskHeartbeatTimeout)
	assert.Equal(t, defaults.DefaultTaskScheduleToCloseTimeout, at.Defaults.DefaultTaskScheduleToCloseTimeout)
}

// TestListActivityTypes_FilterStatus verifies registrationStatus filter.
func TestListActivityTypes_FilterStatus(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterActivityType("dom", "act1", "1.0", "", swf.ActivityTypeDefaults{}))
	require.NoError(t, b.RegisterActivityType("dom", "act2", "1.0", "", swf.ActivityTypeDefaults{}))
	require.NoError(t, b.DeprecateActivityType("dom", "act2", "1.0"))

	reg, err := b.ListActivityTypes("dom", "REGISTERED")
	require.NoError(t, err)
	dep, err := b.ListActivityTypes("dom", "DEPRECATED")
	require.NoError(t, err)
	all, err := b.ListActivityTypes("dom", "")
	require.NoError(t, err)

	assert.Len(t, reg, 1)
	assert.Len(t, dep, 1)
	assert.Len(t, all, 2)
}

// TestUndeprecateActivityType verifies round-trip deprecate→undeprecate.
func TestUndeprecateActivityType(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterActivityType("dom", "act", "1.0", "", swf.ActivityTypeDefaults{}))
	require.NoError(t, b.DeprecateActivityType("dom", "act", "1.0"))
	require.NoError(t, b.UndeprecateActivityType("dom", "act", "1.0"))

	at, err := b.DescribeActivityType("dom", "act", "1.0")
	require.NoError(t, err)
	assert.Equal(t, "REGISTERED", at.Status)
}

// TestUndeprecateActivityType_AlreadyActive verifies TypeAlreadyExistsFault on an active type.
func TestUndeprecateActivityType_AlreadyActive(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterActivityType("dom", "act", "1.0", "", swf.ActivityTypeDefaults{}))

	err := b.UndeprecateActivityType("dom", "act", "1.0")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrTypeAlreadyExists)
}

// TestDeleteActivityType verifies DeleteActivityType requires the type to be
// deprecated first (real AWS: "Prior to deletion, activity types must first
// be deprecated") and removes it from the registered-types table on success.
func TestDeleteActivityType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr        error
		name           string
		deprecateFirst bool
	}{
		{name: "NotDeprecated", wantErr: swf.ErrTypeNotDeprecated},
		{name: "Deprecated", deprecateFirst: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
			require.NoError(t, b.RegisterActivityType("dom", "act1", "1.0", "", swf.ActivityTypeDefaults{}))

			if tt.deprecateFirst {
				require.NoError(t, b.DeprecateActivityType("dom", "act1", "1.0"))
			}

			err := b.DeleteActivityType("dom", "act1", "1.0")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				_, describeErr := b.DescribeActivityType("dom", "act1", "1.0")
				require.NoError(t, describeErr, "type must remain registered when delete is rejected")

				return
			}
			require.NoError(t, err)

			_, err = b.DescribeActivityType("dom", "act1", "1.0")
			assert.ErrorIs(t, err, swf.ErrNotFound)
		})
	}
}

// TestDeleteActivityType_NotFound verifies deleting an unregistered activity
// type fails with UnknownResourceFault.
func TestDeleteActivityType_NotFound(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	err := b.DeleteActivityType("dom", "nonexistent", "1.0")
	require.ErrorIs(t, err, swf.ErrNotFound)
}
