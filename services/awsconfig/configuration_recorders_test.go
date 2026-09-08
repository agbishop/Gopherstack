package awsconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

func TestAWSConfigBackend_PutConfigurationRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		recName    string
		roleARN    string
		wantName   string
		wantStatus string
		wantLen    int
	}{
		{
			name:       "success",
			recName:    "default",
			roleARN:    "arn:aws:iam::000000000000:role/config",
			wantLen:    1,
			wantName:   "default",
			wantStatus: "PENDING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			err := b.PutConfigurationRecorder(tt.recName, tt.roleARN, nil)
			require.NoError(t, err)

			recorders := b.DescribeConfigurationRecorders(nil)
			require.Len(t, recorders, tt.wantLen)
			assert.Equal(t, tt.wantName, recorders[0].Name)
			assert.Equal(t, tt.wantStatus, recorders[0].Status)
		})
	}
}

func TestAWSConfigBackend_PutConfigurationRecorder_MaxOneCustomerManaged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, b *awsconfig.InMemoryBackend)
		wantErr error
		name    string
		recName string
	}{
		{
			name:    "second_customer_managed_recorder_rejected",
			recName: "second",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))
			},
			wantErr: awsconfig.ErrAlreadyExists,
		},
		{
			name:    "same_name_update_allowed",
			recName: "default",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))
			},
			wantErr: nil,
		},
		{
			name:    "service_linked_recorder_does_not_block",
			recName: "customer-managed",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				_, _, err := b.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
				require.NoError(t, err)
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			tt.setup(t, b)

			err := b.PutConfigurationRecorder(tt.recName, "arn:aws:iam::000000000000:role/config", nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAWSConfigBackend_StartConfigurationRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		recName    string
		setup      func(t *testing.T, b *awsconfig.InMemoryBackend)
		wantErr    error
		wantStatus string
	}{
		{
			name:    "success",
			recName: "default",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))
				require.NoError(t, b.PutDeliveryChannel("default", "my-bucket", "", "", nil))
			},
			wantStatus: "ACTIVE",
		},
		{
			name:    "not_found",
			recName: "nonexistent",
			wantErr: awsconfig.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := b.StartConfigurationRecorder(tt.recName)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			recorders := b.DescribeConfigurationRecorders(nil)
			require.Len(t, recorders, 1)
			assert.Equal(t, tt.wantStatus, recorders[0].Status)
		})
	}
}

func TestAWSConfigBackend_DescribeConfigurationRecorders(t *testing.T) {
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
			name: "one_recorder",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			recorders := b.DescribeConfigurationRecorders(nil)
			assert.Len(t, recorders, tt.wantCount)
		})
	}
}

func TestAWSConfigBackend_AssociateResourceTypes(t *testing.T) {
	t.Parallel()

	t.Run("known_recorder_by_name_mutates_recording_group", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))

		recorder, err := b.AssociateResourceTypes("default", []string{"AWS::EC2::Instance"})
		require.NoError(t, err)
		require.NotNil(t, recorder)
		assert.Equal(t, "default", recorder.Name)
		assert.Contains(t, recorder.Arn, "config-recorder/default")
		require.NotNil(t, recorder.RecordingGroup)
		assert.Equal(t, []string{"AWS::EC2::Instance"}, recorder.RecordingGroup.ResourceTypes)

		// The mutation is persisted on the backend, not just the returned copy.
		recs := b.DescribeConfigurationRecorders([]string{"default"})
		require.Len(t, recs, 1)
		require.NotNil(t, recs[0].RecordingGroup)
		assert.Equal(t, []string{"AWS::EC2::Instance"}, recs[0].RecordingGroup.ResourceTypes)
	})

	t.Run("known_recorder_by_full_arn", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackendWithMeta("000000000000", "us-east-1")
		require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))

		recorder, err := b.AssociateResourceTypes(
			"arn:aws:config:us-east-1:000000000000:config-recorder/default",
			[]string{"AWS::S3::Bucket"},
		)
		require.NoError(t, err)
		require.NotNil(t, recorder)
		assert.Equal(t, "default", recorder.Name)
	})

	t.Run("dedups_repeated_resource_types", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))

		_, err := b.AssociateResourceTypes("default", []string{"AWS::EC2::Instance"})
		require.NoError(t, err)
		recorder, err := b.AssociateResourceTypes("default", []string{"AWS::EC2::Instance", "AWS::S3::Bucket"})
		require.NoError(t, err)
		assert.Equal(t, []string{"AWS::EC2::Instance", "AWS::S3::Bucket"}, recorder.RecordingGroup.ResourceTypes)
	})

	t.Run("unknown_recorder_errors_not_found", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.AssociateResourceTypes(
			"arn:aws:config:us-east-1:000000000000:config-recorder/unknown",
			[]string{"AWS::S3::Bucket"},
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNotFound)
	})
}

