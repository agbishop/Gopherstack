package cloudwatchlogs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
)

func TestCloudWatchLogsBackend_DescribeDeliveries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(b *cloudwatchlogs.InMemoryBackend)
		name    string
		wantLen int
	}{
		{
			name: "returns_all",
			setup: func(b *cloudwatchlogs.InMemoryBackend) {
				cloudwatchlogs.AddDeliveryInternal(
					b,
					cloudwatchlogs.Delivery{ID: "d1", CreationTime: 1},
				)
				cloudwatchlogs.AddDeliveryInternal(
					b,
					cloudwatchlogs.Delivery{ID: "d2", CreationTime: 2},
				)
			},
			wantLen: 2,
		},
		{
			name:    "empty_returns_empty",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(b)
			}

			deliveries, _, err := b.DescribeDeliveries(50, "")
			require.NoError(t, err)
			assert.Len(t, deliveries, tt.wantLen)
		})
	}
}

func TestCloudWatchLogsBackend_GetAndDeleteDelivery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(b *cloudwatchlogs.InMemoryBackend)
		name    string
		id      string
	}{
		{
			name: "get_existing",
			setup: func(b *cloudwatchlogs.InMemoryBackend) {
				cloudwatchlogs.AddDeliveryInternal(b, cloudwatchlogs.Delivery{ID: "d1"})
			},
			id: "d1",
		},
		{
			name:    "get_missing",
			id:      "nonexistent",
			wantErr: cloudwatchlogs.ErrDeliveryNotFound,
		},
		{
			name:    "get_empty_id",
			id:      "",
			wantErr: cloudwatchlogs.ErrValidationException,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(b)
			}

			d, err := b.GetDelivery(tt.id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.id, d.ID)
		})
	}

	t.Run("delete_existing", func(t *testing.T) {
		t.Parallel()

		b := cloudwatchlogs.NewInMemoryBackend()
		cloudwatchlogs.AddDeliveryInternal(b, cloudwatchlogs.Delivery{ID: "d1"})

		err := b.DeleteDelivery("d1")
		require.NoError(t, err)

		_, err = b.GetDelivery("d1")
		require.ErrorIs(t, err, cloudwatchlogs.ErrDeliveryNotFound)
	})

	t.Run("delete_missing", func(t *testing.T) {
		t.Parallel()

		b := cloudwatchlogs.NewInMemoryBackend()
		err := b.DeleteDelivery("nonexistent")
		require.ErrorIs(t, err, cloudwatchlogs.ErrDeliveryNotFound)
	})
}

// TestCreateDelivery_DeliveryDestinationType is a regression guard: real
// types.Delivery.DeliveryDestinationType ("Displays whether the delivery
// destination associated with this delivery is CloudWatch Logs, Amazon S3,
// Firehose, or X-Ray", cloudwatchlogs@v1.81.1 types/types.go:539-540) is
// server-derived from the paired DeliveryDestination at CreateDelivery time,
// not client-supplied.
func TestCreateDelivery_DeliveryDestinationType(t *testing.T) {
	t.Parallel()

	t.Run("derives_type_from_paired_destination", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)
		dest, err := b.PutDeliveryDestination("dst", "arn:aws:s3:::bucket", "JSON", "S3", nil)
		require.NoError(t, err)

		delivery, err := b.CreateDelivery("src", dest.Arn, "", nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, "S3", delivery.DeliveryDestinationType)

		got, err := b.GetDelivery(delivery.ID)
		require.NoError(t, err)
		assert.Equal(t, "S3", got.DeliveryDestinationType)
	})

	t.Run("unknown_destination_arn_leaves_type_empty", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend(t)

		delivery, err := b.CreateDelivery("src", "arn:aws:logs:::delivery-destination:ghost", "", nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, delivery.DeliveryDestinationType)
	})
}

