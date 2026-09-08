package comprehend_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreatePassthroughFields_PresenceValidation covers gopherstack-wl0s:
// CreateFlywheel and CreateEndpoint use the same generic-CRUD CreateResource
// path (store.go) that stores and echoes the whole input map via cloneMap,
// so a supplied value already round-trips fine -- what was missing was
// rejecting a request that omits a field aws-sdk-go-v2/service/
// comprehend@v1.43.4/validators.go marks "This member is required":
// CreateFlywheelInput's DataAccessRoleArn and DataLakeS3Uri, and
// CreateEndpointInput's DesiredInferenceUnits. FlywheelName/EndpointName are
// not covered here because CreateResource's own Name-presence check already
// rejects their absence.
//
// Note DataAccessRoleArn is tested here even though the originating audit
// (gopherstack-wl0s) named only DataLakeS3Uri and DesiredInferenceUnits --
// validateOpCreateFlywheelInput marks DataAccessRoleArn required too.
//
// Also covers CreateDatasetInput's FlywheelArn and InputDataConfig, which
// validateOpCreateDatasetInput likewise marks "This member is required" but
// which requiredResourceFields (store.go) never listed for
// resourceTypeDataset -- CreateDataset accepted a request missing either
// field and echoed it back as a zero value instead of rejecting it.
//
// Each case proves both directions: omitting the field is rejected with
// InvalidRequestException (the code both ops' own
// awsAwsjson11_deserializeOpError<Op> switch declares for
// InvalidRequestException, confirmed per op in deserializers.go), and
// supplying it is accepted and the value round-trips unchanged through the
// matching Describe* operation.
func TestCreatePassthroughFields_PresenceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		validBody    map[string]any
		name         string
		action       string
		describeOp   string
		arnField     string
		missingField string
	}{
		{
			name:   "create_flywheel_data_access_role_arn",
			action: "CreateFlywheel", describeOp: "DescribeFlywheel",
			arnField: "FlywheelArn", missingField: "DataAccessRoleArn",
			validBody: flywheelBody("presence-fw-role"),
		},
		{
			name:   "create_flywheel_data_lake_s3_uri",
			action: "CreateFlywheel", describeOp: "DescribeFlywheel",
			arnField: "FlywheelArn", missingField: "DataLakeS3Uri",
			validBody: flywheelBody("presence-fw-lake"),
		},
		{
			name:   "create_endpoint_desired_inference_units",
			action: "CreateEndpoint", describeOp: "DescribeEndpoint",
			arnField: "EndpointArn", missingField: "DesiredInferenceUnits",
			validBody: endpointBody("presence-ep-units"),
		},
		{
			name:   "create_dataset_flywheel_arn",
			action: "CreateDataset", describeOp: "DescribeDataset",
			arnField: "DatasetArn", missingField: "FlywheelArn",
			validBody: datasetBody("presence-ds-flywheel"),
		},
		{
			name:   "create_dataset_input_data_config",
			action: "CreateDataset", describeOp: "DescribeDataset",
			arnField: "DatasetArn", missingField: "InputDataConfig",
			validBody: datasetBody("presence-ds-config"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_missing_rejected", func(t *testing.T) {
			t.Parallel()

			body := mergedBody(tt.validBody, nil)
			delete(body, tt.missingField)

			rec := rawRequest(t, newHandler(), tt.action, toJSON(t, body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			resp := decodeBody(t, rec)
			assert.Equal(t, "InvalidRequestException", resp["__type"])
		})

		t.Run(tt.name+"_present_round_trips", func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			created := request(t, h, tt.action, tt.validBody)
			arn, ok := created[tt.arnField].(string)
			require.True(t, ok)

			described := request(t, h, tt.describeOp, map[string]any{tt.arnField: arn})
			propsField := tt.describeOp[len("Describe"):] + "Properties"
			props, ok := described[propsField].(map[string]any)
			require.True(t, ok)

			want := tt.validBody[tt.missingField]
			if wantInt, isInt := want.(int); isInt {
				assert.InEpsilon(t, float64(wantInt), props[tt.missingField], 0)
			} else {
				assert.Equal(t, want, props[tt.missingField])
			}
		})
	}
}
