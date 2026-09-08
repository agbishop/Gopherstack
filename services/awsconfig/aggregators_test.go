package awsconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

func TestAWSConfigBackend_DeleteAggregationAuthorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, b *awsconfig.InMemoryBackend)
		name      string
		accountID string
		region    string
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutAggregationAuthorization("123456789012", "us-east-1", nil))
			},
			accountID: "123456789012",
			region:    "us-east-1",
		},
		{
			// Real AWS Config's DeleteAggregationAuthorization is idempotent (its
			// declared error model has no not-found exception), so deleting a
			// nonexistent authorization also succeeds.
			name:      "not_found_is_idempotent",
			accountID: "999999999999",
			region:    "eu-west-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			require.NoError(t, b.DeleteAggregationAuthorization(tt.accountID, tt.region))
		})
	}
}

func TestAWSConfigBackend_DeleteConfigurationAggregator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, b *awsconfig.InMemoryBackend)
		name    string
		delName string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigurationAggregator("agg1", nil, nil, nil))
			},
			delName: "agg1",
		},
		{
			name:    "not_found",
			delName: "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := b.DeleteConfigurationAggregator(tt.delName)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAWSConfigBackend_DescribeAggregationAuthorizations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, b *awsconfig.InMemoryBackend)
		name      string
		wantCount int
	}{
		{
			name:      "empty",
			wantCount: 0,
		},
		{
			name: "multiple_sorted",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutAggregationAuthorization("222222222222", "us-west-2", nil))
				require.NoError(t, b.PutAggregationAuthorization("111111111111", "us-east-1", nil))
				require.NoError(t, b.PutAggregationAuthorization("111111111111", "eu-west-1", nil))
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			auths := b.DescribeAggregationAuthorizations()
			assert.Len(t, auths, tt.wantCount)

			for i := 1; i < len(auths); i++ {
				prev := auths[i-1].AuthorizedAccountID + "#" + auths[i-1].AuthorizedAwsRegion
				curr := auths[i].AuthorizedAccountID + "#" + auths[i].AuthorizedAwsRegion
				assert.LessOrEqual(t, prev, curr)
			}
		})
	}
}

func TestAWSConfigBackend_DescribeConfigurationAggregatorSourcesStatus(t *testing.T) {
	t.Parallel()

	t.Run("unknown_aggregator_errors", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.DescribeConfigurationAggregatorSourcesStatus("does-not-exist")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNoSuchAggregator)
	})

	t.Run("reports_configured_account_sources", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationAggregator("agg1", []awsconfig.AccountAggregationSource{
			{AccountIDs: []string{"111111111111", "222222222222"}, AwsRegions: []string{"us-east-1"}},
		}, nil, nil))

		statuses, err := b.DescribeConfigurationAggregatorSourcesStatus("agg1")
		require.NoError(t, err)
		require.Len(t, statuses, 2)

		for _, s := range statuses {
			assert.Equal(t, "us-east-1", s.AwsRegion)
			assert.Equal(t, "SUCCEEDED", s.LastUpdateStatus)
			assert.Equal(t, "ACCOUNT", s.SourceType)
		}
	})

	t.Run("reports_organization_source", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationAggregator(
			"agg1", nil, &awsconfig.OrganizationAggregationSource{RoleArn: "arn:aws:iam::123:role/org"}, nil,
		))

		statuses, err := b.DescribeConfigurationAggregatorSourcesStatus("agg1")
		require.NoError(t, err)
		require.Len(t, statuses, 1)
		assert.Equal(t, "ORGANIZATION", statuses[0].SourceType)
	})
}

// delete_validation's wantErr was awsconfig.ErrValidation until this pass;
// DeletePendingAggregationRequest's deserializer declares only
// InvalidParameterValueException, never ValidationException
// (configservice@v1.68.4 deserializers.go).
func TestAWSConfigBackend_PendingAggregationRequests(t *testing.T) {
	t.Parallel()

	t.Run("authorization_with_no_consuming_aggregator_is_pending", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutAggregationAuthorization("111111111111", "us-east-1", nil))

		pending := b.DescribePendingAggregationRequests()
		require.Len(t, pending, 1)
		assert.Equal(t, "111111111111", pending[0].RequesterAccountID)
		assert.Equal(t, "us-east-1", pending[0].RequesterAwsRegion)
	})

	t.Run("authorization_consumed_by_aggregator_is_not_pending", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutAggregationAuthorization("111111111111", "us-east-1", nil))
		require.NoError(t, b.PutConfigurationAggregator("agg1", []awsconfig.AccountAggregationSource{
			{AccountIDs: []string{"111111111111"}, AwsRegions: []string{"us-east-1"}},
		}, nil, nil))

		assert.Empty(t, b.DescribePendingAggregationRequests())
	})

	t.Run("delete_removes_the_request", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutAggregationAuthorization("111111111111", "us-east-1", nil))
		require.NoError(t, b.DeletePendingAggregationRequest("111111111111", "us-east-1"))
		assert.Empty(t, b.DescribePendingAggregationRequests())
	})

	t.Run("delete_validation", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		err := b.DeletePendingAggregationRequest("", "us-east-1")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrInvalidParameterValue)
	})
}

// TestAWSConfigBackend_DeleteAggregationAuthorization_Validation's wantErr was
// awsconfig.ErrValidation until this pass; DeleteAggregationAuthorization's
// deserializer declares only InvalidParameterValueException, never
// ValidationException (configservice@v1.68.4 deserializers.go) -- the old
// assertion locked in the exact wire-code defect this pass fixed.
func TestAWSConfigBackend_DeleteAggregationAuthorization_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		accountID string
		region    string
	}{
		{
			name:      "empty_account_id_fails",
			accountID: "",
			region:    "us-east-1",
			wantErr:   awsconfig.ErrInvalidParameterValue,
		},
		{
			name:      "empty_region_fails",
			accountID: "123456789012",
			region:    "",
			wantErr:   awsconfig.ErrInvalidParameterValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			err := b.DeleteAggregationAuthorization(tt.accountID, tt.region)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
