package scheduler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/scheduler"
)

func TestSchedulerHandler_TagResource(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	// Create a schedule and get its ARN
	createRec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(5 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:a",
			"RoleArn": "arn:r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})
	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	arn := createResp["ScheduleArn"]

	rec := doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": arn,
		"Tags":        wireTagsBody(map[string]string{"env": "test", "team": "platform"}),
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSchedulerHandler_ListTagsForResource(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	// Create schedule and tag it
	createRec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(5 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:a",
			"RoleArn": "arn:r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})
	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	arn := createResp["ScheduleArn"]

	doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": arn,
		"Tags":        wireTagsBody(map[string]string{"env": "prod"}),
	})

	rec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": arn})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp, "Tags")
	tags := wireTagsToMap(t, resp["Tags"])
	assert.Equal(t, "prod", tags["env"])
}

func TestTagResource_AndListTags(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	createRec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "tagged-sched",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	schedARN := createResp["ScheduleArn"]

	tagRec := doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": schedARN,
		"Tags":        wireTagsBody(map[string]string{"env": "prod", "team": "platform"}),
	})
	require.Equal(t, http.StatusOK, tagRec.Code)

	listRec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": schedARN})
	require.Equal(t, http.StatusOK, listRec.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &tagsResp))
	tags := wireTagsToMap(t, tagsResp["Tags"])
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "platform", tags["team"])
}

func TestTagResource_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": "arn:aws:scheduler:us-east-1:000000000000:schedule/default/nonexistent",
		"Tags":        wireTagsBody(map[string]string{"k": "v"}),
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListTagsForResource_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceArn": "arn:aws:scheduler:us-east-1:000000000000:schedule/default/nonexistent",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSchedulerHandler_UntagResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupTags   map[string]string
		name        string
		removeKeys  []string
		wantRemoved []string
		wantKept    []string
		wantCode    int
		notFound    bool
	}{
		{
			name:        "remove_one_tag_from_schedule",
			setupTags:   map[string]string{"env": "test", "team": "platform"},
			removeKeys:  []string{"env"},
			wantCode:    http.StatusOK,
			wantRemoved: []string{"env"},
			wantKept:    []string{"team"},
		},
		{
			name:       "remove_all_tags",
			setupTags:  map[string]string{"env": "test"},
			removeKeys: []string{"env"},
			wantCode:   http.StatusOK,
		},
		{
			name:       "not_found",
			wantCode:   http.StatusNotFound,
			removeKeys: []string{"env"},
			notFound:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)

			var resourceARN string
			if !tt.notFound {
				createRec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
					"Name":               "tagged-schedule",
					"ScheduleExpression": "rate(1 minute)",
					"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
					"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
				})
				require.Equal(t, http.StatusOK, createRec.Code)
				var createResp map[string]string
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
				resourceARN = createResp["ScheduleArn"]

				if len(tt.setupTags) > 0 {
					doSchedulerRequest(t, h, "TagResource", map[string]any{
						"ResourceArn": resourceARN,
						"Tags":        wireTagsBody(tt.setupTags),
					})
				}
			} else {
				resourceARN = "arn:aws:scheduler:us-east-1:000000000000:schedule/default/nonexistent"
			}

			rec := doSchedulerRequest(t, h, "UntagResource", map[string]any{
				"ResourceArn": resourceARN,
				"TagKeys":     tt.removeKeys,
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				listRec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": resourceARN})
				require.Equal(t, http.StatusOK, listRec.Code)
				var listResp map[string]any
				require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
				remaining := wireTagsToMap(t, listResp["Tags"])
				for _, key := range tt.wantRemoved {
					assert.NotContains(t, remaining, key)
				}
				for _, key := range tt.wantKept {
					assert.Contains(t, remaining, key)
				}
			}
		})
	}
}

