package docdb_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/docdb"
)

func TestHandler_EventSubscriptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_event_subscription",
			vals: url.Values{
				"Action":           {"CreateEventSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"my-sub"},
				"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:my-topic"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-sub",
		},
		{
			name: "add_source_identifier",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"my-sub"},
					"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:my-topic"},
				})
			},
			vals: url.Values{
				"Action":           {"AddSourceIdentifierToSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"my-sub"},
				"SourceIdentifier": {"my-cluster"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-cluster",
		},
		{
			name: "delete_event_subscription",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"my-sub"},
					"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:my-topic"},
				})
			},
			vals: url.Values{
				"Action":           {"DeleteEventSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"my-sub"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DeleteEventSubscriptionResponse",
		},
		{
			name: "create_duplicate_subscription",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"dup-sub"},
				})
			},
			vals: url.Values{
				"Action":           {"CreateEventSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"dup-sub"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "SubscriptionAlreadyExist",
		},
		{
			name: "delete_nonexistent_subscription",
			vals: url.Values{
				"Action":           {"DeleteEventSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"nonexistent"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "SubscriptionNotFound",
		},
		{
			name: "add_source_id_nonexistent_subscription",
			vals: url.Values{
				"Action":           {"AddSourceIdentifierToSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"nonexistent"},
				"SourceIdentifier": {"some-cluster"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "SubscriptionNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_DescribeEventSubscriptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "describe_all_subscriptions",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"my-sub"},
				})
			},
			vals: url.Values{
				"Action":  {"DescribeEventSubscriptions"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-sub",
		},
		{
			name: "describe_subscription_by_name",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"my-sub"},
				})
			},
			vals: url.Values{
				"Action":           {"DescribeEventSubscriptions"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"my-sub"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-sub",
		},
		{
			name: "describe_subscriptions_empty",
			vals: url.Values{
				"Action":  {"DescribeEventSubscriptions"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeEventSubscriptionsResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_ModifyEventSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "modify_subscription",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"my-sub"},
					"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:old-topic"},
				})
			},
			vals: url.Values{
				"Action":           {"ModifyEventSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"my-sub"},
				"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:new-topic"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "new-topic",
		},
		{
			name: "modify_subscription_not_found",
			vals: url.Values{
				"Action":           {"ModifyEventSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"nonexistent"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "SubscriptionNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_RemoveSourceIdentifierFromSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "remove_source_identifier",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":               {"CreateEventSubscription"},
					"Version":              {"2014-10-31"},
					"SubscriptionName":     {"my-sub"},
					"SourceIds.SourceId.1": {"my-cluster"},
				})
			},
			vals: url.Values{
				"Action":           {"RemoveSourceIdentifierFromSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"my-sub"},
				"SourceIdentifier": {"my-cluster"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "RemoveSourceIdentifierFromSubscriptionResponse",
		},
		{
			name: "remove_source_identifier_not_found",
			vals: url.Values{
				"Action":           {"RemoveSourceIdentifierFromSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"nonexistent"},
				"SourceIdentifier": {"my-cluster"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "SubscriptionNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_DescribeEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "describe_events",
			vals: url.Values{
				"Action":  {"DescribeEvents"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeEventsResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_DescribeEventCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "describe_event_categories",
			vals: url.Values{
				"Action":  {"DescribeEventCategories"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeEventCategoriesResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestDescribeEventCategoriesFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals            url.Values
		name            string
		wantContains    string
		wantNotContains string
		wantStatus      int
	}{
		{
			name: "no_source_type_filter",
			vals: url.Values{
				"Action":  {"DescribeEventCategories"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "db-cluster",
		},
		{
			name: "filter_by_db_instance",
			vals: url.Values{
				"Action":     {"DescribeEventCategories"},
				"Version":    {"2014-10-31"},
				"SourceType": {"db-instance"},
			},
			wantStatus:      http.StatusOK,
			wantContains:    "db-instance",
			wantNotContains: "db-cluster-snapshot",
		},
		{
			name: "filter_by_snapshot",
			vals: url.Values{
				"Action":     {"DescribeEventCategories"},
				"Version":    {"2014-10-31"},
				"SourceType": {"db-cluster-snapshot"},
			},
			wantStatus:      http.StatusOK,
			wantContains:    "db-cluster-snapshot",
			wantNotContains: "db-instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
			if tt.wantNotContains != "" {
				assert.NotContains(t, rr.Body.String(), tt.wantNotContains)
			}
		})
	}
}

func TestEventSubscriptionSourceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_subscription_with_source_type",
			vals: url.Values{
				"Action":                          {"CreateEventSubscription"},
				"Version":                         {"2014-10-31"},
				"SubscriptionName":                {"my-sub"},
				"SourceType":                      {"db-cluster"},
				"EventCategories.EventCategory.1": {"backup"},
				"EventCategories.EventCategory.2": {"failover"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "db-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestEventSubscription_FullLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "modify_subscription_topic",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"mod-sub"},
					"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:old-topic"},
				})
			},
			vals: url.Values{
				"Action":           {"ModifyEventSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"mod-sub"},
				"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:new-topic"},
			},
			wantStatus:   200,
			wantContains: "new-topic",
		},
		{
			name: "remove_source_identifier",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":               {"CreateEventSubscription"},
					"Version":              {"2014-10-31"},
					"SubscriptionName":     {"src-id-sub"},
					"SourceIds.SourceId.1": {"my-cluster"},
				})
			},
			vals: url.Values{
				"Action":           {"RemoveSourceIdentifierFromSubscription"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"src-id-sub"},
				"SourceIdentifier": {"my-cluster"},
			},
			wantStatus:   200,
			wantContains: "RemoveSourceIdentifierFromSubscriptionResponse",
		},
		{
			name: "describe_event_subscriptions",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":           {"CreateEventSubscription"},
					"Version":          {"2014-10-31"},
					"SubscriptionName": {"desc-sub"},
				})
			},
			vals: url.Values{
				"Action":           {"DescribeEventSubscriptions"},
				"Version":          {"2014-10-31"},
				"SubscriptionName": {"desc-sub"},
			},
			wantStatus:   200,
			wantContains: "desc-sub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestDescribeEventCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sourceType   string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "all_categories",
			sourceType:   "",
			wantContains: "db-cluster",
			wantStatus:   200,
		},
		{
			name:         "db_cluster_categories",
			sourceType:   "db-cluster",
			wantContains: "failover",
			wantStatus:   200,
		},
		{
			name:         "db_instance_categories",
			sourceType:   "db-instance",
			wantContains: "recovery",
			wantStatus:   200,
		},
		{
			name:         "snapshot_categories",
			sourceType:   "db-cluster-snapshot",
			wantContains: "restoration",
			wantStatus:   200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			vals := url.Values{
				"Action":  {"DescribeEventCategories"},
				"Version": {"2014-10-31"},
			}
			if tt.sourceType != "" {
				vals.Set("SourceType", tt.sourceType)
			}
			rr := doRequest(t, h, vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestCreateEventSubscription_SourceIdsAndCategoriesNotSwapped locks in the
// fix for a real bug found this pass: the handler's call into
// Backend.CreateEventSubscription passed its sourceIDs/eventCategories
// arguments in the wrong positional order, so a real client's SourceIds came
// back (wrongly) as EventCategoriesList and its EventCategories came back
// (wrongly) as SourceIdsList -- invisible to any test that only checked one
// of the two lists at a time (as every pre-existing test in this file did).
func TestCreateEventSubscription_SourceIdsAndCategoriesNotSwapped(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                          {"CreateEventSubscription"},
		"Version":                         {"2014-10-31"},
		"SubscriptionName":                {"swap-check-sub"},
		"SourceIds.SourceId.1":            {"my-cluster-id"},
		"EventCategories.EventCategory.1": {"backup"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	// The source ID must land inside SourceIdsList, not EventCategoriesList.
	require.Contains(t, body, "<SourceIdsList><SourceId>my-cluster-id</SourceId></SourceIdsList>")
	// The event category must land inside EventCategoriesList, not SourceIdsList.
	require.Contains(t, body, "<EventCategoriesList><EventCategory>backup</EventCategory></EventCategoriesList>")
}

// TestEventSubscription_FullFieldRoundTrip locks in the fix for the wire
// gap found this pass: xmlEventSubscription previously omitted
// EventCategoriesList/EventSubscriptionArn/Enabled/CustomerAwsId/
// SubscriptionCreationTime entirely, so a real client reading back the event
// categories or ARN it just set on Create always saw them silently dropped
// even though the backend tracked EventCategories correctly internally.
func TestEventSubscription_FullFieldRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                          {"CreateEventSubscription"},
		"Version":                         {"2014-10-31"},
		"SubscriptionName":                {"full-field-sub"},
		"SnsTopicArn":                     {"arn:aws:sns:us-east-1:000000000000:topic"},
		"SourceType":                      {"db-cluster"},
		"EventCategories.EventCategory.1": {"failover"},
		"Enabled":                         {"false"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	assert.Contains(t, body, "<EventCategoriesList><EventCategory>failover</EventCategory></EventCategoriesList>")
	assert.Contains(t, body, "<EventSubscriptionArn>")
	assert.Contains(t, body, "<CustomerAwsId>000000000000</CustomerAwsId>")
	assert.Contains(t, body, "<SubscriptionCreationTime>")
	assert.Contains(t, body, "<Enabled>false</Enabled>")

	// Enabled must default to true when the caller doesn't specify it
	// (matching AWS's own default for a new subscription).
	defaultRR := doRequest(t, h, url.Values{
		"Action":           {"CreateEventSubscription"},
		"Version":          {"2014-10-31"},
		"SubscriptionName": {"default-enabled-sub"},
	})
	require.Equal(t, http.StatusOK, defaultRR.Code)
	assert.Contains(t, defaultRR.Body.String(), "<Enabled>true</Enabled>")

	// ModifyEventSubscription's Enabled must be a real, wire-visible mutation.
	modifyRR := doRequest(t, h, url.Values{
		"Action":           {"ModifyEventSubscription"},
		"Version":          {"2014-10-31"},
		"SubscriptionName": {"default-enabled-sub"},
		"Enabled":          {"false"},
	})
	require.Equal(t, http.StatusOK, modifyRR.Code)
	assert.Contains(t, modifyRR.Body.String(), "<Enabled>false</Enabled>")
}

// TestDescribeEvents_RealLog locks in the fix for the "DescribeEvents always
// returns an empty event list" gap identified in PARITY.md: there was no
// event log backing this backend at all, so DescribeEvents could never
// report anything regardless of what a caller had actually done.
func TestDescribeEvents_RealLog(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"events-cluster"},
	})
	doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"events-instance"},
		"DBClusterIdentifier":  {"events-cluster"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribeEvents"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "events-cluster")
	assert.Contains(t, body, "events-instance")
	assert.Contains(t, body, "<SourceType>db-cluster</SourceType>")
	assert.Contains(t, body, "<SourceType>db-instance</SourceType>")

	// SourceIdentifier filtering must narrow the result to just that resource.
	filteredRR := doRequest(t, h, url.Values{
		"Action":           {"DescribeEvents"},
		"Version":          {"2014-10-31"},
		"SourceIdentifier": {"events-cluster"},
	})
	require.Equal(t, http.StatusOK, filteredRR.Code)
	filteredBody := filteredRR.Body.String()
	assert.Contains(t, filteredBody, "events-cluster")
	assert.NotContains(t, filteredBody, "events-instance")

	// Deleting the cluster must record a second, distinct event for it
	// (the instance is deleted first: DeleteDBCluster refuses a cluster
	// that still has instances).
	doRequest(t, h, url.Values{
		"Action":               {"DeleteDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"events-instance"},
	})
	doRequest(t, h, url.Values{
		"Action":              {"DeleteDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"events-cluster"},
		"SkipFinalSnapshot":   {"true"},
	})
	afterDeleteRR := doRequest(t, h, url.Values{
		"Action":           {"DescribeEvents"},
		"Version":          {"2014-10-31"},
		"SourceIdentifier": {"events-cluster"},
	})
	require.Equal(t, http.StatusOK, afterDeleteRR.Code)
	assert.Equal(t, 2, strings.Count(afterDeleteRR.Body.String(), "<Event>"),
		"create and delete must each record a distinct event")
}

// TestDeleteEventSubscription_ClearsTags verifies that DeleteEventSubscription
// clears tags for the deleted subscription's ARN. Otherwise a new subscription
// created with the same (user-chosen, reusable) SubscriptionName -- which
// builds the same deterministic ARN -- inherits the deleted subscription's tags.
func TestDeleteEventSubscription_ClearsTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const subARN = "arn:aws:rds:us-east-1:000000000000:es:reused-sub"

	doRequest(t, h, url.Values{
		"Action":           {"CreateEventSubscription"},
		"Version":          {"2014-10-31"},
		"SubscriptionName": {"reused-sub"},
		"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:my-topic"},
	})

	doRequest(t, h, url.Values{
		"Action":           {"AddTagsToResource"},
		"Version":          {"2014-10-31"},
		"ResourceName":     {subARN},
		"Tags.Tag.1.Key":   {"env"},
		"Tags.Tag.1.Value": {"prod"},
	})

	listRR := doRequest(t, h, url.Values{
		"Action":       {"ListTagsForResource"},
		"Version":      {"2014-10-31"},
		"ResourceName": {subARN},
	})
	require.Contains(t, listRR.Body.String(), "prod")

	doRequest(t, h, url.Values{
		"Action":           {"DeleteEventSubscription"},
		"Version":          {"2014-10-31"},
		"SubscriptionName": {"reused-sub"},
	})

	doRequest(t, h, url.Values{
		"Action":           {"CreateEventSubscription"},
		"Version":          {"2014-10-31"},
		"SubscriptionName": {"reused-sub"},
		"SnsTopicArn":      {"arn:aws:sns:us-east-1:000000000000:my-topic"},
	})

	afterRR := doRequest(t, h, url.Values{
		"Action":       {"ListTagsForResource"},
		"Version":      {"2014-10-31"},
		"ResourceName": {subARN},
	})
	require.Equal(t, http.StatusOK, afterRR.Code)
	assert.NotContains(t, afterRR.Body.String(), "prod",
		"recreated event subscription must not inherit the deleted subscription's tags")
}
