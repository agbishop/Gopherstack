package swf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/swf"
)

func TestRegisterWorkflowType(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("my-domain", "", "NONE"))

	err := b.RegisterWorkflowType("my-domain", "my-workflow", "1.0", "", swf.WorkflowTypeDefaults{})
	require.NoError(t, err)

	wts, err := b.ListWorkflowTypes("my-domain", "")
	require.NoError(t, err)
	require.Len(t, wts, 1)
	assert.Equal(t, "my-workflow", wts[0].Name)
}

// TestErrValidation_RegisterWorkflowType verifies validation of required fields.
func TestErrValidation_RegisterWorkflowType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		domain  string
		wfName  string
		version string
	}{
		{name: "empty_domain", domain: "", wfName: "wf", version: "1.0"},
		{name: "empty_name", domain: "d1", wfName: "", version: "1.0"},
		{name: "empty_version", domain: "d1", wfName: "wf", version: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			err := b.RegisterWorkflowType(tt.domain, tt.wfName, tt.version, "", swf.WorkflowTypeDefaults{})

			require.Error(t, err)
			assert.ErrorIs(t, err, swf.ErrValidation)
		})
	}
}

// TestRegisterWorkflowType_DomainChecks verifies ErrNotFound (UnknownResourceFault)
// both when the domain does not exist and when it has been deprecated -- per
// DeprecateDomain's doc ("it cannot be used to create new workflow executions or
// register new types"). RegisterWorkflowType's modelled error set has
// UnknownResourceFault but no DomainDeprecatedFault.
func TestRegisterWorkflowType_DomainChecks(t *testing.T) {
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

			err := b.RegisterWorkflowType("dom", "wf", "1.0", "", swf.WorkflowTypeDefaults{})

			require.Error(t, err)
			assert.ErrorIs(t, err, swf.ErrNotFound)
		})
	}
}

// TestRegisterWorkflowType_StoredDescription verifies description is stored.
func TestRegisterWorkflowType_StoredDescription(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf", "1.0", "my desc", swf.WorkflowTypeDefaults{}))

	wt, err := b.DescribeWorkflowType("dom", "wf", "1.0")
	require.NoError(t, err)
	assert.Equal(t, "my desc", wt.Description)
}

func TestWorkflowTypeDefaults(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	defaults := swf.WorkflowTypeDefaults{
		DefaultTaskList:                     "my-task-list",
		DefaultTaskPriority:                 "10",
		DefaultTaskStartToCloseTimeout:      "3600",
		DefaultExecutionStartToCloseTimeout: "86400",
		DefaultChildPolicy:                  "TERMINATE",
		DefaultLambdaRole:                   "arn:aws:iam::123:role/swf-lambda",
	}
	require.NoError(t, b.RegisterWorkflowType("dom", "wf1", "1.0", "desc", defaults))

	wt, err := b.DescribeWorkflowType("dom", "wf1", "1.0")
	require.NoError(t, err)
	assert.Equal(t, defaults.DefaultTaskList, wt.Defaults.DefaultTaskList)
	assert.Equal(t, defaults.DefaultChildPolicy, wt.Defaults.DefaultChildPolicy)
	assert.Equal(t, defaults.DefaultExecutionStartToCloseTimeout, wt.Defaults.DefaultExecutionStartToCloseTimeout)
}

// TestListWorkflowTypes_FilterStatus verifies registrationStatus filter works.
func TestListWorkflowTypes_FilterStatus(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf1", "1.0", "", swf.WorkflowTypeDefaults{}))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf2", "1.0", "", swf.WorkflowTypeDefaults{}))
	require.NoError(t, b.DeprecateWorkflowType("dom", "wf2", "1.0"))

	registered, err := b.ListWorkflowTypes("dom", "REGISTERED")
	require.NoError(t, err)
	deprecated, err := b.ListWorkflowTypes("dom", "DEPRECATED")
	require.NoError(t, err)
	all, err := b.ListWorkflowTypes("dom", "")
	require.NoError(t, err)

	assert.Len(t, registered, 1)
	assert.Equal(t, "wf1", registered[0].Name)
	assert.Len(t, deprecated, 1)
	assert.Equal(t, "wf2", deprecated[0].Name)
	assert.Len(t, all, 2)
}

// TestUndeprecateWorkflowType verifies round-trip deprecate→undeprecate.
func TestUndeprecateWorkflowType(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf", "1.0", "", swf.WorkflowTypeDefaults{}))
	require.NoError(t, b.DeprecateWorkflowType("dom", "wf", "1.0"))
	require.NoError(t, b.UndeprecateWorkflowType("dom", "wf", "1.0"))

	wt, err := b.DescribeWorkflowType("dom", "wf", "1.0")
	require.NoError(t, err)
	assert.Equal(t, "REGISTERED", wt.Status)
}

// TestUndeprecateWorkflowType_AlreadyRegistered returns ErrTypeAlreadyExists.
func TestUndeprecateWorkflowType_AlreadyRegistered(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf", "1.0", "", swf.WorkflowTypeDefaults{}))

	err := b.UndeprecateWorkflowType("dom", "wf", "1.0")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrTypeAlreadyExists)
}

// TestUndeprecateWorkflowType_AlreadyActive verifies TypeAlreadyExistsFault on an active type.
func TestUndeprecateWorkflowType_AlreadyActive(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf", "1.0", "", swf.WorkflowTypeDefaults{}))

	err := b.UndeprecateWorkflowType("dom", "wf", "1.0")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrTypeAlreadyExists)
}

// TestDeleteWorkflowType verifies DeleteWorkflowType requires the type to be
// deprecated first (real AWS: "Prior to deletion, workflow types must first
// be deprecated"), removes it from the registered-types table on success, and
// leaves it visible to ListWorkflowTypes/DescribeWorkflowType afterward as
// not-found -- while executions started under the (now-deleted) type are
// unaffected, since DeleteWorkflowType never touches the executions table.
func TestDeleteWorkflowType(t *testing.T) {
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
			require.NoError(t, b.RegisterWorkflowType("dom", "wf1", "1.0", "", swf.WorkflowTypeDefaults{}))

			if tt.deprecateFirst {
				require.NoError(t, b.DeprecateWorkflowType("dom", "wf1", "1.0"))
			}

			err := b.DeleteWorkflowType("dom", "wf1", "1.0")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				_, describeErr := b.DescribeWorkflowType("dom", "wf1", "1.0")
				require.NoError(t, describeErr, "type must remain registered when delete is rejected")

				return
			}
			require.NoError(t, err)

			_, err = b.DescribeWorkflowType("dom", "wf1", "1.0")
			assert.ErrorIs(t, err, swf.ErrNotFound)
		})
	}
}

// TestDeleteWorkflowType_NotFound verifies deleting an unregistered workflow
// type fails with UnknownResourceFault.
func TestDeleteWorkflowType_NotFound(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	err := b.DeleteWorkflowType("dom", "nonexistent", "1.0")
	require.ErrorIs(t, err, swf.ErrNotFound)
}
