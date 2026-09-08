package omics_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeleteRun_RequiresTerminalState verifies real AWS DeleteRun semantics
// (api_op_DeleteRun.go: "You can only delete a run that has reached a
// COMPLETED, FAILED, or CANCELLED stage."): a freshly-started run (PENDING)
// must not be deletable, but a CANCELLED run must be.
func TestDeleteRun_RequiresTerminalState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	startRec := doRequest(t, h, http.MethodPost, "/run", map[string]any{
		"workflowId": "wf123",
		"roleArn":    "arn:aws:iam::000000000000:role/role",
		"name":       "run-to-delete",
	})
	require.Equal(t, http.StatusCreated, startRec.Code)

	var startResp map[string]any
	require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &startResp))
	runID := startResp["id"].(string)

	delRec := doRequest(t, h, http.MethodDelete, "/run/"+runID, nil)
	assert.Equal(t, http.StatusConflict, delRec.Code, "a PENDING run must not be deletable")

	getRec := doRequest(t, h, http.MethodGet, "/run/"+runID, nil)
	assert.Equal(t, http.StatusOK, getRec.Code, "the run must survive the rejected delete")

	cancelRec := doRequest(t, h, http.MethodPost, "/run/"+runID+"/cancel", nil)
	require.Equal(t, http.StatusOK, cancelRec.Code)

	delRec2 := doRequest(t, h, http.MethodDelete, "/run/"+runID, nil)
	assert.Equal(t, http.StatusOK, delRec2.Code, "CANCELLED is a terminal state, delete must now succeed")
}

// TestDeleteReferenceStore_RequiresEmpty verifies real AWS DeleteReferenceStore
// semantics (api_op_DeleteReferenceStore.go: "You can only delete a reference
// store when it does not contain any reference genomes."): a store with a
// reference must not be deletable until the reference is removed.
func TestDeleteReferenceStore_RequiresEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	storeRec := doRequest(t, h, http.MethodPost, "/referencestore", map[string]any{"name": "store-1"})
	require.Equal(t, http.StatusCreated, storeRec.Code)

	var store map[string]any
	require.NoError(t, json.Unmarshal(storeRec.Body.Bytes(), &store))
	storeID := store["id"].(string)

	jobRec := doRequest(t, h, http.MethodPost, "/referencestore/"+storeID+"/importjob", map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/role",
		"sources": []map[string]any{{"sourceFile": "s3://bucket/ref.fa", "name": "ref-1"}},
	})
	require.Equal(t, http.StatusCreated, jobRec.Code)

	listRec := doRequest(t, h, http.MethodPost, "/referencestore/"+storeID+"/references", nil)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	refs, ok := listResp["references"].([]any)
	require.True(t, ok)
	require.Len(t, refs, 1)
	refID := refs[0].(map[string]any)["id"].(string)

	delRec := doRequest(t, h, http.MethodDelete, "/referencestore/"+storeID, nil)
	assert.Equal(t, http.StatusConflict, delRec.Code, "a store still containing a reference must not be deletable")

	getRec := doRequest(t, h, http.MethodGet, "/referencestore/"+storeID, nil)
	assert.Equal(t, http.StatusOK, getRec.Code, "the store must survive the rejected delete")

	delRefRec := doRequest(t, h, http.MethodDelete, "/referencestore/"+storeID+"/reference/"+refID, nil)
	require.Equal(t, http.StatusOK, delRefRec.Code)

	delRec2 := doRequest(t, h, http.MethodDelete, "/referencestore/"+storeID, nil)
	assert.Equal(t, http.StatusOK, delRec2.Code, "an empty store must now be deletable")
}

// TestDeleteSequenceStore_RequiresEmpty verifies real AWS DeleteSequenceStore
// semantics (api_op_DeleteSequenceStore.go: "You can only delete a sequence
// store when it does not contain any read sets."): a store with a read set
// must not be deletable until the read set is removed via BatchDeleteReadSet.
func TestDeleteSequenceStore_RequiresEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	storeRec := doRequest(t, h, http.MethodPost, "/sequencestore", map[string]any{"name": "store-1"})
	require.Equal(t, http.StatusCreated, storeRec.Code)

	var store map[string]any
	require.NoError(t, json.Unmarshal(storeRec.Body.Bytes(), &store))
	storeID := store["id"].(string)

	jobRec := doRequest(t, h, http.MethodPost, "/sequencestore/"+storeID+"/importjob", map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/role",
		"sources": []map[string]any{{"sourceFileType": "BAM", "name": "read-1"}},
	})
	require.Equal(t, http.StatusCreated, jobRec.Code)

	listRec := doRequest(t, h, http.MethodPost, "/sequencestore/"+storeID+"/readsets", nil)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	readSets, ok := listResp["readSets"].([]any)
	require.True(t, ok)
	require.Len(t, readSets, 1)
	readSetID := readSets[0].(map[string]any)["id"].(string)

	delRec := doRequest(t, h, http.MethodDelete, "/sequencestore/"+storeID, nil)
	assert.Equal(t, http.StatusConflict, delRec.Code, "a store still containing a read set must not be deletable")

	getRec := doRequest(t, h, http.MethodGet, "/sequencestore/"+storeID, nil)
	assert.Equal(t, http.StatusOK, getRec.Code, "the store must survive the rejected delete")

	delReadSetRec := doRequest(t, h, http.MethodPost, "/sequencestore/"+storeID+"/readset/batch/delete",
		map[string]any{"ids": []string{readSetID}})
	require.Equal(t, http.StatusOK, delReadSetRec.Code)

	delRec2 := doRequest(t, h, http.MethodDelete, "/sequencestore/"+storeID, nil)
	assert.Equal(t, http.StatusOK, delRec2.Code, "an empty store must now be deletable")
}

