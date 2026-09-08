package databrew_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/databrew"
)

// ---- Project backend ----

func TestCreateProject_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	p, err := b.CreateProject(
		context.Background(),
		"my-project",
		"ds1",
		"r1",
		"arn:aws:iam::123456789012:role/Role",
		databrew.Sample{Type: "FIRST_N", Size: 500},
		map[string]string{"k": "v"},
	)
	require.NoError(t, err)
	assert.Equal(t, "my-project", p.Name)
	assert.Zero(t, p.OpenDate)
	assert.Equal(t, "v", p.Tags["k"])
	assert.NotEmpty(t, p.Arn)
}

func TestCreateProject_EmptyName(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(context.Background(), "", "ds", "r", "", databrew.Sample{}, nil)
	require.Error(t, err)
}

func TestCreateProject_Duplicate(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(context.Background(), "p", "", "r", "", databrew.Sample{}, nil)
	require.NoError(t, err)
	_, err = b.CreateProject(context.Background(), "p", "", "r", "", databrew.Sample{}, nil)
	require.Error(t, err)
}

func TestCreateProject_InvalidSampleType(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(
		context.Background(),
		"bad-p",
		"",
		"r",
		"",
		databrew.Sample{Type: "INVALID"},
		nil,
	)
	require.Error(t, err)
}

func TestDescribeProject_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(context.Background(), "proj1", "ds", "r", "", databrew.Sample{}, nil)
	require.NoError(t, err)
	p, err := b.DescribeProject(context.Background(), "proj1")
	require.NoError(t, err)
	assert.Equal(t, "proj1", p.Name)
}

func TestDescribeProject_NotFound(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.DescribeProject(context.Background(), "no-such")
	require.Error(t, err)
}

func TestListProjects(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(context.Background(), "p1", "", "r", "", databrew.Sample{}, nil)
	require.NoError(t, err)
	_, err = b.CreateProject(context.Background(), "p2", "", "r", "", databrew.Sample{}, nil)
	require.NoError(t, err)
	list, _ := b.ListProjects(context.Background(), 100, "")
	assert.Len(t, list, 2)
}

func TestUpdateProject_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(
		context.Background(),
		"upd-p",
		"old-ds",
		"r",
		"old-role",
		databrew.Sample{},
		nil,
	)
	require.NoError(t, err)
	err = b.UpdateProject(
		context.Background(),
		"upd-p",
		"new-role",
		databrew.Sample{Type: "RANDOM", Size: 100},
	)
	require.NoError(t, err)
	p, err := b.DescribeProject(context.Background(), "upd-p")
	require.NoError(t, err)
	assert.Equal(
		t,
		"old-ds",
		p.DatasetName,
		"DatasetName is not one of UpdateProjectInput's fields; it must be unchanged",
	)
	assert.Equal(t, "new-role", p.RoleArn)
}

func TestUpdateProject_NotFound(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	err := b.UpdateProject(context.Background(), "no-such", "", databrew.Sample{})
	require.Error(t, err)
}

func TestUpdateProject_InvalidSampleType(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(
		context.Background(),
		"upd-bad-p",
		"",
		"r",
		"",
		databrew.Sample{},
		nil,
	)
	require.NoError(t, err)
	err = b.UpdateProject(
		context.Background(),
		"upd-bad-p",
		"",
		databrew.Sample{Type: "INVALID"},
	)
	require.Error(t, err)
}

func TestDeleteProject_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateProject(context.Background(), "del-p", "", "r", "", databrew.Sample{}, nil)
	require.NoError(t, err)
	err = b.DeleteProject(context.Background(), "del-p")
	require.NoError(t, err)
	_, err = b.DescribeProject(context.Background(), "del-p")
	require.Error(t, err)
}

func TestDeleteProject_NotFound(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	err := b.DeleteProject(context.Background(), "no-such")
	require.Error(t, err)
}

// TestDeleteProject_UsedByJob verifies a project still referenced by a job's
// ProjectName cannot be deleted, mirroring DeleteDataset's referential-
// integrity check (see its doc comment): CreateRecipeJobInput.ProjectName
// lets a job reference a project instead of a dataset+recipe pair, and
// CreateJob's own validateJobResourceRefs already refuses to create a job
// naming a nonexistent project. ConflictException is modeled for
// DeleteProject (aws-sdk-go-v2/service/databrew's
// awsRestjson1_deserializeOpErrorDeleteProject).
func TestDeleteProject_UsedByJob(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	ctx := context.Background()
	_, err := b.CreateProject(ctx, "p-used-job", "", "", "", databrew.Sample{}, nil)
	require.NoError(t, err)
	_, err = b.CreateJob(
		ctx, "j-used-proj", "RECIPE", "", "p-used-job", "", "",
		nil, nil, databrew.JobExtras{},
	)
	require.NoError(t, err)

	err = b.DeleteProject(ctx, "p-used-job")
	require.Error(t, err)
	require.ErrorIs(t, err, databrew.ErrConflict)

	_, err = b.DescribeProject(ctx, "p-used-job")
	require.NoError(t, err, "project must still exist")
}

