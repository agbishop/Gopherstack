package lambda_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/lambda"
)

// TestDeleteFunction_ClearsSideState verifies that DeleteFunction fully cleans up
// per-function state (aliases, resource-policy permissions, reserved concurrency)
// so that recreating a function with the same name -- a routine CI/CD redeploy --
// does not inherit stale state from the deleted function. Previously DeleteFunction
// cleared only b.functions, b.runtimes, and the event-source-mapping cascade, so
// principals granted access via AddPermission retained invoke access to the new
// function.
func TestDeleteFunction_ClearsSideState(t *testing.T) {
	t.Parallel()

	h, _ := newInMemoryHandler(t)
	const fnName = "ghost-row-fn"

	createFunctionForTest(t, h, fnName)

	pubRec := callInMemoryHandler(t, h, http.MethodPost,
		"/2015-03-31/functions/"+fnName+"/versions", `{}`)
	require.Equal(t, http.StatusCreated, pubRec.Code)

	aliasRec := callInMemoryHandler(t, h, http.MethodPost,
		"/2015-03-31/functions/"+fnName+"/aliases",
		`{"Name":"prod","FunctionVersion":"1"}`)
	require.Equal(t, http.StatusCreated, aliasRec.Code)

	permRec := callInMemoryHandler(t, h, http.MethodPost,
		"/2015-03-31/functions/"+fnName+"/policy",
		`{"StatementId":"AllowInvoke","Action":"lambda:InvokeFunction","Principal":"s3.amazonaws.com"}`)
	require.Equal(t, http.StatusCreated, permRec.Code)

	concRec := callInMemoryHandler(t, h, http.MethodPut,
		"/2017-10-31/functions/"+fnName+"/concurrency",
		`{"ReservedConcurrentExecutions":5}`)
	require.Equal(t, http.StatusOK, concRec.Code)

	delRec := callInMemoryHandler(t, h, http.MethodDelete,
		"/2015-03-31/functions/"+fnName, "")
	require.Equal(t, http.StatusNoContent, delRec.Code)

	createFunctionForTest(t, h, fnName)

	policyRec := callInMemoryHandler(t, h, http.MethodGet,
		"/2015-03-31/functions/"+fnName+"/policy", "")
	assert.Equal(t, http.StatusNotFound, policyRec.Code,
		"recreated function must not inherit the deleted function's resource-policy statements")

	aliasListRec := callInMemoryHandler(t, h, http.MethodGet,
		"/2015-03-31/functions/"+fnName+"/aliases", "")
	require.Equal(t, http.StatusOK, aliasListRec.Code)

	var aliasOut lambda.ListAliasesOutput
	require.NoError(t, json.NewDecoder(aliasListRec.Body).Decode(&aliasOut))
	assert.Empty(t, aliasOut.Aliases,
		"recreated function must not inherit the deleted function's aliases")

	concGetRec := callInMemoryHandler(t, h, http.MethodGet,
		"/2019-09-30/functions/"+fnName+"/concurrency", "")
	assert.Equal(t, http.StatusNotFound, concGetRec.Code,
		"recreated function must not inherit the deleted function's reserved concurrency")
}