// TestDeleteReference_RequiresNoAssociatedReadSet verifies real AWS
// DeleteReference semantics (api_op_DeleteReference.go: "The read set
// associated with the reference genome must first be deleted before deleting
// the reference genome."): a reference with an associated read set must not
// be deletable until that read set is removed.
func TestDeleteReference_RequiresNoAssociatedReadSet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	refStoreRec := doRequest(t, h, http.MethodPost, "/referencestore", map[string]any{"name": "ref-store-1"})
	require.Equal(t, http.StatusCreated, refStoreRec.Code)

	var refStore map[string]any
	require.NoError(t, json.Unmarshal(refStoreRec.Body.Bytes(), &refStore))
	refStoreID := refStore["id"].(string)

	refJobRec := doRequest(t, h, http.MethodPost, "/referencestore/"+refStoreID+"/importjob", map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/role",
		"sources": []map[string]any{{"sourceFile": "s3://bucket/ref.fa", "name": "ref-1"}},
	})
	require.Equal(t, http.StatusCreated, refJobRec.Code)

	refListRec := doRequest(t, h, http.MethodPost, "/referencestore/"+refStoreID+"/references", nil)
	require.Equal(t, http.StatusOK, refListRec.Code)

	var refListResp map[string]any
	require.NoError(t, json.Unmarshal(refListRec.Body.Bytes(), &refListResp))
	refs, ok := refListResp["references"].([]any)
	require.True(t, ok)
	require.Len(t, refs, 1)
	ref := refs[0].(map[string]any)
	refID := ref["id"].(string)
	refArn := ref["arn"].(string)
	require.NotEmpty(t, refArn)

	seqStoreRec := doRequest(t, h, http.MethodPost, "/sequencestore", map[string]any{"name": "seq-store-1"})
	require.Equal(t, http.StatusCreated, seqStoreRec.Code)

	var seqStore map[string]any
	require.NoError(t, json.Unmarshal(seqStoreRec.Body.Bytes(), &seqStore))
	seqStoreID := seqStore["id"].(string)

	readSetJobRec := doRequest(t, h, http.MethodPost, "/sequencestore/"+seqStoreID+"/importjob", map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/role",
		"sources": []map[string]any{{
			"sourceFileType": "BAM",
			"name":           "read-1",
			"referenceArn":   refArn,
		}},
	})
	require.Equal(t, http.StatusCreated, readSetJobRec.Code)

	readSetListRec := doRequest(t, h, http.MethodPost, "/sequencestore/"+seqStoreID+"/readsets", nil)
	require.Equal(t, http.StatusOK, readSetListRec.Code)

	var readSetListResp map[string]any
	require.NoError(t, json.Unmarshal(readSetListRec.Body.Bytes(), &readSetListResp))
	readSets, ok := readSetListResp["readSets"].([]any)
	require.True(t, ok)
	require.Len(t, readSets, 1)
	readSetID := readSets[0].(map[string]any)["id"].(string)

	delRefRec := doRequest(t, h, http.MethodDelete, "/referencestore/"+refStoreID+"/reference/"+refID, nil)
	assert.Equal(t, http.StatusConflict, delRefRec.Code,
		"a reference with an associated read set must not be deletable")

	getRefRec := doRequest(t, h, http.MethodGet, "/referencestore/"+refStoreID+"/reference/"+refID+"/metadata", nil)
	assert.Equal(t, http.StatusOK, getRefRec.Code, "the reference must survive the rejected delete")

	delReadSetRec := doRequest(t, h, http.MethodPost, "/sequencestore/"+seqStoreID+"/readset/batch/delete",
		map[string]any{"ids": []string{readSetID}})
	require.Equal(t, http.StatusOK, delReadSetRec.Code)

	delRefRec2 := doRequest(t, h, http.MethodDelete, "/referencestore/"+refStoreID+"/reference/"+refID, nil)
	assert.Equal(t, http.StatusOK, delRefRec2.Code, "a reference with no associated read set must now be deletable")
}
