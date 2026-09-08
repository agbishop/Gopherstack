package redshift_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- EnableLogging ----

func TestRedshiftHandler_EnableLogging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=log-cluster")
			},
			body: "Action=EnableLogging&Version=2012-12-01" +
				"&ClusterIdentifier=log-cluster&BucketName=my-bucket&S3KeyPrefix=logs/",
			wantCode:     http.StatusOK,
			wantContains: []string{"EnableLoggingResponse", "true", "my-bucket"},
		},
		{
			name:         "cluster_not_found",
			body:         "Action=EnableLogging&Version=2012-12-01&ClusterIdentifier=nonexistent&BucketName=bucket",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
		{
			name:         "missing_cluster_id",
			body:         "Action=EnableLogging&Version=2012-12-01&BucketName=bucket",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "missing_bucket",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=log-cluster2")
			},
			body:         "Action=EnableLogging&Version=2012-12-01&ClusterIdentifier=log-cluster2",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "s3_key_prefix_invalid_character_rejected",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=log-cluster3")
			},
			body: "Action=EnableLogging&Version=2012-12-01" +
				"&ClusterIdentifier=log-cluster3&BucketName=my-bucket&S3KeyPrefix=" + url.QueryEscape("bad#prefix"),
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidS3KeyPrefixFault"},
		},
		{
			name: "s3_key_prefix_all_documented_characters_accepted",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=log-cluster4")
			},
			body: "Action=EnableLogging&Version=2012-12-01" +
				"&ClusterIdentifier=log-cluster4&BucketName=my-bucket&S3KeyPrefix=" +
				url.QueryEscape(`ab12 _.:/=+\-@`),
			wantCode:     http.StatusOK,
			wantContains: []string{"EnableLoggingResponse"},
		},
		{
			name: "s3_key_prefix_empty_is_legal",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=log-cluster5")
			},
			body:         "Action=EnableLogging&Version=2012-12-01&ClusterIdentifier=log-cluster5&BucketName=my-bucket",
			wantCode:     http.StatusOK,
			wantContains: []string{"EnableLoggingResponse"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DisableLogging ----

func TestRedshiftHandler_DisableLogging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=dis-log-cluster")
			},
			body:         "Action=DisableLogging&Version=2012-12-01&ClusterIdentifier=dis-log-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"DisableLoggingResponse", "false"},
		},
		{
			name:         "cluster_not_found",
			body:         "Action=DisableLogging&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DescribeEvents ----

func TestRedshiftHandler_DescribeEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "list_empty",
			body:         "Action=DescribeEvents&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeEventsResponse"},
		},
		{
			name:         "with_source_filter",
			body:         "Action=DescribeEvents&Version=2012-12-01&SourceIdentifier=my-cluster&SourceType=cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeEventsResponse"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DescribeEventCategories ----

func TestRedshiftHandler_DescribeEventCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "success",
			body:         "Action=DescribeEventCategories&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeEventCategoriesResponse", "cluster"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- CreateEventSubscription ----

func TestRedshiftHandler_CreateEventSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			body: "Action=CreateEventSubscription&Version=2012-12-01" +
				"&SubscriptionName=my-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:mytopic" +
				"&SourceType=cluster&Enabled=true",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateEventSubscriptionResponse", "my-sub"},
		},
		{
			name:         "missing_subscription_name",
			body:         "Action=CreateEventSubscription&Version=2012-12-01&SnsTopicArn=arn:aws:sns:us-east-1:123:topic",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_sns_topic",
			body:         "Action=CreateEventSubscription&Version=2012-12-01&SubscriptionName=my-sub",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "duplicate_subscription",
			body: "Action=CreateEventSubscription&Version=2012-12-01" +
				"&SubscriptionName=dup-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:topic",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"SubscriptionAlreadyExist"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.name == "duplicate_subscription" {
				postRedshiftForm(t, h, "Action=CreateEventSubscription&Version=2012-12-01"+
					"&SubscriptionName=dup-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:topic")
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DeleteEventSubscription ----

func TestRedshiftHandler_DeleteEventSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateEventSubscription&Version=2012-12-01"+
					"&SubscriptionName=del-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:topic")
			},
			body:         "Action=DeleteEventSubscription&Version=2012-12-01&SubscriptionName=del-sub",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteEventSubscriptionResponse"},
		},
		{
			name:         "not_found",
			body:         "Action=DeleteEventSubscription&Version=2012-12-01&SubscriptionName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"SubscriptionNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DescribeEventSubscriptions ----

func TestRedshiftHandler_DescribeEventSubscriptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "list_empty",
			body:         "Action=DescribeEventSubscriptions&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeEventSubscriptionsResponse"},
		},
		{
			name: "list_with_subscription",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateEventSubscription&Version=2012-12-01"+
					"&SubscriptionName=list-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:topic")
			},
			body:         "Action=DescribeEventSubscriptions&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeEventSubscriptionsResponse", "list-sub"},
		},
		{
			name: "describe_specific",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateEventSubscription&Version=2012-12-01"+
					"&SubscriptionName=specific-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:topic")
			},
			body:         "Action=DescribeEventSubscriptions&Version=2012-12-01&SubscriptionName=specific-sub",
			wantCode:     http.StatusOK,
			wantContains: []string{"specific-sub"},
		},
		{
			name:         "not_found",
			body:         "Action=DescribeEventSubscriptions&Version=2012-12-01&SubscriptionName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"SubscriptionNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- ModifyEventSubscription ----

func TestRedshiftHandler_ModifyEventSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateEventSubscription&Version=2012-12-01"+
					"&SubscriptionName=mod-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:old-topic")
			},
			body: "Action=ModifyEventSubscription&Version=2012-12-01" +
				"&SubscriptionName=mod-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:new-topic",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyEventSubscriptionResponse", "mod-sub", "new-topic"},
		},
		{
			name:         "not_found",
			body:         "Action=ModifyEventSubscription&Version=2012-12-01&SubscriptionName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"SubscriptionNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- Backend: EventSubscriptionCount ----

func TestRedshiftBackend_EventSubscriptionCount(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	require.Equal(t, 0, redshift.EventSubscriptionCount(b))

	h := redshift.NewHandler(b)
	postRedshiftForm(t, h, "Action=CreateEventSubscription&Version=2012-12-01"+
		"&SubscriptionName=count-sub&SnsTopicArn=arn:aws:sns:us-east-1:123:topic")
	require.Equal(t, 1, redshift.EventSubscriptionCount(b))

	postRedshiftForm(t, h, "Action=DeleteEventSubscription&Version=2012-12-01&SubscriptionName=count-sub")
	require.Equal(t, 0, redshift.EventSubscriptionCount(b))
}