func TestDeliveryDestination_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		verify func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		name   string
	}{
		{
			name: "put_get_describe_delete",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				dest, err := b.PutDeliveryDestination("my-dest", "arn:aws:s3:::my-bucket", "JSON", "S3", nil)
				require.NoError(t, err)
				assert.Equal(t, "my-dest", dest.Name)
				assert.NotEmpty(t, dest.Arn)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				got, err := b.GetDeliveryDestination("my-dest")
				require.NoError(t, err)
				assert.Equal(t, "arn:aws:s3:::my-bucket", got.TargetArn)

				dests, _ := b.DescribeDeliveryDestinations("", 0)
				require.Len(t, dests, 1)

				err = b.DeleteDeliveryDestination("my-dest")
				require.NoError(t, err)

				_, err = b.GetDeliveryDestination("my-dest")
				require.Error(t, err)
			},
		},
		{
			// Real AWS: "You can't delete a delivery destination if any
			// current deliveries are associated with it."
			name: "delete_rejected_while_delivery_associated",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				dest, err := b.PutDeliveryDestination("assoc-dest", "arn:aws:s3:::assoc-bucket", "JSON", "S3", nil)
				require.NoError(t, err)

				delivery, err := b.CreateDelivery("assoc-src", dest.Arn, "", nil, nil, nil)
				require.NoError(t, err)

				err = b.DeleteDeliveryDestination("assoc-dest")
				require.ErrorIs(t, err, cloudwatchlogs.ErrDeliveryDestinationInUse)

				require.NoError(t, b.DeleteDelivery(delivery.ID))

				require.NoError(t, b.DeleteDeliveryDestination("assoc-dest"))
			},
		},
		{
			name: "put_updates_existing",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliveryDestination("dest1", "arn:aws:s3:::old", "JSON", "S3", nil)
				require.NoError(t, err)
				_, err = b.PutDeliveryDestination("dest1", "arn:aws:s3:::new", "TEXT", "S3", nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				got, err := b.GetDeliveryDestination("dest1")
				require.NoError(t, err)
				assert.Equal(t, "arn:aws:s3:::new", got.TargetArn)
				assert.Equal(t, "TEXT", got.OutputFormat)
			},
		},
		{
			name: "get_not_found_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.GetDeliveryDestination("ghost")
				require.ErrorIs(t, err, cloudwatchlogs.ErrDeliveryDestinationNotFound)
			},
		},
		{
			name: "delete_not_found_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				err := b.DeleteDeliveryDestination("ghost")
				require.ErrorIs(t, err, cloudwatchlogs.ErrDeliveryDestinationNotFound)
			},
		},
		{
			name: "policy_put_get_delete",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliveryDestination("policy-dest", "arn:aws:s3:::bucket", "JSON", "S3", nil)
				require.NoError(t, err)
				err = b.PutDeliveryDestinationPolicy("policy-dest", `{"Statement":[]}`)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				policy, err := b.GetDeliveryDestinationPolicy("policy-dest")
				require.NoError(t, err)
				assert.Contains(t, policy, "Statement")

				err = b.DeleteDeliveryDestinationPolicy("policy-dest")
				require.NoError(t, err)

				policy, err = b.GetDeliveryDestinationPolicy("policy-dest")
				require.NoError(t, err)
				assert.Empty(t, policy)
			},
		},
		{
			name: "policy_on_nonexistent_dest_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				err := b.PutDeliveryDestinationPolicy("ghost", `{}`)
				require.ErrorIs(t, err, cloudwatchlogs.ErrDeliveryDestinationNotFound)
			},
		},
		{
			name: "put_empty_name_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliveryDestination("", "arn:aws:s3:::b", "JSON", "S3", nil)
				require.ErrorIs(t, err, cloudwatchlogs.ErrValidationException)
			},
		},
		{
			name: "describe_sorted_by_name",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliveryDestination("z-dest", "arn:aws:s3:::z", "JSON", "S3", nil)
				require.NoError(t, err)
				_, err = b.PutDeliveryDestination("a-dest", "arn:aws:s3:::a", "JSON", "S3", nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				dests, _ := b.DescribeDeliveryDestinations("", 0)
				require.Len(t, dests, 2)
				assert.Equal(t, "a-dest", dests[0].Name)
				assert.Equal(t, "z-dest", dests[1].Name)
			},
		},
		{
			// aws-sdk-go-v2 types.DeliveryDestinationType's only members are
			// S3/CWL/FH/XRAY.
			name: "invalid_destination_type_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliveryDestination("bad-type-dest", "arn:aws:s3:::b", "JSON", "GCS", nil)
				require.ErrorIs(t, err, cloudwatchlogs.ErrValidationException)
			},
		},
		{
			name: "destination_type_persisted_and_updatable",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				dest, err := b.PutDeliveryDestination("typed-dest", "arn:aws:firehose:::stream/s", "JSON", "FH", nil)
				require.NoError(t, err)
				assert.Equal(t, "FH", dest.DeliveryDestinationType)

				// An empty destinationType on update leaves the stored value
				// unchanged rather than clearing it, matching how
				// PutDeliveryDestination treats other optional fields.
				dest, err = b.PutDeliveryDestination("typed-dest", "arn:aws:firehose:::stream/s2", "JSON", "", nil)
				require.NoError(t, err)
				assert.Equal(t, "FH", dest.DeliveryDestinationType)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				got, err := b.GetDeliveryDestination("typed-dest")
				require.NoError(t, err)
				assert.Equal(t, "FH", got.DeliveryDestinationType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			if tt.setup != nil {
				tt.setup(t, b)
			}
			if tt.verify != nil {
				tt.verify(t, b)
			}
		})
	}
}

