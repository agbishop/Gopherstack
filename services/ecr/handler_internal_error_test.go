package ecr //nolint:testpackage // needs access to the unexported classifyError method.

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

var errUnmatchedForTest = errors.New("boom: matches none of classifyError's sentinel checks")

// TestClassifyError_DefaultBranchEmitsServerException is a white-box test
// of classifyError's default branch: ecr@v1.60.4 types/errors.go models
// "ServerException" (ErrorFault: FaultServer, doc comment "These errors are
// usually caused by a server-side issue") as the service's 5xx fault, wired
// into all 58 of ecr's operation deserializers (confirmed via
// deserializers.go's error-code switches), so any backend error not
// classified under one of the enumerated sentinel/not-found groups must
// surface that code, not the unmodeled "InternalServerError" this branch
// returned before the fix (gopherstack-o7gx).
//
// classifyError's default is reachable only when a backend error doesn't
// match any of the enumerated sentinels, errUnknownAction, or a JSON
// syntax/type error (itself mapped to InvalidParameterException here); no
// currently-wired dispatch path leaves an error unclassified, so there is
// no legitimately-constructed real SDK client request that reaches this
// branch today. This test drives classifyError directly with a synthetic
// unmatched error to pin the wire-level contract regardless.
func TestClassifyError_DefaultBranchEmitsServerException(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	status, code := h.classifyError(errUnmatchedForTest)

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "ServerException", code)
}

// TestClassifyError_NotFoundGroupEmitsSpecificType is a white-box test
// proving classifyError's not-found group emits each sentinel's own AWS
// exception name rather than a shared generic string. ecr@v1.60.4's
// deserializers.go error-code switches (e.g. line 1124 for
// DeleteLifecyclePolicy) dispatch on the literal codes
// "LifecyclePolicyNotFoundException", "PullThroughCacheRuleNotFoundException",
// "RegistryPolicyNotFoundException", and "TemplateNotFoundException" — none
// of them recognise the plain "NotFoundException" gopherstack previously sent
// for all four. A real SDK client hitting that mismatch falls through the
// deserializer's default case to a generic smithy.GenericAPIError instead of
// the documented typed exception, so errors.As against e.g.
// *types.LifecyclePolicyNotFoundException never matches.
func TestClassifyError_NotFoundGroupEmitsSpecificType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err      error
		name     string
		wantType string
	}{
		{
			name:     "pull through cache rule not found",
			err:      ErrPullThroughCacheRuleNotFound,
			wantType: "PullThroughCacheRuleNotFoundException",
		},
		{
			name:     "lifecycle policy not found",
			err:      ErrLifecyclePolicyNotFound,
			wantType: "LifecyclePolicyNotFoundException",
		},
		{
			name:     "repository creation template not found",
			err:      ErrRepositoryCreationTemplateNotFound,
			wantType: "TemplateNotFoundException",
		},
		{
			name:     "registry policy not found",
			err:      ErrRegistryPolicyNotFound,
			wantType: "RegistryPolicyNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := &Handler{}

			_, code := h.classifyError(tt.err)

			assert.Equal(t, tt.wantType, code)
		})
	}
}