func TestAWSConfigBackend_DisassociateResourceTypes(t *testing.T) {
	t.Parallel()

	t.Run("removes_resource_types", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/config", nil))
		_, err := b.AssociateResourceTypes("default", []string{"AWS::EC2::Instance", "AWS::S3::Bucket"})
		require.NoError(t, err)

		require.NoError(t, b.DisassociateResourceTypes("default", []string{"AWS::EC2::Instance"}))

		recs := b.DescribeConfigurationRecorders([]string{"default"})
		require.Len(t, recs, 1)
		require.NotNil(t, recs[0].RecordingGroup)
		assert.Equal(t, []string{"AWS::S3::Bucket"}, recs[0].RecordingGroup.ResourceTypes)
	})

	t.Run("unknown_recorder_errors_not_found", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		err := b.DisassociateResourceTypes("unknown", []string{"AWS::EC2::Instance"})
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNotFound)
	})

	t.Run("empty_arn_is_validation_error", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		err := b.DisassociateResourceTypes("", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrValidation)
	})
}

func TestAWSConfigBackend_StopConfigurationRecorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, b *awsconfig.InMemoryBackend)
		name       string
		recName    string
		wantErr    error
		wantStatus string
	}{
		{
			name: "stops_active_recorder",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::123:role/r", nil))
				require.NoError(t, b.PutDeliveryChannel("default", "my-bucket", "", "", nil))
				require.NoError(t, b.StartConfigurationRecorder("default"))
			},
			recName:    "default",
			wantStatus: "PENDING",
		},
		{
			name:    "not_found",
			recName: "nonexistent",
			wantErr: awsconfig.ErrNotFound,
		},
		{
			name:    "empty_name_returns_validation_error",
			recName: "",
			wantErr: awsconfig.ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := b.StopConfigurationRecorder(tt.recName)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			statuses := b.DescribeConfigurationRecorderStatus(nil)
			require.Len(t, statuses, 1)
			assert.False(t, statuses[0].Recording)
		})
	}
}

func TestAWSConfigBackend_PutConfigurationRecorder_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		name    string
		recName string
		roleARN string
	}{
		{
			name:    "empty_name_fails",
			recName: "",
			roleARN: "arn:aws:iam::000000000000:role/r",
			wantErr: awsconfig.ErrInvalidConfigurationRecorderName,
		},
		{
			name:    "empty_roleARN_fails",
			recName: "default",
			roleARN: "",
			wantErr: awsconfig.ErrInvalidRole,
		},
		{
			name:    "update_preserves_status",
			recName: "default",
			roleARN: "arn:aws:iam::000000000000:role/new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.name == "update_preserves_status" {
				require.NoError(t, b.PutConfigurationRecorder("default", "arn:aws:iam::000000000000:role/old", nil))
				require.NoError(t, b.PutDeliveryChannel("default", "bucket", "", "", nil))
				require.NoError(t, b.StartConfigurationRecorder("default"))
			}

			err := b.PutConfigurationRecorder(tt.recName, tt.roleARN, nil)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.name == "update_preserves_status" {
				recorders := b.DescribeConfigurationRecorders(nil)
				require.Len(t, recorders, 1)
				assert.Equal(t, "ACTIVE", recorders[0].Status)
				assert.Equal(t, tt.roleARN, recorders[0].RoleARN)
			}
		})
	}
}

