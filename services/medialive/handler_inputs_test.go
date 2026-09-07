package medialive_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	medialivesdk "github.com/aws/aws-sdk-go-v2/service/medialive"
	medialivetypes "github.com/aws/aws-sdk-go-v2/service/medialive/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/medialive"
)

func TestInput_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	inputID := createTestInput(t, h)

	assert.Equal(t, 1, medialive.InputCount(h.Backend.(*medialive.InMemoryBackend)))

	// Describe
	rec := doRequest(t, h, http.MethodGet, "/prod/inputs/"+inputID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "test-input", descResp["name"])
	assert.Equal(t, "DETACHED", descResp["state"])
	assert.Contains(t, descResp["arn"], "arn:aws:medialive:us-east-1:000000000000:input:")

	// Update
	rec = doRequest(t, h, http.MethodPut, "/prod/inputs/"+inputID, map[string]any{
		"name": "updated-input",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	inp := updateResp["input"].(map[string]any)
	assert.Equal(t, "updated-input", inp["name"])

	// List
	rec = doRequest(t, h, http.MethodGet, "/prod/inputs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Len(t, listResp["inputs"], 1)

	// Delete: real terraform-provider-aws's waitInputDeleted polls
	// DescribeInput for state InputStateDeleted (internal/service/medialive/
	// input.go), so the record must stay describable with state DELETED
	// rather than vanish -- an immediate 404 reads as "not yet observed",
	// not "done", and the waiter exhausts its retries.
	rec = doRequest(t, h, http.MethodDelete, "/prod/inputs/"+inputID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, medialive.InputCount(h.Backend.(*medialive.InMemoryBackend)))

	rec = doRequest(t, h, http.MethodGet, "/prod/inputs/"+inputID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var deletedResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &deletedResp))
	assert.Equal(t, "DELETED", deletedResp["state"])
}

// TestInput_SdiSources_CreateAndUpdate covers gopherstack-ir0p: CreateInput
// and UpdateInput both carry SdiSources (api_op_CreateInput.go,
// api_op_UpdateInput.go), and types.Input.SdiSources means DescribeInput
// must echo whatever was attached, but the handlers never parsed the field.
func TestInput_SdiSources_CreateAndUpdate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// No documented default in CreateInputInput.SdiSources's doc comment;
	// an omitted field yields an empty list.
	rec := doRequest(t, h, http.MethodPost, "/prod/inputs", map[string]any{
		"name": "no-sdi", "type": "UDP_PUSH",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	noSdi := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	assert.Empty(t, noSdi["sdiSources"])

	// Create with two distinct SdiSources: DescribeInput must report the
	// exact list, not merely "non-empty".
	rec = doRequest(t, h, http.MethodPost, "/prod/inputs", map[string]any{
		"name": "with-sdi", "type": "UDP_PUSH",
		"sdiSources": []any{"sdi-aaa", "sdi-bbb"},
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	created := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	inputID := created["id"].(string)
	assert.Equal(t, []any{"sdi-aaa", "sdi-bbb"}, created["sdiSources"])

	rec = doRequest(t, h, http.MethodGet, "/prod/inputs/"+inputID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	desc := decodeBody(t, rec.Body.Bytes())
	assert.Equal(t, []any{"sdi-aaa", "sdi-bbb"}, desc["sdiSources"])

	// Update to a wholly different pair: a partial-replace bug (e.g. only
	// overwriting index 0, or appending instead of replacing) would leave a
	// survivor from the original list.
	rec = doRequest(t, h, http.MethodPut, "/prod/inputs/"+inputID, map[string]any{
		"sdiSources": []any{"sdi-ccc", "sdi-ddd"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	updated := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	assert.Equal(t, []any{"sdi-ccc", "sdi-ddd"}, updated["sdiSources"])

	rec = doRequest(t, h, http.MethodGet, "/prod/inputs/"+inputID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	desc = decodeBody(t, rec.Body.Bytes())
	assert.Equal(t, []any{"sdi-ccc", "sdi-ddd"}, desc["sdiSources"])
}

// TestInput_SdiSources_UpdateWithoutFieldPreservesList pins the fix for the
// data-loss regression found in review: UpdateInput must not touch
// SdiSources when the caller's body doesn't mention the key at all. Neither
// api_op_UpdateInput.go's SdiSources doc comment nor any other AWS source
// documents this as a tri-state -- the guard exists because an unconditional
// replace would silently wipe attachments on a plain rename, which is a bug
// independent of what the doc omits, and it matches how this same handler
// already treats an absent name or roleArn.
func TestInput_SdiSources_UpdateWithoutFieldPreservesList(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/inputs", map[string]any{
		"name": "keep-sdi", "type": "UDP_PUSH",
		"sdiSources": []any{"sdi-aaa", "sdi-bbb"},
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	created := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	inputID := created["id"].(string)

	// Update mentions only "name" -- no "sdiSources" key anywhere in the body.
	rec = doRequest(t, h, http.MethodPut, "/prod/inputs/"+inputID, map[string]any{
		"name": "renamed",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	updated := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	assert.Equal(t, "renamed", updated["name"])
	assert.Equal(t, []any{"sdi-aaa", "sdi-bbb"}, updated["sdiSources"])

	rec = doRequest(t, h, http.MethodGet, "/prod/inputs/"+inputID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	desc := decodeBody(t, rec.Body.Bytes())
	assert.Equal(t, []any{"sdi-aaa", "sdi-bbb"}, desc["sdiSources"])
}

// TestInput_SdiSources_UpdateWithExplicitEmptyClearsList distinguishes the
// no-change case above from a caller-intended clear: an explicit
// "sdiSources": [] is present in the body and must replace the list with
// empty, not be conflated with an absent key.
func TestInput_SdiSources_UpdateWithExplicitEmptyClearsList(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/inputs", map[string]any{
		"name": "clear-sdi", "type": "UDP_PUSH",
		"sdiSources": []any{"sdi-aaa", "sdi-bbb"},
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	created := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	inputID := created["id"].(string)

	rec = doRequest(t, h, http.MethodPut, "/prod/inputs/"+inputID, map[string]any{
		"sdiSources": []any{},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	updated := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	assert.Equal(t, []any{}, updated["sdiSources"])

	rec = doRequest(t, h, http.MethodGet, "/prod/inputs/"+inputID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	desc := decodeBody(t, rec.Body.Bytes())
	assert.Equal(t, []any{}, desc["sdiSources"])
}

func TestInput_MissingName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/prod/inputs", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListInputs_Empty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/prod/inputs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp["inputs"])
}

func TestCreatePartnerInput(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/inputs", map[string]any{
		"name": "primary", "type": "UDP_PUSH",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	inputID := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)["id"].(string)

	rec = doRequest(t, h, http.MethodPost, "/prod/inputs/"+inputID+"/partners", map[string]any{})
	require.Equal(t, http.StatusCreated, rec.Code)
	partner := decodeBody(t, rec.Body.Bytes())["input"].(map[string]any)
	assert.NotEmpty(t, partner["id"])
	assert.NotEqual(t, inputID, partner["id"])

	rec = doRequest(t, h, http.MethodPost, "/prod/inputs/missing/partners", map[string]any{})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestDeleteInput_RealClient_StaysDescribableAsDeleted drives Create/Delete/
// Describe through the real aws-sdk-go-v2 medialive client, mirroring
// terraform-provider-aws's waitInputDeleted (internal/service/medialive/
// input.go): it polls DescribeInput expecting InputStateDeleted, and a
// NotFoundException does not satisfy that target -- it reads as "not found
// yet", so the provider burns its retry budget and reports "couldn't find
// resource" instead of succeeding. Before the fix, DeleteInput removed the
// record outright and this DescribeInput call 404'd.
func TestDeleteInput_RealClient_StaysDescribableAsDeleted(t *testing.T) {
	t.Parallel()

	backend := medialive.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestMediaLiveClient(t, medialive.NewHandler(backend))
	ctx := t.Context()

	created, err := client.CreateInput(ctx, &medialivesdk.CreateInputInput{
		Name: aws.String("delete-fix-input"),
		Type: medialivetypes.InputTypeRtmpPush,
		Destinations: []medialivetypes.InputDestinationRequest{
			{StreamName: aws.String("live/stream")},
		},
	})
	require.NoError(t, err)

	_, err = client.DeleteInput(ctx, &medialivesdk.DeleteInputInput{InputId: created.Input.Id})
	require.NoError(t, err)

	desc, err := client.DescribeInput(ctx, &medialivesdk.DescribeInputInput{InputId: created.Input.Id})
	require.NoError(t, err, "the waiter's Target state can only be observed if DescribeInput keeps succeeding")
	assert.Equal(t, medialivetypes.InputStateDeleted, desc.State)
}