// ---- Project handler ----

func TestHandlerCreateProject(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(t, h, http.MethodPost, "/databrew/v1/projects", map[string]any{
		"Name": "my-proj", "RecipeName": "r1", "DatasetName": "ds1",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerDescribeProject(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/projects", map[string]any{
		"Name": "proj1", "RecipeName": "r1",
	})
	rec := databrewReq(t, h, http.MethodGet, "/databrew/v1/projects/proj1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerListProjects(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(t, h, http.MethodGet, "/databrew/v1/projects", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerUpdateProject(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/projects", map[string]any{
		"Name": "upd-proj", "RecipeName": "r1", "DatasetName": "orig-ds",
	})
	// UpdateProjectInput has no DatasetName member (real SDK shape); only
	// RoleArn/Sample are updatable.
	rec := databrewReq(t, h, http.MethodPut, "/databrew/v1/projects/upd-proj", map[string]any{
		"RoleArn": "arn:aws:iam::123456789012:role/new",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	desc := databrewReq(t, h, http.MethodGet, "/databrew/v1/projects/upd-proj", nil)
	require.Equal(t, http.StatusOK, desc.Code)
	var proj map[string]any
	require.NoError(t, json.Unmarshal(desc.Body.Bytes(), &proj))
	assert.Equal(t, "orig-ds", proj["DatasetName"], "DatasetName must be immutable via UpdateProject")
	assert.Equal(t, "arn:aws:iam::123456789012:role/new", proj["RoleArn"])
}

func TestHandlerDeleteProject(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/projects", map[string]any{
		"Name": "del-proj", "RecipeName": "r1",
	})
	rec := databrewReq(t, h, http.MethodDelete, "/databrew/v1/projects/del-proj", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---- Project session handler ----

func TestHandlerStartProjectSession(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/projects", map[string]any{
		"Name": "sess-proj", "RecipeName": "r1",
	})
	rec := databrewReq(
		t,
		h,
		http.MethodPut,
		"/databrew/v1/projects/sess-proj/startProjectSession",
		map[string]any{
			"AssumeControl": true,
		},
	)
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "sess-proj", resp["Name"])
}

func TestHandlerSendProjectSessionAction(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/projects", map[string]any{
		"Name": "action-proj", "RecipeName": "r1",
	})
	rec := databrewReq(
		t,
		h,
		http.MethodPut,
		"/databrew/v1/projects/action-proj/sendProjectSessionAction",
		map[string]any{
			"Action": map[string]any{"Operation": "TRIM"},
		},
	)
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "action-proj", resp["Name"])
}

// TestHandlerStartProjectSession_ClientSessionID proves StartProjectSession
// returns a non-empty ClientSessionId, a real response field
// (StartProjectSessionOutput.ClientSessionId) that was previously always
// dropped.
func TestHandlerStartProjectSession_ClientSessionID(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/projects", map[string]any{
		"Name": "sess-id-proj", "RecipeName": "r1",
	})
	rec := databrewReq(t, h, http.MethodPut, "/databrew/v1/projects/sess-id-proj/startProjectSession", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["ClientSessionId"])
}

// TestHandlerStartProjectSession_NoSuchProject and
// TestHandlerSendProjectSessionAction_NoSuchProject prove both ops reject a
// project name that doesn't exist instead of reporting success -- previously
// neither handler consulted the backend at all, so any name (typo or not)
// echoed back a 200. ResourceNotFoundException is a documented error for
// both StartProjectSession and SendProjectSessionAction (botocore
// databrew/2017-07-25 service-2.json operations[*].errors).
func TestHandlerStartProjectSession_NoSuchProject(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(t, h, http.MethodPut, "/databrew/v1/projects/no-such-proj/startProjectSession", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlerSendProjectSessionAction_NoSuchProject(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(
		t, h, http.MethodPut, "/databrew/v1/projects/no-such-proj/sendProjectSessionAction",
		map[string]any{"Action": map[string]any{"Operation": "TRIM"}},
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