func TestDeliverySource_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		verify func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		name   string
	}{
		{
			name: "put_get_describe_delete",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				src, err := b.PutDeliverySource(
					"my-src",
					"APPLICATION_LOGS",
					[]string{"arn:aws:ec2:::instance/i-123"},
					nil,
				)
				require.NoError(t, err)
				assert.Equal(t, "my-src", src.Name)
				assert.NotEmpty(t, src.Arn)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				got, err := b.GetDeliverySource("my-src")
				require.NoError(t, err)
				assert.Equal(t, "APPLICATION_LOGS", got.LogType)
				assert.Len(t, got.ResourceArns, 1)
				assert.Equal(t, "ec2", got.Service, "service must be derived from the resource ARN")

				srcs, _ := b.DescribeDeliverySources("", 0)
				require.Len(t, srcs, 1)

				err = b.DeleteDeliverySource("my-src")
				require.NoError(t, err)

				_, err = b.GetDeliverySource("my-src")
				require.Error(t, err)
			},
		},
		{
			// Real AWS: "You can't delete a delivery source if any current
			// deliveries are associated with it."
			name: "delete_rejected_while_delivery_associated",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliverySource("assoc-src", "APPLICATION_LOGS", nil, nil)
				require.NoError(t, err)

				delivery, err := b.CreateDelivery("assoc-src", "arn:aws:s3:::assoc-dest", "", nil, nil, nil)
				require.NoError(t, err)

				err = b.DeleteDeliverySource("assoc-src")
				require.ErrorIs(t, err, cloudwatchlogs.ErrDeliverySourceInUse)

				require.NoError(t, b.DeleteDelivery(delivery.ID))

				require.NoError(t, b.DeleteDeliverySource("assoc-src"))
			},
		},
		{
			name: "put_updates_existing",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliverySource("src1", "FLOW_LOGS", []string{"arn:old"}, nil)
				require.NoError(t, err)
				_, err = b.PutDeliverySource("src1", "VPC_FLOW_LOGS", []string{"arn:new"}, nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				got, err := b.GetDeliverySource("src1")
				require.NoError(t, err)
				assert.Equal(t, "VPC_FLOW_LOGS", got.LogType)
				require.Len(t, got.ResourceArns, 1)
				assert.Equal(t, "arn:new", got.ResourceArns[0])
			},
		},
		{
			name: "get_not_found_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.GetDeliverySource("ghost")
				require.ErrorIs(t, err, cloudwatchlogs.ErrDeliverySourceNotFound)
			},
		},
		{
			name: "delete_not_found_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				err := b.DeleteDeliverySource("ghost")
				require.ErrorIs(t, err, cloudwatchlogs.ErrDeliverySourceNotFound)
			},
		},
		{
			name: "put_empty_name_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliverySource("", "FLOW_LOGS", nil, nil)
				require.ErrorIs(t, err, cloudwatchlogs.ErrValidationException)
			},
		},
		{
			name: "describe_sorted_by_name",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliverySource("z-src", "FLOW_LOGS", nil, nil)
				require.NoError(t, err)
				_, err = b.PutDeliverySource("a-src", "FLOW_LOGS", nil, nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				srcs, _ := b.DescribeDeliverySources("", 0)
				require.Len(t, srcs, 2)
				assert.Equal(t, "a-src", srcs[0].Name)
				assert.Equal(t, "z-src", srcs[1].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			if tt.setup != nil {
				tt.setup(t, b)
			}
			if tt.verify != nil {
				tt.verify(t, b)
			}
		})
	}
}

func TestUpdateDeliveryConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) string
		verify func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, deliveryID string)
		name   string
	}{
		{
			name: "update_field_delimiter",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) string {
				t.Helper()
				_, err := b.CreateDelivery("src", "arn:aws:logs:::delivery-destination:dst", "", nil, nil, nil)
				require.NoError(t, err)
				deliveries, _, err := b.DescribeDeliveries(100, "")
				require.NoError(t, err)
				require.Len(t, deliveries, 1)
				id := deliveries[0].ID
				err = b.UpdateDeliveryConfiguration(id, ",", nil, nil)
				require.NoError(t, err)

				return id
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, _ string) {
				t.Helper()
				deliveries, _, err := b.DescribeDeliveries(100, "")
				require.NoError(t, err)
				require.Len(t, deliveries, 1)
				assert.Equal(t, ",", deliveries[0].FieldDelimiter)
			},
		},
		{
			name: "update_record_fields",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) string {
				t.Helper()
				_, err := b.CreateDelivery("src", "arn:aws:logs:::delivery-destination:dst", "", nil, nil, nil)
				require.NoError(t, err)
				deliveries, _, err := b.DescribeDeliveries(100, "")
				require.NoError(t, err)
				require.Len(t, deliveries, 1)
				id := deliveries[0].ID
				err = b.UpdateDeliveryConfiguration(id, "", []string{"@timestamp", "@message"}, nil)
				require.NoError(t, err)

				return id
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, _ string) {
				t.Helper()
				deliveries, _, err := b.DescribeDeliveries(100, "")
				require.NoError(t, err)
				assert.Equal(t, []string{"@timestamp", "@message"}, deliveries[0].RecordFields)
			},
		},
		{
			name: "update_both_delimiter_and_fields",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) string {
				t.Helper()
				_, err := b.CreateDelivery("src", "arn:aws:logs:::delivery-destination:dst", "", nil, nil, nil)
				require.NoError(t, err)
				deliveries, _, err := b.DescribeDeliveries(100, "")
				require.NoError(t, err)
				require.Len(t, deliveries, 1)
				id := deliveries[0].ID
				err = b.UpdateDeliveryConfiguration(id, "\t", []string{"@timestamp", "@message", "@logStream"}, nil)
				require.NoError(t, err)

				return id
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, _ string) {
				t.Helper()
				deliveries, _, err := b.DescribeDeliveries(100, "")
				require.NoError(t, err)
				require.Len(t, deliveries, 1)
				assert.Equal(t, "\t", deliveries[0].FieldDelimiter)
				assert.Equal(t, []string{"@timestamp", "@message", "@logStream"}, deliveries[0].RecordFields)
			},
		},
		{
			name: "update_nonexistent_delivery_errors",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) string {
				t.Helper()
				err := b.UpdateDeliveryConfiguration("nonexistent-id", ",", nil, nil)
				require.Error(t, err)

				return ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			var deliveryID string
			if tt.setup != nil {
				deliveryID = tt.setup(t, b)
			}
			if tt.verify != nil {
				tt.verify(t, b, deliveryID)
			}
		})
	}
}
