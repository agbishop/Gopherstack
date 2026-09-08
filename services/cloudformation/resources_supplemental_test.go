package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// TestResourceCreator_SupplementalTypes_NilBackends ensures every supplemental resource type
// returns a stub physical ID (no panic, no error) when the backing service is nil.
func TestResourceCreator_SupplementalTypes_NilBackends(t *testing.T) {
	t.Parallel()

	tests := []struct {
		props        map[string]any
		name         string
		logicalID    string
		resourceType string
	}{
		{name: "logs_log_stream", logicalID: "Stream", resourceType: "AWS::Logs::LogStream",
			props: map[string]any{"LogGroupName": "/g", "LogStreamName": "s"}},
		{name: "logs_metric_filter", logicalID: "MF", resourceType: "AWS::Logs::MetricFilter",
			props: map[string]any{"LogGroupName": "/g", "FilterName": "mf"}},
		{name: "logs_subscription_filter", logicalID: "SF", resourceType: "AWS::Logs::SubscriptionFilter",
			props: map[string]any{"LogGroupName": "/g", "DestinationArn": "arn:aws:lambda:::f"}},
		{name: "logs_resource_policy", logicalID: "RP", resourceType: "AWS::Logs::ResourcePolicy",
			props: map[string]any{"PolicyName": "p", "PolicyDocument": "{}"}},
		{name: "logs_query_definition", logicalID: "QD", resourceType: "AWS::Logs::QueryDefinition",
			props: map[string]any{"Name": "q", "QueryString": "fields @message"}},
		{name: "ec2_volume", logicalID: "Vol", resourceType: "AWS::EC2::Volume",
			props: map[string]any{"AvailabilityZone": "us-east-1a", "Size": float64(10)}},
		{name: "ec2_volume_attachment", logicalID: "VA", resourceType: "AWS::EC2::VolumeAttachment",
			props: map[string]any{"VolumeId": "vol-1", "InstanceId": "i-1"}},
		{name: "ec2_network_interface", logicalID: "ENI", resourceType: "AWS::EC2::NetworkInterface",
			props: map[string]any{"SubnetId": "subnet-1"}},
		{name: "apigwv2_integration", logicalID: "Int", resourceType: "AWS::ApiGatewayV2::Integration",
			props: map[string]any{"ApiId": "api-1", "IntegrationType": "AWS_PROXY"}},
		{name: "apigwv2_route", logicalID: "Route", resourceType: "AWS::ApiGatewayV2::Route",
			props: map[string]any{"ApiId": "api-1", "RouteKey": "GET /"}},
		{name: "apigwv2_authorizer", logicalID: "Auth", resourceType: "AWS::ApiGatewayV2::Authorizer",
			props: map[string]any{"ApiId": "api-1", "Name": "a", "AuthorizerType": "REQUEST"}},
		{name: "kms_alias", logicalID: "Alias", resourceType: "AWS::KMS::Alias",
			props: map[string]any{"AliasName": "alias/k", "TargetKeyId": "key-1"}},
		{name: "sns_topic_policy", logicalID: "TP", resourceType: "AWS::SNS::TopicPolicy",
			props: map[string]any{"Topics": []any{"arn:aws:sns:::t"}, "PolicyDocument": "{}"}},
		{name: "events_connection", logicalID: "Conn", resourceType: "AWS::Events::Connection",
			props: map[string]any{"Name": "c", "AuthorizationType": "API_KEY"}},
		{name: "events_archive", logicalID: "Arch", resourceType: "AWS::Events::Archive",
			props: map[string]any{"ArchiveName": "a", "SourceArn": "arn:aws:events:::event-bus/default"}},
		{name: "sfn_activity", logicalID: "Act", resourceType: "AWS::StepFunctions::Activity",
			props: map[string]any{"Name": "act"}},
		{name: "ssm_document", logicalID: "Doc", resourceType: "AWS::SSM::Document",
			props: map[string]any{"Name": "d", "Content": "{}", "DocumentType": "Command"}},
		{name: "secrets_resource_policy", logicalID: "SRP", resourceType: "AWS::SecretsManager::ResourcePolicy",
			props: map[string]any{"SecretId": "s", "ResourcePolicy": "{}"}},
		{name: "cloudfront_function", logicalID: "Fn", resourceType: "AWS::CloudFront::Function",
			props: map[string]any{"Name": "fn"}},
		{name: "cloudfront_cache_policy", logicalID: "CP", resourceType: "AWS::CloudFront::CachePolicy",
			props: map[string]any{"CachePolicyConfig": map[string]any{"Name": "cp"}}},
		{name: "cloudfront_oac", logicalID: "OAC", resourceType: "AWS::CloudFront::OriginAccessControl",
			props: map[string]any{"OriginAccessControlConfig": map[string]any{"Name": "oac"}}},
		{name: "cloudfront_rhp", logicalID: "RHP", resourceType: "AWS::CloudFront::ResponseHeadersPolicy",
			props: map[string]any{"ResponseHeadersPolicyConfig": map[string]any{"Name": "rhp"}}},
		{
			name:         "app_autoscaling_scalable_target",
			logicalID:    "MyScalableTarget",
			resourceType: "AWS::ApplicationAutoScaling::ScalableTarget",
			props: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/web",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       float64(1),
				"MaxCapacity":       float64(5),
			},
		},
		{
			name:         "app_autoscaling_scaling_policy",
			logicalID:    "MyScalingPolicy",
			resourceType: "AWS::ApplicationAutoScaling::ScalingPolicy",
			props: map[string]any{
				"PolicyName":        "stub-policy",
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/web",
				"ScalableDimension": "ecs:service:DesiredCount",
			},
		},
		{
			name:         "secretsmanager_rotation_schedule",
			logicalID:    "MyRotation",
			resourceType: "AWS::SecretsManager::RotationSchedule",
			props:        map[string]any{"SecretId": "arn:aws:secretsmanager:us-east-1:000000000000:secret:MySecret"},
		},
		{
			name:         "secretsmanager_secret_target_attachment",
			logicalID:    "MyAttachment",
			resourceType: "AWS::SecretsManager::SecretTargetAttachment",
			props: map[string]any{
				"SecretId":   "arn:aws:secretsmanager:us-east-1:000000000000:secret:MySecret",
				"TargetId":   "db-instance-id",
				"TargetType": "AWS::RDS::DBInstance",
			},
		},
		{
			name:         "ssm_maintenance_window",
			logicalID:    "MyMW",
			resourceType: "AWS::SSM::MaintenanceWindow",
			props: map[string]any{
				"Name":     "stub-mw",
				"Schedule": "cron(0 2 ? * SUN *)",
				"Duration": float64(4),
				"Cutoff":   float64(1),
			},
		},
		{
			name:         "ssm_association",
			logicalID:    "MyAssociation",
			resourceType: "AWS::SSM::Association",
			props:        map[string]any{"Name": "AWS-RunShellScript"},
		},
		{
			name:         "dynamodb_global_table",
			logicalID:    "MyGlobalTable",
			resourceType: "AWS::DynamoDB::GlobalTable",
			props:        map[string]any{"TableName": "stub-global"},
		},
		{
			name:         "glue_crawler",
			logicalID:    "MyCrawler",
			resourceType: "AWS::Glue::Crawler",
			props:        map[string]any{"Name": "stub-crawler", "Role": "AWSGlueRole"},
		},
		{
			name:         "glue_table",
			logicalID:    "MyTable",
			resourceType: "AWS::Glue::Table",
			props: map[string]any{
				"DatabaseName": "default",
				"TableInput":   map[string]any{"Name": "stub-table"},
			},
		},
		{
			name:         "glue_trigger",
			logicalID:    "MyTrigger",
			resourceType: "AWS::Glue::Trigger",
			props:        map[string]any{"Name": "stub-trigger", "Type": "ON_DEMAND"},
		},
		{
			name:         "glue_connection",
			logicalID:    "MyConnection",
			resourceType: "AWS::Glue::Connection",
			props: map[string]any{
				"ConnectionInput": map[string]any{
					"Name":           "stub-conn",
					"ConnectionType": "JDBC",
				},
			},
		},
		{
			name:         "glue_partition",
			logicalID:    "MyPartition",
			resourceType: "AWS::Glue::Partition",
			props: map[string]any{
				"DatabaseName": "default",
				"TableName":    "stub-table",
				"PartitionInput": map[string]any{
					"Values": []any{"2024"},
				},
			},
		},
		{
			name:         "appsync_datasource",
			logicalID:    "MyDS",
			resourceType: "AWS::AppSync::DataSource",
			props:        map[string]any{"ApiId": "api-stub", "Name": "stub-ds", "Type": "NONE"},
		},
		{
			name:         "appsync_resolver",
			logicalID:    "MyResolver",
			resourceType: "AWS::AppSync::Resolver",
			props: map[string]any{
				"ApiId":          "api-stub",
				"TypeName":       "Query",
				"FieldName":      "getStub",
				"DataSourceName": "stub-ds",
			},
		},
		{
			name:         "appsync_function_configuration",
			logicalID:    "MyFunction",
			resourceType: "AWS::AppSync::FunctionConfiguration",
			props: map[string]any{
				"ApiId":          "api-stub",
				"Name":           "stub-fn",
				"DataSourceName": "stub-ds",
			},
		},
		{
			name:         "appsync_api_key",
			logicalID:    "MyApiKey",
			resourceType: "AWS::AppSync::ApiKey",
			props:        map[string]any{"ApiId": "api-stub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rc := cloudformation.NewResourceCreator(&cloudformation.ServiceBackends{
				AccountID: "000000000000",
				Region:    "us-east-1",
			})

			physID, err := rc.Create(t.Context(), tt.logicalID, tt.resourceType, tt.props, nil, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, physID)

			require.NoError(t, rc.Delete(t.Context(), tt.resourceType, physID, tt.props))
		})
	}
}