func TestUntagResource(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	b := h.Backend.(*scheduler.InMemoryBackend)

	grp, err := b.CreateScheduleGroup(context.Background(), "tag-grp", map[string]string{"k1": "v1", "k2": "v2"})
	require.NoError(t, err)

	untagRec := doSchedulerRequest(t, h, "UntagResource", map[string]any{
		"ResourceArn": grp.ARN,
		"TagKeys":     []string{"k1"},
	})
	require.Equal(t, http.StatusOK, untagRec.Code)

	listRec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": grp.ARN})
	require.Equal(t, http.StatusOK, listRec.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &tagsResp))
	tags := wireTagsToMap(t, tagsResp["Tags"])
	assert.NotContains(t, tags, "k1")
	assert.Contains(t, tags, "k2")
}

func TestSchedulerHandler_UntagResource_ScheduleGroup(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{
		"Name": "tagged-group",
		"Tags": wireTagsBody(map[string]string{"env": "test", "team": "platform"}),
	})

	getRec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": "tagged-group"})
	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	groupARN, _ := getResp["Arn"].(string)
	require.NotEmpty(t, groupARN)

	rec := doSchedulerRequest(t, h, "UntagResource", map[string]any{
		"ResourceArn": groupARN,
		"TagKeys":     []string{"env"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	listRec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": groupARN})
	require.Equal(t, http.StatusOK, listRec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	remaining := wireTagsToMap(t, listResp["Tags"])
	assert.NotContains(t, remaining, "env")
	assert.Contains(t, remaining, "team")
}

func TestSchedulerHandler_TagResource_ScheduleGroup(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": "tag-group"})
	getRec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": "tag-group"})
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	groupARN, _ := getResp["Arn"].(string)

	rec := doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": groupARN,
		"Tags":        wireTagsBody(map[string]string{"owner": "team-a"}),
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	listRec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": groupARN})
	require.Equal(t, http.StatusOK, listRec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	tagsMap := wireTagsToMap(t, listResp["Tags"])
	assert.Equal(t, "team-a", tagsMap["owner"])
}

// TestRESTTagResource verifies POST /tags/{resourceArn} tags a resource.
func TestRESTTagResource(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	// Create a schedule and grab its ARN.
	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "tag-via-rest",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	schedARN := createOut["ScheduleArn"]
	require.NotEmpty(t, schedARN)

	// Tag via REST path.
	rec2 := doRESTRequest(t, h, http.MethodPost, "/tags/"+schedARN, map[string]any{
		"Tags": wireTagsBody(map[string]string{"env": "prod", "team": "platform"}),
	}, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	// Verify tags via ListTagsForResource.
	rec3 := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": schedARN})
	require.Equal(t, http.StatusOK, rec3.Code)

	var tagsOut map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &tagsOut))
	tags := wireTagsToMap(t, tagsOut["Tags"])
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "platform", tags["team"])
}

// TestRESTListTagsForResource verifies GET /tags/{resourceArn}.
func TestRESTListTagsForResource(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "list-tags-rest",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"Tags":               map[string]string{"key1": "val1"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	schedARN := createOut["ScheduleArn"]

	// Tag the resource first via the standard operation.
	_ = doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": schedARN,
		"Tags":        wireTagsBody(map[string]string{"key1": "val1"}),
	})

	rec2 := doRESTRequest(t, h, http.MethodGet, "/tags/"+schedARN, nil, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var tagsOut map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &tagsOut))
	tags := wireTagsToMap(t, tagsOut["Tags"])
	assert.Equal(t, "val1", tags["key1"])
}

// TestRESTUntagResource verifies DELETE /tags/{resourceArn}?tagKeys=...
func TestRESTUntagResource(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "untag-rest",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	schedARN := createOut["ScheduleArn"]

	_ = doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": schedARN,
		"Tags":        wireTagsBody(map[string]string{"key1": "val1", "key2": "val2"}),
	})

	rec2 := doRESTRequest(t, h, http.MethodDelete, "/tags/"+schedARN, nil, map[string]string{
		"TagKeys": "key1",
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	rec3 := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": schedARN})
	var tagsOut map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &tagsOut))
	tags := wireTagsToMap(t, tagsOut["Tags"])
	assert.NotContains(t, tags, "key1")
	assert.Equal(t, "val2", tags["key2"])
}

