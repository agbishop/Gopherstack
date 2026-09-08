package cloudfront_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_FunctionConfigQuantities_WrongCode covers gopherstack-lmkr: CreateFunction,
// UpdateFunction, CreateConnectionFunction and UpdateConnectionFunction all run
// FunctionConfig.KeyValueStoreAssociations through the shared Quantity/Items walker, but
// unlike CreateDistribution/CreateCachePolicy/etc. none of these four ops' own SDK error
// deserializer declares InconsistentQuantities (cloudfront@v1.67.4 deserializers.go) --
// only InvalidArgument. A real client sending a mismatched KeyValueStoreAssociations
// therefore got the wrong wire code. The control cases prove the walker still accepts a
// consistent Quantity/Items pair (and, for real CloudFront config bodies elsewhere in this
// package, still emits InconsistentQuantities -- see Test_InconsistentQuantities_EndToEnd).
func Test_FunctionConfigQuantities_WrongCode(t *testing.T) {
	t.Parallel()

	const mismatchKVSA = `<KeyValueStoreAssociations>` +
		`<Quantity>2</Quantity>` +
		`<Items><KeyValueStoreAssociation>` +
		`<KeyValueStoreARN>arn:aws:cloudfront::123456789012:key-value-store/kvs-1</KeyValueStoreARN>` +
		`</KeyValueStoreAssociation></Items>` +
		`</KeyValueStoreAssociations>`
	const matchKVSA = `<KeyValueStoreAssociations>` +
		`<Quantity>1</Quantity>` +
		`<Items><KeyValueStoreAssociation>` +
		`<KeyValueStoreARN>arn:aws:cloudfront::123456789012:key-value-store/kvs-1</KeyValueStoreARN>` +
		`</KeyValueStoreAssociation></Items>` +
		`</KeyValueStoreAssociations>`

	t.Run("create_function_mismatch_invalid_argument_not_created", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		body := []byte(`<CreateFunctionRequest><Name>fn-mismatch</Name>` +
			`<FunctionConfig><Comment>c</Comment><Runtime>cloudfront-js-2.0</Runtime>` +
			mismatchKVSA + `</FunctionConfig>` +
			`<FunctionCode>ZnVuY3Rpb24gaGFuZGxlcigpIHt9</FunctionCode></CreateFunctionRequest>`)

		rec := doXML(t, h, http.MethodPost, "/2020-05-31/function", body)

		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "InvalidArgument")
		assert.NotContains(t, rec.Body.String(), "InconsistentQuantities")
		assert.Empty(t, h.Backend.ListFunctions(), "mismatch must not create the function")
	})

	t.Run("create_function_match_control_created", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		body := []byte(`<CreateFunctionRequest><Name>fn-match</Name>` +
			`<FunctionConfig><Comment>c</Comment><Runtime>cloudfront-js-2.0</Runtime>` +
			matchKVSA + `</FunctionConfig>` +
			`<FunctionCode>ZnVuY3Rpb24gaGFuZGxlcigpIHt9</FunctionCode></CreateFunctionRequest>`)

		rec := doXML(t, h, http.MethodPost, "/2020-05-31/function", body)

		assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.Len(t, h.Backend.ListFunctions(), 1)
	})

	t.Run("update_function_mismatch_invalid_argument_not_mutated", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		created, err := h.Backend.CreateFunction("fn-upd", "orig", "cloudfront-js-2.0", "code-v1", nil)
		require.NoError(t, err)

		body := []byte(`<UpdateFunctionRequest>` +
			`<FunctionConfig><Comment>new</Comment><Runtime>cloudfront-js-2.0</Runtime>` +
			mismatchKVSA + `</FunctionConfig>` +
			`<FunctionCode>bmV3LWNvZGU=</FunctionCode></UpdateFunctionRequest>`)

		rec := doXMLWithHeaders(t, h, http.MethodPut, "/2020-05-31/function/fn-upd", body,
			map[string]string{"If-Match": created.ETag})

		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "InvalidArgument")
		assert.NotContains(t, rec.Body.String(), "InconsistentQuantities")

		after, getErr := h.Backend.GetFunction("fn-upd")
		require.NoError(t, getErr)
		assert.Equal(t, created.ETag, after.ETag, "mismatch must not mutate the function")
		assert.Equal(t, "orig", after.Comment)
	})

	t.Run("create_connection_function_mismatch_invalid_argument_not_created", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		body := []byte(`<CreateConnectionFunctionRequest><Name>cfn-mismatch</Name>` +
			`<ConnectionFunctionConfig><Comment>c</Comment><Runtime>cloudfront-js-2.0</Runtime>` +
			mismatchKVSA + `</ConnectionFunctionConfig>` +
			`<ConnectionFunctionCode>ZnVuY3Rpb24gaGFuZGxlcigpIHt9</ConnectionFunctionCode>` +
			`</CreateConnectionFunctionRequest>`)

		rec := doXML(t, h, http.MethodPost, "/2020-05-31/connection-function", body)

		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "InvalidArgument")
		assert.NotContains(t, rec.Body.String(), "InconsistentQuantities")
		assert.Empty(t, h.Backend.ListConnectionFunctions(), "mismatch must not create the connection function")
	})

	t.Run("update_connection_function_mismatch_invalid_argument_not_mutated", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		created, err := h.Backend.CreateConnectionFunctionWithCode(
			"cfn-upd", "orig", "cloudfront-js-2.0", []byte("code-v1"), nil,
		)
		require.NoError(t, err)

		body := []byte(`<UpdateConnectionFunctionRequest>` +
			`<ConnectionFunctionConfig><Comment>new</Comment><Runtime>cloudfront-js-2.0</Runtime>` +
			mismatchKVSA + `</ConnectionFunctionConfig>` +
			`<ConnectionFunctionCode>bmV3LWNvZGU=</ConnectionFunctionCode></UpdateConnectionFunctionRequest>`)

		rec := doXMLWithHeaders(t, h, http.MethodPut, "/2020-05-31/connection-function/"+created.ID, body,
			map[string]string{"If-Match": created.ETag})

		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "InvalidArgument")
		assert.NotContains(t, rec.Body.String(), "InconsistentQuantities")

		after, getErr := h.Backend.GetConnectionFunction(created.ID)
		require.NoError(t, getErr)
		assert.Equal(t, created.ETag, after.ETag, "mismatch must not mutate the connection function")
		assert.Equal(t, "orig", after.Comment)
	})
}
