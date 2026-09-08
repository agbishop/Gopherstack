package elasticache

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// errTestBackendUnavailable and errTestNotFoundSentinel are static test-only
// errors for TestDescribeListChecked_BackendError_DoesNotDoubleWrite.
var (
	errTestBackendUnavailable = errors.New("boom: backend unavailable")
	errTestNotFoundSentinel   = errors.New("distinct notFound sentinel")
)

// TestDescribeListChecked_BackendError_DoesNotDoubleWrite pins
// describeListChecked's InternalFailure branch (handler.go, the default case
// sibling to the notFound branch pinned by
// TestDescribeListChecked_NotFound_DoesNotDoubleWrite in handler_error_test.go).
// None of describeListChecked's ~7 real callers can currently reach this
// branch: each wraps pkgs/store's describePaged (via pagination.go), whose
// only non-nil return is the caller's notFound sentinel, so there is no live
// wire-level test for it. This drives describeListChecked directly, white-box,
// with a synthetic backend error to pin the branch anyway.
func TestDescribeListChecked_BackendError_DoesNotDoubleWrite(t *testing.T) {
	t.Parallel()

	e := echo.New()
	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, err := describeListChecked(c, form,
		func(string, int) (page.Page[CacheSubnetGroup], error) {
			return page.Page[CacheSubnetGroup]{}, errTestBackendUnavailable
		},
		errTestNotFoundSentinel, http.StatusBadRequest, "SomeNotFoundCode", "not found message")

	require.ErrorIs(t, err, errResponseWritten)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "InternalFailure")
	assert.Contains(t, body, "boom: backend unavailable")
	assert.Equal(t, 1, strings.Count(body, "<ErrorResponse"),
		"exactly one error document must be written, not a second write appended")
}