// TestRESTUntagResourceMultipleTagKeys verifies DELETE
// /tags/{resourceArn}?TagKeys=a&TagKeys=b removes all listed keys in one call.
// aws-sdk-go-v2's awsRestjson1_serializeOpHttpBindingsUntagResourceInput sends
// TagKeys as a repeated query parameter (encoder.AddQuery("TagKeys", ...) per
// key), not a single comma-separated value under a lowercase "tagKeys" name.
func TestRESTUntagResourceMultipleTagKeys(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "untag-rest-multi",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	schedARN := createOut["ScheduleArn"]

	_ = doSchedulerRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": schedARN,
		"Tags":        wireTagsBody(map[string]string{"key1": "val1", "key2": "val2", "key3": "val3"}),
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/tags/"+schedARN, nil)
	req.URL.RawQuery = "TagKeys=key1&TagKeys=key2"
	rec2 := httptest.NewRecorder()
	c := e.NewContext(req, rec2)
	require.NoError(t, h.Handler()(c))
	require.Equal(t, http.StatusOK, rec2.Code)

	rec3 := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": schedARN})
	var tagsOut map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &tagsOut))
	tags := wireTagsToMap(t, tagsOut["Tags"])
	assert.NotContains(t, tags, "key1")
	assert.NotContains(t, tags, "key2")
	assert.Equal(t, "val3", tags["key3"])
}

// TestRouteMatcher_TagsPath verifies /tags/{arn} is routed to scheduler only when
// the ARN belongs to the Scheduler service.
func TestRouteMatcher_TagsPath(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	m := h.RouteMatcher()

	e := echo.New()

	makeCtx := func(path string) *echo.Context {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		return c
	}

	assert.True(t, m(makeCtx("/schedules")), "/schedules should match")
	assert.True(t, m(makeCtx("/schedule-groups/foo")), "/schedule-groups/foo should match")
	assert.True(
		t,
		m(makeCtx("/tags/arn:aws:scheduler:us-east-1:123:schedule/default/my-sched")),
		"/tags/arn:aws:scheduler:... should match",
	)
	assert.True(
		t,
		m(makeCtx("/tags/arn:aws:scheduler:us-east-1:000000000000:schedule-group/default")),
		"/tags/arn:aws:scheduler:...:schedule-group/... should match",
	)
	// Non-scheduler ARNs must NOT be claimed by the Scheduler route matcher to
	// avoid intercepting tag requests destined for other services (QLDB, Pipes, FIS, etc.).
	assert.False(
		t,
		m(makeCtx("/tags/arn:aws:qldb:us-east-1:000000000000:ledger/my-ledger")),
		"/tags/arn:aws:qldb:... should NOT match",
	)
	assert.False(
		t,
		m(makeCtx("/tags/arn:aws:pipes:us-east-1:000000000000:pipe/my-pipe")),
		"/tags/arn:aws:pipes:... should NOT match",
	)
	assert.False(
		t,
		m(makeCtx("/tags/arn:aws:fis:us-east-1:000000000000:experiment-template/abc")),
		"/tags/arn:aws:fis:... should NOT match",
	)
	assert.False(t, m(makeCtx("/tags")), "bare /tags should NOT match")
	assert.False(t, m(makeCtx("/other/path")), "/other/path should not match")
}

// TestExtractResource_TagsPath verifies ExtractResource returns ARN for /tags/{arn}.
func TestExtractResource_TagsPath(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	e := echo.New()
	arn := "arn:aws:scheduler:us-east-1:000000000000:schedule/default/my-sched"

	req := httptest.NewRequest(http.MethodGet, "/tags/"+arn, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	resource := h.ExtractResource(c)
	assert.Equal(t, arn, resource)
}