func TestAWSConfigBackend_DescribeConfigurationRecorders_NameFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    []string
		wantCount int
	}{
		{
			name:      "no_filter_returns_all_sorted",
			wantCount: 3,
		},
		{
			name:      "filter_one",
			filter:    []string{"rec-a"},
			wantCount: 1,
		},
		{
			name:      "filter_nonexistent",
			filter:    []string{"no-such"},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			require.NoError(t, b.PutConfigurationRecorder("rec-a", "arn:aws:iam::123:role/r", nil))
			_, _, err := b.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
			require.NoError(t, err)
			_, _, err = b.PutServiceLinkedConfigurationRecorder("backup.amazonaws.com", nil)
			require.NoError(t, err)

			recs := b.DescribeConfigurationRecorders(tt.filter)
			assert.Len(t, recs, tt.wantCount)

			if tt.wantCount > 1 && len(tt.filter) == 0 {
				// verify sorted
				for i := 1; i < len(recs); i++ {
					assert.Less(t, recs[i-1].Name, recs[i].Name)
				}
			}
		})
	}
}

func TestAWSConfigBackend_AssociateResourceTypes_EmptyARN(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_, err := b.AssociateResourceTypes("", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, awsconfig.ErrValidation)
}

func TestAWSConfigBackend_DeleteConfigurationRecorder(t *testing.T) {
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
				require.NoError(t, b.PutConfigurationRecorder("rec1", "arn:aws:iam::000000000000:role/r", nil))
			},
			delName: "rec1",
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

			err := b.DeleteConfigurationRecorder(tt.delName)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAWSConfigBackend_ServiceLinkedConfigurationRecorder(t *testing.T) {
	t.Parallel()

	t.Run("empty_service_principal_fails", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, _, err := b.PutServiceLinkedConfigurationRecorder("", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrValidation)
	})

	t.Run("create_is_active_and_appears_in_describe", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		name, arn, err := b.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
		require.NoError(t, err)
		assert.Equal(t, "AWSConfigurationRecorderForGuardduty", name)
		assert.Contains(t, arn, "config-recorder/"+name)

		statuses := b.DescribeConfigurationRecorderStatus([]string{name})
		require.Len(t, statuses, 1)
		assert.True(t, statuses[0].Recording)
	})

	t.Run("put_is_idempotent_per_principal", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		name1, arn1, err := b.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
		require.NoError(t, err)
		name2, arn2, err := b.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
		require.NoError(t, err)

		assert.Equal(t, name1, name2)
		assert.Equal(t, arn1, arn2)
		assert.Len(t, b.DescribeConfigurationRecorders(nil), 1)
	})

	t.Run("delete_unknown_principal_errors_not_found", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, _, err := b.DeleteServiceLinkedConfigurationRecorder("guardduty.amazonaws.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNotFound)
	})

	t.Run("delete_removes_recorder", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		name, _, err := b.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
		require.NoError(t, err)

		delName, delArn, err := b.DeleteServiceLinkedConfigurationRecorder("guardduty.amazonaws.com")
		require.NoError(t, err)
		assert.Equal(t, name, delName)
		assert.NotEmpty(t, delArn)
		assert.Empty(t, b.DescribeConfigurationRecorders(nil))
	})

	t.Run("survives_snapshot_restore", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		name, _, err := b.PutServiceLinkedConfigurationRecorder("guardduty.amazonaws.com", nil)
		require.NoError(t, err)

		snap := b.Snapshot(t.Context())
		require.NotNil(t, snap)

		fresh := awsconfig.NewInMemoryBackend()
		require.NoError(t, fresh.Restore(t.Context(), snap))

		// The servicePrincipal -> recorder-name link must have survived the
		// round trip, not just the ConfigurationRecorder row itself.
		delName, _, err := fresh.DeleteServiceLinkedConfigurationRecorder("guardduty.amazonaws.com")
		require.NoError(t, err)
		assert.Equal(t, name, delName)
	})
}

// azureConfig is a shared valid ConnectorConfiguration used across the
// Connector/PutThirdPartyServiceLinkedConfigurationRecorder tests below.
func azureConfig(clientID, tenantID string) *awsconfig.ConnectorConfiguration {
	return &awsconfig.ConnectorConfiguration{
		Azure: &awsconfig.AzureConnectorConfiguration{ClientIdentifier: clientID, TenantIdentifier: tenantID},
	}
}