// TestResourceCreator_SupplementalTypes_RealBackends validates create and delete with real in-memory backends.
func TestResourceCreator_SupplementalTypes_RealBackends(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFn      func(*cloudformation.ServiceBackends)
		props        map[string]any
		name         string
		logicalID    string
		resourceType string
	}{
		{
			name:         "app_autoscaling_scalable_target",
			logicalID:    "MyScalableTarget",
			resourceType: "AWS::ApplicationAutoScaling::ScalableTarget",
			props: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/web",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       float64(1),
				"MaxCapacity":       float64(10),
			},
		},
		{
			name:         "app_autoscaling_scaling_policy",
			logicalID:    "MyScalingPolicy",
			resourceType: "AWS::ApplicationAutoScaling::ScalingPolicy",
			props: map[string]any{
				"PolicyName":        "unit-cfn-policy",
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/web",
				"ScalableDimension": "ecs:service:DesiredCount",
				"PolicyType":        "StepScaling",
			},
		},
		{
			name:         "secretsmanager_rotation_schedule",
			logicalID:    "MyRotation",
			resourceType: "AWS::SecretsManager::RotationSchedule",
			props: map[string]any{
				"SecretId":          "MySecret",
				"RotationLambdaARN": "arn:aws:lambda:us-east-1:000000000000:function:unit-cfn-rotator",
			},
		},
		{
			name:         "secretsmanager_secret_target_attachment",
			logicalID:    "MyAttachment",
			resourceType: "AWS::SecretsManager::SecretTargetAttachment",
			props: map[string]any{
				"SecretId":   "arn:aws:secretsmanager:us-east-1:000000000000:secret:MySecret",
				"TargetId":   "db-12345",
				"TargetType": "AWS::RDS::DBInstance",
			},
		},
		{
			name:         "ssm_maintenance_window",
			logicalID:    "MyMW",
			resourceType: "AWS::SSM::MaintenanceWindow",
			props: map[string]any{
				"Name":     "unit-cfn-mw",
				"Schedule": "cron(0 2 ? * SUN *)",
				"Duration": float64(4),
				"Cutoff":   float64(1),
			},
		},
		{
			name:         "glue_crawler",
			logicalID:    "MyCrawler",
			resourceType: "AWS::Glue::Crawler",
			props: map[string]any{
				"Name": "unit-cfn-crawler",
				"Role": "AWSGlueServiceRole",
			},
		},
		{
			name:         "glue_trigger",
			logicalID:    "MyTrigger",
			resourceType: "AWS::Glue::Trigger",
			props: map[string]any{
				"Name": "unit-cfn-trigger",
				"Type": "ON_DEMAND",
			},
		},
		{
			name:         "glue_connection",
			logicalID:    "MyConnection",
			resourceType: "AWS::Glue::Connection",
			props: map[string]any{
				"ConnectionInput": map[string]any{
					"Name":           "unit-cfn-conn",
					"ConnectionType": "JDBC",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backends := newExtraServiceBackends(t)
			rc := cloudformation.NewResourceCreator(backends)

			// A ScalingPolicy requires its ScalableTarget to be registered first
			// (real CloudFormation models the target as a separate resource).
			if tt.resourceType == "AWS::ApplicationAutoScaling::ScalingPolicy" {
				_, regErr := backends.AppAutoScaling.Backend.RegisterScalableTarget(
					tt.props["ServiceNamespace"].(string),
					tt.props["ResourceId"].(string),
					tt.props["ScalableDimension"].(string),
					int32p(1), int32p(10), nil, "", nil,
				)
				require.NoError(t, regErr)
			}

			// A RotationSchedule now really calls RotateSecret, which requires the
			// referenced secret to exist and already have a current version.
			if tt.resourceType == "AWS::SecretsManager::RotationSchedule" {
				_, secretErr := rc.Create(t.Context(), "MySecret", "AWS::SecretsManager::Secret",
					map[string]any{"Name": "MySecret", "SecretString": "s3cr3t"}, nil, nil)
				require.NoError(t, secretErr)
			}

			physID, err := rc.Create(t.Context(), tt.logicalID, tt.resourceType, tt.props, nil, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, physID)

			err = rc.Delete(t.Context(), tt.resourceType, physID, nil)
			require.NoError(t, err)
		})
	}
}
