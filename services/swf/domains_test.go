package swf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/swf"
)

func TestRegisterDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		domain      string
		description string
		retention   string
		wantName    string
		wantStatus  string
		preRegister []string
		wantCount   int
	}{
		{
			name:        "success",
			domain:      "my-domain",
			description: "test domain",
			retention:   "30",
			wantCount:   1,
			wantName:    "my-domain",
			wantStatus:  "REGISTERED",
		},
		{
			name:        "success_none_retention",
			domain:      "my-domain2",
			description: "",
			retention:   "NONE",
			wantCount:   1,
			wantName:    "my-domain2",
			wantStatus:  "REGISTERED",
		},
		{
			name:        "AlreadyExists",
			preRegister: []string{"my-domain"},
			domain:      "my-domain",
			wantErr:     swf.ErrAlreadyExists,
		},
		{
			name:      "invalid_retention",
			domain:    "bad-ret",
			retention: "999",
			wantErr:   swf.ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := swf.NewInMemoryBackend()
			for _, d := range tt.preRegister {
				require.NoError(t, b.RegisterDomain(d, "", "NONE"))
			}

			err := b.RegisterDomain(tt.domain, tt.description, tt.retention)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)

			domains, err := b.ListDomains("REGISTERED")
			require.NoError(t, err)
			require.Len(t, domains, tt.wantCount)
			assert.Equal(t, tt.wantName, domains[0].Name)
			assert.Equal(t, tt.wantStatus, domains[0].Status)
		})
	}
}

// TestRegisterDomain_ErrValidation verifies empty name returns ErrValidation.
func TestRegisterDomain_ErrValidation(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	err := b.RegisterDomain("", "desc", "NONE")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrValidation)
}

func TestRegisterDomain_RetentionStoredAndReturned(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("ret-dom", "desc", "45"))

	d, err := b.DescribeDomain("ret-dom")
	require.NoError(t, err)
	assert.Equal(t, "45", d.WorkflowExecutionRetentionPeriodInDays)
}

func TestDeprecateDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		domain      string
		wantCount   int
		preRegister bool
	}{
		{
			name:        "success",
			preRegister: true,
			domain:      "my-domain",
			wantCount:   1,
		},
		{
			name:    "NotFound",
			domain:  "nonexistent",
			wantErr: swf.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := swf.NewInMemoryBackend()
			if tt.preRegister {
				require.NoError(t, b.RegisterDomain(tt.domain, "", "NONE"))
			}

			err := b.DeprecateDomain(tt.domain)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)

			domains, err := b.ListDomains("DEPRECATED")
			require.NoError(t, err)
			require.Len(t, domains, tt.wantCount)
		})
	}
}

// TestDeprecateDomain_AlreadyDeprecated checks double-deprecate returns ErrDeprecated.
func TestDeprecateDomain_AlreadyDeprecated(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.DeprecateDomain("dom"))

	err := b.DeprecateDomain("dom")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrDeprecated)
}

// TestRegisterDomain_DeprecatedDomain checks that re-registering a deprecated
// domain returns ErrAlreadyExists (DomainAlreadyExistsFault), not ErrDeprecated
// (DomainDeprecatedFault) -- per aws-sdk-go-v2's DomainAlreadyExistsFault doc
// comment: "You may get this fault if you are registering a domain that is
// either already registered or deprecated". RegisterDomain's modelled error
// set has no DomainDeprecatedFault at all.
func TestRegisterDomain_DeprecatedDomain(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.DeprecateDomain("dom"))

	err := b.RegisterDomain("dom", "", "NONE")

	require.Error(t, err)
	require.ErrorIs(t, err, swf.ErrAlreadyExists)
	assert.NotErrorIs(t, err, swf.ErrDeprecated)
}

// TestDeprecateDomain_CascadesToRegisteredTypes verifies the real AWS
// documented behavior ("Deprecating a domain also deprecates all activity
// and workflow types registered in the domain") -- every REGISTERED
// workflow/activity type in the domain must flip to DEPRECATED, while an
// already-open execution using one of those types keeps running untouched
// ("Executions that were started before the domain was deprecated continue
// to run").
func TestDeprecateDomain_CascadesToRegisteredTypes(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf-type", "1.0", "", swf.WorkflowTypeDefaults{}))
	require.NoError(t, b.RegisterActivityType("dom", "act-type", "1.0", "", swf.ActivityTypeDefaults{}))
	// A type already deprecated before the domain-level deprecate must not
	// error or otherwise change state.
	require.NoError(t, b.RegisterWorkflowType("dom", "already-deprecated", "1.0", "", swf.WorkflowTypeDefaults{}))
	require.NoError(t, b.DeprecateWorkflowType("dom", "already-deprecated", "1.0"))

	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:              "dom",
		WorkflowID:          "wf-1",
		WorkflowTypeName:    "wf-type",
		WorkflowTypeVersion: "1.0",
		TaskList:            "tasks",
	})
	require.NoError(t, err)

	require.NoError(t, b.DeprecateDomain("dom"))

	wt, err := b.DescribeWorkflowType("dom", "wf-type", "1.0")
	require.NoError(t, err)
	assert.Equal(t, "DEPRECATED", wt.Status)

	at, err := b.DescribeActivityType("dom", "act-type", "1.0")
	require.NoError(t, err)
	assert.Equal(t, "DEPRECATED", at.Status)

	alreadyDeprecated, err := b.DescribeWorkflowType("dom", "already-deprecated", "1.0")
	require.NoError(t, err)
	assert.Equal(t, "DEPRECATED", alreadyDeprecated.Status)

	// The already-running execution must be untouched by the cascade.
	exec, err := b.DescribeWorkflowExecution("dom", "wf-1", "")
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", exec.Status)
}

func TestListDomains(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("d1", "", "NONE"))
	require.NoError(t, b.RegisterDomain("d2", "", "NONE"))

	all, err := b.ListDomains("")
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestListDomains_InvalidStatus(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	_, err := b.ListDomains("INVALID")
	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrValidation)
}

// TestUndeprecateDomain verifies round-trip deprecate→undeprecate.
func TestUndeprecateDomain(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.DeprecateDomain("dom"))
	require.NoError(t, b.UndeprecateDomain("dom"))

	d, err := b.DescribeDomain("dom")
	require.NoError(t, err)
	assert.Equal(t, "REGISTERED", d.Status)
}

// TestUndeprecateDomain_NotFound returns ErrNotFound for missing domain.
func TestUndeprecateDomain_NotFound(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	err := b.UndeprecateDomain("missing")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrNotFound)
}

// TestUndeprecateDomain_AlreadyRegistered returns ErrAlreadyExists if not deprecated.
func TestUndeprecateDomain_AlreadyRegistered(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	err := b.UndeprecateDomain("dom")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrAlreadyExists)
}

// TestUndeprecateDomain_AlreadyActive verifies DomainAlreadyExistsFault on an
// active (never-deprecated) domain.
func TestUndeprecateDomain_AlreadyActive(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	err := b.UndeprecateDomain("dom")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrAlreadyExists)
}