func TestAWSConfigBackend_Connectors(t *testing.T) {
	t.Parallel()

	t.Run("nil_configuration_fails_validation", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.PutConnector(nil, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrValidation)
	})

	t.Run("empty_azure_fields_fail_validation", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.PutConnector(azureConfig("", "tenant-1"), nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrValidation)
	})

	t.Run("create_get_list_roundtrip", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		arn, err := b.PutConnector(azureConfig("client-1", "tenant-1"), []awsconfig.Tag{{Key: "env", Value: "prod"}})
		require.NoError(t, err)
		assert.Contains(t, arn, "connector/")

		got, err := b.GetConnector(arn)
		require.NoError(t, err)
		assert.Equal(t, arn, got.Arn)
		require.NotNil(t, got.ConnectorConfiguration)
		require.NotNil(t, got.ConnectorConfiguration.Azure)
		assert.Equal(t, "client-1", got.ConnectorConfiguration.Azure.ClientIdentifier)

		summaries := b.ListConnectors(nil)
		require.Len(t, summaries, 1)
		assert.Equal(t, arn, summaries[0].Arn)
		assert.Equal(t, "AZURE", summaries[0].Provider)
		assert.Equal(t, "tenant-1", summaries[0].TenantIdentifier)

		assert.Equal(t, []awsconfig.Tag{{Key: "env", Value: "prod"}}, b.ListTagsForResource(arn))
	})

	t.Run("duplicate_configuration_conflicts_not_upsert", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		arn1, err := b.PutConnector(azureConfig("client-1", "tenant-1"), nil)
		require.NoError(t, err)

		arn2, err := b.PutConnector(azureConfig("client-1", "tenant-1"), nil)
		require.ErrorIs(t, err, awsconfig.ErrConflict)
		assert.Empty(t, arn2)
		assert.NotEmpty(t, arn1)

		// Distinct configuration is a distinct connector, not a conflict.
		arn3, err := b.PutConnector(azureConfig("client-2", "tenant-2"), nil)
		require.NoError(t, err)
		assert.NotEqual(t, arn1, arn3)
		assert.Len(t, b.ListConnectors(nil), 2)
	})

	t.Run("list_filters_by_provider", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.PutConnector(azureConfig("client-1", "tenant-1"), nil)
		require.NoError(t, err)

		matching := b.ListConnectors([]awsconfig.ConnectorFilter{
			{FilterName: "provider", FilterValues: []string{"AZURE"}},
		})
		assert.Len(t, matching, 1)

		none := b.ListConnectors([]awsconfig.ConnectorFilter{
			{FilterName: "provider", FilterValues: []string{"GCP"}},
		})
		assert.Empty(t, none)
	})

	t.Run("get_delete_unknown_arn_errors_resource_not_found", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.GetConnector("arn:aws:config:us-east-1:000000000000:connector/nonexistent")
		require.ErrorIs(t, err, awsconfig.ErrResourceNotFound)

		err = b.DeleteConnector("arn:aws:config:us-east-1:000000000000:connector/nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrResourceNotFound)
	})

	t.Run("delete_removes_connector", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		arn, err := b.PutConnector(azureConfig("client-1", "tenant-1"), nil)
		require.NoError(t, err)

		require.NoError(t, b.DeleteConnector(arn))
		assert.Empty(t, b.ListConnectors(nil))
	})

	t.Run("survives_snapshot_restore", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		arn, err := b.PutConnector(azureConfig("client-1", "tenant-1"), nil)
		require.NoError(t, err)

		snap := b.Snapshot(t.Context())
		require.NotNil(t, snap)

		fresh := awsconfig.NewInMemoryBackend()
		require.NoError(t, fresh.Restore(t.Context(), snap))

		got, err := fresh.GetConnector(arn)
		require.NoError(t, err)
		assert.Equal(t, arn, got.Arn)
	})
}

func TestAWSConfigBackend_PutThirdPartyServiceLinkedConfigurationRecorder(t *testing.T) {
	t.Parallel()

	newConnector := func(t *testing.T, b *awsconfig.InMemoryBackend) string {
		t.Helper()

		arn, err := b.PutConnector(azureConfig("client-1", "tenant-1"), nil)
		require.NoError(t, err)

		return arn
	}

	scope := &awsconfig.ScopeConfiguration{ScopeType: "tenant", AllRegions: true}

	t.Run("missing_required_fields_fail_validation", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		connectorArn := newConnector(t, b)

		_, _, err := b.PutThirdPartyServiceLinkedConfigurationRecorder("", connectorArn, scope, nil)
		require.ErrorIs(t, err, awsconfig.ErrValidation)

		_, _, err = b.PutThirdPartyServiceLinkedConfigurationRecorder("svc.amazonaws.com", "", scope, nil)
		require.ErrorIs(t, err, awsconfig.ErrValidation)

		_, _, err = b.PutThirdPartyServiceLinkedConfigurationRecorder("svc.amazonaws.com", connectorArn, nil, nil)
		require.ErrorIs(t, err, awsconfig.ErrValidation)
	})

	t.Run("unknown_connector_fails_validation", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, _, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", "arn:aws:config:us-east-1:000000000000:connector/nonexistent", scope, nil,
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrValidation)
	})

	t.Run("create_is_active_and_visible_via_describe", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		connectorArn := newConnector(t, b)

		name, arn, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn, scope, nil,
		)
		require.NoError(t, err)
		assert.NotEmpty(t, name)
		assert.NotEmpty(t, arn)

		recorders := b.DescribeConfigurationRecorders([]string{name})
		require.Len(t, recorders, 1)
		assert.Equal(t, "ACTIVE", recorders[0].Status)
		assert.Equal(t, connectorArn, recorders[0].ConnectorArn)
		assert.Equal(t, "svc.amazonaws.com", recorders[0].ServicePrincipal)
		require.NotNil(t, recorders[0].ScopeConfiguration)
		assert.Equal(t, "tenant", recorders[0].ScopeConfiguration.ScopeType)

		statuses := b.DescribeConfigurationRecorderStatus([]string{name})
		require.Len(t, statuses, 1)
		assert.True(t, statuses[0].Recording)
	})

	t.Run("same_connector_is_idempotent_update", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		connectorArn := newConnector(t, b)

		name1, arn1, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn, scope, nil,
		)
		require.NoError(t, err)

		updatedScope := &awsconfig.ScopeConfiguration{ScopeType: "subscription", AllRegions: false}
		name2, arn2, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn, updatedScope, nil,
		)
		require.NoError(t, err)

		assert.Equal(t, name1, name2)
		assert.Equal(t, arn1, arn2)
		assert.Len(t, b.DescribeConfigurationRecorders(nil), 1)

		recorders := b.DescribeConfigurationRecorders([]string{name1})
		require.Len(t, recorders, 1)
		assert.Equal(t, "subscription", recorders[0].ScopeConfiguration.ScopeType)
	})

	t.Run("different_connector_same_principal_conflicts", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		connectorArn1 := newConnector(t, b)

		_, _, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn1, scope, nil,
		)
		require.NoError(t, err)

		connectorArn2, err := b.PutConnector(azureConfig("client-2", "tenant-2"), nil)
		require.NoError(t, err)

		_, _, err = b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn2, scope, nil,
		)
		require.ErrorIs(t, err, awsconfig.ErrConflict)
		assert.Len(t, b.DescribeConfigurationRecorders(nil), 1)
	})

	t.Run("deletable_via_generic_delete_configuration_recorder", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		connectorArn := newConnector(t, b)

		name, _, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn, scope, nil,
		)
		require.NoError(t, err)

		require.NoError(t, b.DeleteConfigurationRecorder(name))
		assert.Empty(t, b.DescribeConfigurationRecorders(nil))

		// A fresh Put for the same principal now creates rather than conflicts.
		name2, _, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn, scope, nil,
		)
		require.NoError(t, err)
		assert.Equal(t, name, name2)
	})

	t.Run("survives_snapshot_restore", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		connectorArn := newConnector(t, b)

		name, _, err := b.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn, scope, nil,
		)
		require.NoError(t, err)

		snap := b.Snapshot(t.Context())
		require.NotNil(t, snap)

		fresh := awsconfig.NewInMemoryBackend()
		require.NoError(t, fresh.Restore(t.Context(), snap))

		// The recorder must be visible AND the servicePrincipal index must
		// still resolve it (not just the raw recorders table), since a
		// second Put after restore should update-not-conflict.
		recorders := fresh.DescribeConfigurationRecorders([]string{name})
		require.Len(t, recorders, 1)

		_, _, err = fresh.PutThirdPartyServiceLinkedConfigurationRecorder(
			"svc.amazonaws.com", connectorArn, scope, nil,
		)
		require.NoError(t, err)
		assert.Len(t, fresh.DescribeConfigurationRecorders(nil), 1)
	})
}
