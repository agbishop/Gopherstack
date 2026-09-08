package pipes_test

// ECS and AWS Batch targets have by far the deepest parameter surface of any
// Pipes target (network config, capacity providers, placement, overrides for
// ECS; dependencies, container overrides, array/retry config for Batch), so
// they get their own file split out of targets_test.go.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

// TestTargetParams_Batch verifies Batch job target parameters.
func TestTargetParams_Batch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		jobDefinition string
		jobName       string
		arraySize     int
		retryAttempts int
	}{
		{
			name:          "basic_batch",
			jobDefinition: "arn:aws:batch:us-west-2:123456789012:job-definition/my-job",
			jobName:       "my-job-run",
		},
		{
			name:          "array_with_retry",
			jobDefinition: "arn:aws:batch:us-west-2:123456789012:job-definition/array-job",
			jobName:       "array-run",
			arraySize:     10,
			retryAttempts: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			batchParams := map[string]any{
				"JobDefinition": tt.jobDefinition,
				"JobName":       tt.jobName,
			}
			if tt.arraySize > 0 {
				batchParams["ArrayProperties"] = map[string]any{"Size": tt.arraySize}
			}
			if tt.retryAttempts > 0 {
				batchParams["RetryStrategy"] = map[string]any{"Attempts": tt.retryAttempts}
			}

			resp := auditCreate(t, h, tt.name+"-batch-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:batch:us-west-2:123456789012:job-queue/my-queue",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"BatchJobParameters": batchParams,
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			bp, _ := tp["BatchJobParameters"].(map[string]any)
			require.NotNil(t, bp, "BatchJobParameters missing")
			assert.Equal(t, tt.jobDefinition, bp["JobDefinition"])
			assert.Equal(t, tt.jobName, bp["JobName"])
		})
	}
}

// TestTargetParams_ECS verifies ECS task target parameters.
func TestTargetParams_ECS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		taskDefinitionArn string
		launchType        string
		taskCount         int
	}{
		{
			name:              "fargate_task",
			taskDefinitionArn: "arn:aws:ecs:us-west-2:123456789012:task-definition/my-task:1",
			launchType:        "FARGATE",
			taskCount:         1,
		},
		{
			name:              "ec2_task_multi",
			taskDefinitionArn: "arn:aws:ecs:us-west-2:123456789012:task-definition/batch-task:2",
			launchType:        "EC2",
			taskCount:         5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			resp := auditCreate(t, h, tt.name+"-ecs-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:ecs:us-west-2:123456789012:cluster/my-cluster",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn": tt.taskDefinitionArn,
						"LaunchType":        tt.launchType,
						"TaskCount":         tt.taskCount,
					},
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			ep, _ := tp["EcsTaskParameters"].(map[string]any)
			require.NotNil(t, ep, "EcsTaskParameters missing")
			assert.Equal(t, tt.taskDefinitionArn, ep["TaskDefinitionArn"])
			assert.Equal(t, tt.launchType, ep["LaunchType"])
		})
	}
}

// --- ECS RunTaskParameters tests ---

// TestECS_NetworkConfiguration verifies ECS network config round-trips through HTTP.
func TestECS_NetworkConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		assignPublicIP string
		subnets        []any
		securityGroups []any
	}{
		{
			name:           "fargate_with_public_ip",
			subnets:        []any{"subnet-aaa", "subnet-bbb"},
			securityGroups: []any{"sg-111", "sg-222"},
			assignPublicIP: "ENABLED",
		},
		{
			name:           "ec2_no_public_ip",
			subnets:        []any{"subnet-ccc"},
			securityGroups: []any{"sg-333"},
			assignPublicIP: "DISABLED",
		},
		{
			name:           "single_subnet_no_sg",
			subnets:        []any{"subnet-ddd"},
			securityGroups: nil,
			assignPublicIP: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2ECSTarget,
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						"LaunchType":        "FARGATE",
						"NetworkConfiguration": map[string]any{
							"AwsvpcConfiguration": map[string]any{
								"Subnets":        tt.subnets,
								"SecurityGroups": tt.securityGroups,
								"AssignPublicIp": tt.assignPublicIP,
							},
						},
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp, ok := resp["TargetParameters"].(map[string]any)
			require.True(t, ok, "TargetParameters missing")
			ecs, ok := tp["EcsTaskParameters"].(map[string]any)
			require.True(t, ok, "EcsTaskParameters missing")
			nc, ok := ecs["NetworkConfiguration"].(map[string]any)
			require.True(t, ok, "NetworkConfiguration missing")
			vpc, ok := nc["AwsvpcConfiguration"].(map[string]any)
			require.True(t, ok, "AwsvpcConfiguration missing")

			if len(tt.subnets) > 0 {
				got, subnetOK := vpc["Subnets"].([]any)
				require.True(t, subnetOK, "Subnets should be array")
				assert.Len(t, got, len(tt.subnets))
				assert.Equal(t, tt.subnets[0], got[0])
			}
			if tt.assignPublicIP != "" {
				assert.Equal(t, tt.assignPublicIP, vpc["AssignPublicIp"])
			}
		})
	}
}

// TestECS_CapacityProviderStrategy verifies capacity provider strategy round-trips.
func TestECS_CapacityProviderStrategy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		providers []map[string]any
		wantLen   int
	}{
		{
			name: "single_fargate",
			providers: []map[string]any{
				{"CapacityProvider": "FARGATE", "Weight": 1, "Base": 0},
			},
			wantLen: 1,
		},
		{
			name: "fargate_spot_fallback",
			providers: []map[string]any{
				{"CapacityProvider": "FARGATE", "Weight": 1, "Base": 1},
				{"CapacityProvider": "FARGATE_SPOT", "Weight": 3, "Base": 0},
			},
			wantLen: 2,
		},
		{
			name: "custom_capacity_provider",
			providers: []map[string]any{
				{"CapacityProvider": "my-cp", "Weight": 2, "Base": 0},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2ECSTarget,
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn":        "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						"CapacityProviderStrategy": tt.providers,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			ecs := tp["EcsTaskParameters"].(map[string]any)
			cps, ok := ecs["CapacityProviderStrategy"].([]any)
			require.True(t, ok, "CapacityProviderStrategy should be array")
			assert.Len(t, cps, tt.wantLen)

			first := cps[0].(map[string]any)
			assert.Equal(t, tt.providers[0]["CapacityProvider"], first["CapacityProvider"])
		})
	}
}

// TestECS_PlacementConstraints verifies ECS placement constraints persist.
func TestECS_PlacementConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		constraints []map[string]any
		wantLen     int
	}{
		{
			name: "distinct_instance",
			constraints: []map[string]any{
				{"Type": "distinctInstance"},
			},
			wantLen: 1,
		},
		{
			name: "member_of_with_expression",
			constraints: []map[string]any{
				{"Type": "memberOf", "Expression": "attribute:ecs.instance-type =~ t3.*"},
			},
			wantLen: 1,
		},
		{
			name: "multiple_constraints",
			constraints: []map[string]any{
				{"Type": "distinctInstance"},
				{"Type": "memberOf", "Expression": "attribute:ecs.az =~ us-east-1*"},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2ECSTarget,
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn":    "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						"PlacementConstraints": tt.constraints,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			ecs := tp["EcsTaskParameters"].(map[string]any)
			pc, ok := ecs["PlacementConstraints"].([]any)
			require.True(t, ok, "PlacementConstraints should be array")
			assert.Len(t, pc, tt.wantLen)

			first := pc[0].(map[string]any)
			assert.Equal(t, tt.constraints[0]["Type"], first["Type"])
		})
	}
}

// TestECS_PlacementStrategy verifies ECS placement strategy persists.
func TestECS_PlacementStrategy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantType  string
		wantField string
		strategy  []map[string]any
		wantLen   int
	}{
		{
			name:      "spread_by_az",
			strategy:  []map[string]any{{"Type": "spread", "Field": "attribute:ecs.availability-zone"}},
			wantLen:   1,
			wantType:  "spread",
			wantField: "attribute:ecs.availability-zone",
		},
		{
			name:      "binpack_memory",
			strategy:  []map[string]any{{"Type": "binpack", "Field": "memory"}},
			wantLen:   1,
			wantType:  "binpack",
			wantField: "memory",
		},
		{
			name: "random_strategy",
			strategy: []map[string]any{
				{"Type": "random"},
			},
			wantLen:  1,
			wantType: "random",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2ECSTarget,
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						"PlacementStrategy": tt.strategy,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			ecs := tp["EcsTaskParameters"].(map[string]any)
			ps, ok := ecs["PlacementStrategy"].([]any)
			require.True(t, ok, "PlacementStrategy should be array")
			assert.Len(t, ps, tt.wantLen)

			first := ps[0].(map[string]any)
			assert.Equal(t, tt.wantType, first["Type"])
			if tt.wantField != "" {
				assert.Equal(t, tt.wantField, first["Field"])
			}
		})
	}
}

// TestECS_Overrides verifies ECS task override fields persist.
func TestECS_Overrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		taskRoleArn      string
		executionRoleArn string
		cpu              string
		memory           string
	}{
		{
			name:        "role_override",
			taskRoleArn: "arn:aws:iam::123456789012:role/task-role",
		},
		{
			name:             "execution_role_override",
			executionRoleArn: "arn:aws:iam::123456789012:role/exec-role",
		},
		{
			name:   "cpu_memory_override",
			cpu:    "512",
			memory: "1024",
		},
		{
			name:             "full_override",
			taskRoleArn:      "arn:aws:iam::123456789012:role/task-role",
			executionRoleArn: "arn:aws:iam::123456789012:role/exec-role",
			cpu:              "1024",
			memory:           "2048",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			overrides := map[string]any{}
			if tt.taskRoleArn != "" {
				overrides["TaskRoleArn"] = tt.taskRoleArn
			}
			if tt.executionRoleArn != "" {
				overrides["ExecutionRoleArn"] = tt.executionRoleArn
			}
			if tt.cpu != "" {
				overrides["Cpu"] = tt.cpu
			}
			if tt.memory != "" {
				overrides["Memory"] = tt.memory
			}

			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2ECSTarget,
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						"Overrides":         overrides,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			ecs := tp["EcsTaskParameters"].(map[string]any)
			ov, ok := ecs["Overrides"].(map[string]any)
			require.True(t, ok, "Overrides should be object")

			if tt.taskRoleArn != "" {
				assert.Equal(t, tt.taskRoleArn, ov["TaskRoleArn"])
			}
			if tt.executionRoleArn != "" {
				assert.Equal(t, tt.executionRoleArn, ov["ExecutionRoleArn"])
			}
			if tt.cpu != "" {
				assert.Equal(t, tt.cpu, ov["Cpu"])
			}
			if tt.memory != "" {
				assert.Equal(t, tt.memory, ov["Memory"])
			}
		})
	}
}

// TestECS_ContainerOverrides_RoundTrip verifies EcsTaskOverride.ContainerOverrides
// round-trips through HTTP, including its lowercase-keyed leaf shapes
// (Environment's name/value, EnvironmentFiles/ResourceRequirements' type/value --
// verified against serializers.go/deserializers.go, which use ECS's own RunTask
// override casing here rather than Pipes' usual PascalCase).
func TestECS_ContainerOverrides_RoundTrip(t *testing.T) {
	t.Parallel()

	h := b2Handler(t)
	body := map[string]any{
		"RoleArn": "arn:aws:iam::123456789012:role/r",
		"Source":  b2SQSSource,
		"Target":  b2ECSTarget,
		"TargetParameters": map[string]any{
			"EcsTaskParameters": map[string]any{
				"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				"Overrides": map[string]any{
					"ContainerOverrides": []map[string]any{
						{
							"Name":              "app",
							"Command":           []string{"echo", "hi"},
							"Cpu":               256,
							"Memory":            512,
							"MemoryReservation": 256,
							"Environment": []map[string]any{
								{"name": "KEY", "value": "VALUE"},
							},
							"EnvironmentFiles": []map[string]any{
								{"type": "s3", "value": "arn:aws:s3:::bucket/env"},
							},
							"ResourceRequirements": []map[string]any{
								{"type": "GPU", "value": "1"},
							},
						},
					},
				},
			},
		},
	}
	resp := b2Create(t, h, "ecs-container-overrides", body)

	tp := resp["TargetParameters"].(map[string]any)
	ecs := tp["EcsTaskParameters"].(map[string]any)
	ov := ecs["Overrides"].(map[string]any)
	cos, ok := ov["ContainerOverrides"].([]any)
	require.True(t, ok, "ContainerOverrides should be array")
	require.Len(t, cos, 1)
	co := cos[0].(map[string]any)

	assert.Equal(t, "app", co["Name"])
	assert.InEpsilon(t, float64(256), co["Cpu"], 0.01)
	assert.InEpsilon(t, float64(512), co["Memory"], 0.01)
	assert.InEpsilon(t, float64(256), co["MemoryReservation"], 0.01)

	env, ok := co["Environment"].([]any)
	require.True(t, ok, "Environment should be array")
	require.Len(t, env, 1)
	envEntry := env[0].(map[string]any)
	assert.Equal(t, "KEY", envEntry["name"], "Environment entries key lowercase (SDK ECS casing)")
	assert.Equal(t, "VALUE", envEntry["value"])

	files, ok := co["EnvironmentFiles"].([]any)
	require.True(t, ok, "EnvironmentFiles should be array")
	require.Len(t, files, 1)
	fileEntry := files[0].(map[string]any)
	assert.Equal(t, "s3", fileEntry["type"], "EnvironmentFiles keys lowercase (SDK ECS casing)")
	assert.Equal(t, "arn:aws:s3:::bucket/env", fileEntry["value"])

	rrs, ok := co["ResourceRequirements"].([]any)
	require.True(t, ok, "ResourceRequirements should be array")
	require.Len(t, rrs, 1)
	rrEntry := rrs[0].(map[string]any)
	assert.Equal(t, "GPU", rrEntry["type"], "ResourceRequirements keys lowercase (SDK ECS casing)")
	assert.Equal(t, "1", rrEntry["value"])
}

// TestECS_EphemeralStorage_RoundTrip verifies EcsTaskOverride.EphemeralStorage
// round-trips, including its lowercase "sizeInGiB" wire key (deserializers.go).
func TestECS_EphemeralStorage_RoundTrip(t *testing.T) {
	t.Parallel()

	h := b2Handler(t)
	body := map[string]any{
		"RoleArn": "arn:aws:iam::123456789012:role/r",
		"Source":  b2SQSSource,
		"Target":  b2ECSTarget,
		"TargetParameters": map[string]any{
			"EcsTaskParameters": map[string]any{
				"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				"Overrides": map[string]any{
					"EphemeralStorage": map[string]any{"sizeInGiB": 42},
				},
			},
		},
	}
	resp := b2Create(t, h, "ecs-ephemeral-storage", body)

	sz := nestedFloat(t, resp,
		"TargetParameters", "EcsTaskParameters", "Overrides", "EphemeralStorage", "sizeInGiB")
	assert.InEpsilon(t, float64(42), sz, 0.01)
}

// TestECS_InferenceAcceleratorOverrides_RoundTrip verifies
// EcsTaskOverride.InferenceAcceleratorOverrides round-trips, including its
// lowercase deviceName/deviceType wire keys (serializers.go).
func TestECS_InferenceAcceleratorOverrides_RoundTrip(t *testing.T) {
	t.Parallel()

	h := b2Handler(t)
	body := map[string]any{
		"RoleArn": "arn:aws:iam::123456789012:role/r",
		"Source":  b2SQSSource,
		"Target":  b2ECSTarget,
		"TargetParameters": map[string]any{
			"EcsTaskParameters": map[string]any{
				"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				"Overrides": map[string]any{
					"InferenceAcceleratorOverrides": []map[string]any{
						{"deviceName": "acc1", "deviceType": "eia1.medium"},
					},
				},
			},
		},
	}
	resp := b2Create(t, h, "ecs-inference-accelerator", body)

	tp := resp["TargetParameters"].(map[string]any)
	ecs := tp["EcsTaskParameters"].(map[string]any)
	ov := ecs["Overrides"].(map[string]any)
	iaos, ok := ov["InferenceAcceleratorOverrides"].([]any)
	require.True(t, ok, "InferenceAcceleratorOverrides should be array")
	require.Len(t, iaos, 1)
	iao := iaos[0].(map[string]any)
	assert.Equal(t, "acc1", iao["deviceName"])
	assert.Equal(t, "eia1.medium", iao["deviceType"])
}

// TestECS_PropagateTagsReferenceIdTags_RoundTrip verifies
// ECSTaskTargetParameters' PropagateTags/ReferenceId/Tags round-trip.
func TestECS_PropagateTagsReferenceIdTags_RoundTrip(t *testing.T) {
	t.Parallel()

	h := b2Handler(t)
	body := map[string]any{
		"RoleArn": "arn:aws:iam::123456789012:role/r",
		"Source":  b2SQSSource,
		"Target":  b2ECSTarget,
		"TargetParameters": map[string]any{
			"EcsTaskParameters": map[string]any{
				"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				"PropagateTags":     "TASK_DEFINITION",
				"ReferenceId":       "ref-123",
				"Tags": []map[string]any{
					{"Key": "env", "Value": "prod"},
				},
			},
		},
	}
	resp := b2Create(t, h, "ecs-propagate-tags", body)

	tp := resp["TargetParameters"].(map[string]any)
	ecs := tp["EcsTaskParameters"].(map[string]any)
	assert.Equal(t, "TASK_DEFINITION", ecs["PropagateTags"])
	assert.Equal(t, "ref-123", ecs["ReferenceId"])
	tags, ok := ecs["Tags"].([]any)
	require.True(t, ok, "Tags should be array")
	require.Len(t, tags, 1)
	tag := tags[0].(map[string]any)
	assert.Equal(t, "env", tag["Key"])
	assert.Equal(t, "prod", tag["Value"])
}

// TestECS_OverridesFull_BackendAPI verifies the full restored EcsTaskOverride
// shape survives a backend Create/Get round trip, using typed Go structs.
func TestECS_OverridesFull_BackendAPI(t *testing.T) {
	t.Parallel()

	b := b2Backend()
	_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
		RoleARN: "arn:aws:iam::123456789012:role/r",
		Name:    "ecs-overrides-full",
		Source:  b2SQSSource,
		Target:  b2ECSTarget,
		TargetParameters: &pipes.TargetParameters{
			EcsTaskParameters: &pipes.ECSTaskTargetParameters{
				TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				PropagateTags:     "TASK_DEFINITION",
				ReferenceID:       "ref-full",
				Tags:              []pipes.Tag{{Key: "env", Value: "prod"}},
				Overrides: &pipes.EcsTaskOverride{
					TaskRoleArn: "arn:aws:iam::123456789012:role/task-role",
					EphemeralStorage: &pipes.EcsEphemeralStorage{
						SizeInGiB: 42,
					},
					ContainerOverrides: []pipes.EcsContainerOverride{
						{
							Name:              "app",
							CPU:               256,
							Memory:            512,
							MemoryReservation: 256,
							Command:           []string{"echo", "hi"},
							Environment: []pipes.EcsEnvironmentVariable{
								{Name: "KEY", Value: "VALUE"},
							},
							EnvironmentFiles: []pipes.EcsEnvironmentFile{
								{Type: "s3", Value: "arn:aws:s3:::bucket/env"},
							},
							ResourceRequirements: []pipes.EcsResourceRequirement{
								{Type: "GPU", Value: "1"},
							},
						},
					},
					InferenceAcceleratorOverrides: []pipes.EcsInferenceAcceleratorOverride{
						{DeviceName: "acc1", DeviceType: "eia1.medium"},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	p, err := b.GetPipe(context.Background(), "ecs-overrides-full")
	require.NoError(t, err)

	ecs := p.TargetParameters.EcsTaskParameters
	assert.Equal(t, "TASK_DEFINITION", ecs.PropagateTags)
	assert.Equal(t, "ref-full", ecs.ReferenceID)
	require.Len(t, ecs.Tags, 1)
	assert.Equal(t, "env", ecs.Tags[0].Key)

	require.NotNil(t, ecs.Overrides)
	require.NotNil(t, ecs.Overrides.EphemeralStorage)
	assert.Equal(t, 42, ecs.Overrides.EphemeralStorage.SizeInGiB)
	require.Len(t, ecs.Overrides.ContainerOverrides, 1)
	co := ecs.Overrides.ContainerOverrides[0]
	assert.Equal(t, "app", co.Name)
	assert.Equal(t, 256, co.CPU)
	require.Len(t, co.Environment, 1)
	assert.Equal(t, "VALUE", co.Environment[0].Value)
	require.Len(t, co.EnvironmentFiles, 1)
	assert.Equal(t, "s3", co.EnvironmentFiles[0].Type)
	require.Len(t, co.ResourceRequirements, 1)
	assert.Equal(t, "GPU", co.ResourceRequirements[0].Type)
	require.Len(t, ecs.Overrides.InferenceAcceleratorOverrides, 1)
	assert.Equal(t, "acc1", ecs.Overrides.InferenceAcceleratorOverrides[0].DeviceName)
}

// TestClone_ECSOverridesIsolation verifies cloneEcsTaskOverride isolates
// ContainerOverrides' nested slices and EphemeralStorage from the stored copy.
func TestClone_ECSOverridesIsolation(t *testing.T) {
	t.Parallel()

	b := b2Backend()
	_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
		RoleARN: "arn:aws:iam::123456789012:role/r",
		Name:    "ecs-overrides-clone",
		Source:  b2SQSSource,
		Target:  b2ECSTarget,
		TargetParameters: &pipes.TargetParameters{
			EcsTaskParameters: &pipes.ECSTaskTargetParameters{
				TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				Tags:              []pipes.Tag{{Key: "env", Value: "prod"}},
				Overrides: &pipes.EcsTaskOverride{
					EphemeralStorage: &pipes.EcsEphemeralStorage{SizeInGiB: 21},
					ContainerOverrides: []pipes.EcsContainerOverride{
						{
							Name:    "app",
							Command: []string{"echo", "hi"},
							Environment: []pipes.EcsEnvironmentVariable{
								{Name: "K", Value: "V"},
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	p1, err := b.GetPipe(context.Background(), "ecs-overrides-clone")
	require.NoError(t, err)

	p1.TargetParameters.EcsTaskParameters.Overrides.EphemeralStorage.SizeInGiB = 999
	p1.TargetParameters.EcsTaskParameters.Overrides.ContainerOverrides[0].Command[0] = "mutated"
	p1.TargetParameters.EcsTaskParameters.Overrides.ContainerOverrides[0].Environment[0].Value = "mutated"
	p1.TargetParameters.EcsTaskParameters.Tags[0].Value = "mutated"

	p2, err := b.GetPipe(context.Background(), "ecs-overrides-clone")
	require.NoError(t, err)

	assert.Equal(t, 21, p2.TargetParameters.EcsTaskParameters.Overrides.EphemeralStorage.SizeInGiB)
	assert.Equal(t, "echo", p2.TargetParameters.EcsTaskParameters.Overrides.ContainerOverrides[0].Command[0])
	assert.Equal(t, "V", p2.TargetParameters.EcsTaskParameters.Overrides.ContainerOverrides[0].Environment[0].Value)
	assert.Equal(t, "prod", p2.TargetParameters.EcsTaskParameters.Tags[0].Value)
}

// TestBatch_ResourceRequirements_RoundTrip verifies
// BatchContainerOverrides.ResourceRequirements round-trips through HTTP.
func TestBatch_ResourceRequirements_RoundTrip(t *testing.T) {
	t.Parallel()

	h := b2Handler(t)
	body := map[string]any{
		"RoleArn": "arn:aws:iam::123456789012:role/r",
		"Source":  b2SQSSource,
		"Target":  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
		"TargetParameters": map[string]any{
			"BatchJobParameters": map[string]any{
				"JobDefinition": "arn:aws:batch:us-east-1:123456789012:job-definition/jd:1",
				"JobName":       "my-job",
				"ContainerOverrides": map[string]any{
					"ResourceRequirements": []map[string]any{
						{"Type": "VCPU", "Value": "2"},
						{"Type": "MEMORY", "Value": "4096"},
					},
				},
			},
		},
	}
	resp := b2Create(t, h, "batch-resource-requirements", body)

	tp := resp["TargetParameters"].(map[string]any)
	batch := tp["BatchJobParameters"].(map[string]any)
	co := batch["ContainerOverrides"].(map[string]any)
	rrs, ok := co["ResourceRequirements"].([]any)
	require.True(t, ok, "ResourceRequirements should be array")
	require.Len(t, rrs, 2)
	first := rrs[0].(map[string]any)
	assert.Equal(t, "VCPU", first["Type"])
	assert.Equal(t, "2", first["Value"])
}

// TestClone_BatchResourceRequirementsIsolation verifies
// cloneBatchJobParameters isolates ResourceRequirements from the stored copy.
func TestClone_BatchResourceRequirementsIsolation(t *testing.T) {
	t.Parallel()

	b := b2Backend()
	_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
		RoleARN: "arn:aws:iam::123456789012:role/r",
		Name:    "batch-rr-clone",
		Source:  b2SQSSource,
		Target:  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
		TargetParameters: &pipes.TargetParameters{
			BatchJobParameters: &pipes.BatchJobTargetParameters{
				JobDefinition: "jd",
				JobName:       "job",
				ContainerOverrides: &pipes.BatchContainerOverrides{
					ResourceRequirements: []pipes.BatchResourceRequirement{
						{Type: "VCPU", Value: "1"},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	p1, err := b.GetPipe(context.Background(), "batch-rr-clone")
	require.NoError(t, err)

	p1.TargetParameters.BatchJobParameters.ContainerOverrides.ResourceRequirements[0].Value = "mutated"

	p2, err := b.GetPipe(context.Background(), "batch-rr-clone")
	require.NoError(t, err)
	assert.Equal(t, "1", p2.TargetParameters.BatchJobParameters.ContainerOverrides.ResourceRequirements[0].Value)
}

// TestECS_ExtraFields verifies Group, PlatformVersion, EnableECSManagedTags, EnableExecuteCommand.
func TestECS_ExtraFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		group                string
		platformVersion      string
		enableECSManagedTags bool
		enableExecuteCommand bool
		taskCount            float64
	}{
		{
			name:            "group_and_platform",
			group:           "service:my-svc",
			platformVersion: "1.4.0",
			taskCount:       2,
		},
		{
			name:                 "execute_command_enabled",
			enableExecuteCommand: true,
		},
		{
			name:                 "ecs_managed_tags",
			enableECSManagedTags: true,
		},
		{
			name:                 "all_flags",
			group:                "family:td",
			platformVersion:      "LATEST",
			enableECSManagedTags: true,
			enableExecuteCommand: true,
			taskCount:            3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			ecsParams := map[string]any{
				"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
			}
			if tt.group != "" {
				ecsParams["Group"] = tt.group
			}
			if tt.platformVersion != "" {
				ecsParams["PlatformVersion"] = tt.platformVersion
			}
			if tt.enableECSManagedTags {
				ecsParams["EnableECSManagedTags"] = true
			}
			if tt.enableExecuteCommand {
				ecsParams["EnableExecuteCommand"] = true
			}
			if tt.taskCount > 0 {
				ecsParams["TaskCount"] = tt.taskCount
			}

			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2ECSTarget,
				"TargetParameters": map[string]any{
					"EcsTaskParameters": ecsParams,
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			ecs := tp["EcsTaskParameters"].(map[string]any)

			if tt.group != "" {
				assert.Equal(t, tt.group, ecs["Group"])
			}
			if tt.platformVersion != "" {
				assert.Equal(t, tt.platformVersion, ecs["PlatformVersion"])
			}
			if tt.enableECSManagedTags {
				assert.Equal(t, true, ecs["EnableECSManagedTags"])
			}
			if tt.enableExecuteCommand {
				assert.Equal(t, true, ecs["EnableExecuteCommand"])
			}
			if tt.taskCount > 0 {
				assert.InEpsilon(t, tt.taskCount, ecs["TaskCount"], 0.01)
			}
		})
	}
}

// TestECS_FullParams verifies all ECS params combined.
func TestECS_FullParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantGroup string
	}{
		{name: "ecs_full_a", wantGroup: "service:alpha"},
		{name: "ecs_full_b", wantGroup: "service:beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  b2SQSSource,
				Target:  b2ECSTarget,
				TargetParameters: &pipes.TargetParameters{
					EcsTaskParameters: &pipes.ECSTaskTargetParameters{
						TaskDefinitionArn:    "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						LaunchType:           "FARGATE",
						Group:                tt.wantGroup,
						PlatformVersion:      "1.4.0",
						TaskCount:            2,
						EnableECSManagedTags: true,
						EnableExecuteCommand: true,
						NetworkConfiguration: &pipes.NetworkConfiguration{
							AwsvpcConfiguration: &pipes.AwsVpcConfiguration{
								Subnets:        []string{"subnet-aaa", "subnet-bbb"},
								SecurityGroups: []string{"sg-111"},
								AssignPublicIP: "ENABLED",
							},
						},
						CapacityProviderStrategy: []pipes.CapacityProviderStrategyItem{
							{CapacityProvider: "FARGATE", Weight: 1, Base: 1},
							{CapacityProvider: "FARGATE_SPOT", Weight: 3, Base: 0},
						},
						PlacementConstraints: []pipes.PlacementConstraint{
							{Type: "memberOf", Expression: "attribute:ecs.az =~ us-east-1*"},
						},
						PlacementStrategy: []pipes.PlacementStrategy{
							{Type: "spread", Field: "attribute:ecs.availability-zone"},
						},
						Overrides: &pipes.EcsTaskOverride{
							TaskRoleArn: "arn:aws:iam::123456789012:role/task-role",
							CPU:         "512",
							Memory:      "1024",
						},
					},
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)

			ecs := p.TargetParameters.EcsTaskParameters
			assert.Equal(t, tt.wantGroup, ecs.Group)
			assert.Equal(t, "1.4.0", ecs.PlatformVersion)
			assert.Equal(t, 2, ecs.TaskCount)
			assert.True(t, ecs.EnableECSManagedTags)
			assert.True(t, ecs.EnableExecuteCommand)
			require.NotNil(t, ecs.NetworkConfiguration)
			assert.Equal(t, "ENABLED", ecs.NetworkConfiguration.AwsvpcConfiguration.AssignPublicIP)
			assert.Len(t, ecs.CapacityProviderStrategy, 2)
			assert.Equal(t, "FARGATE_SPOT", ecs.CapacityProviderStrategy[1].CapacityProvider)
			assert.Len(t, ecs.PlacementConstraints, 1)
			assert.Equal(t, "memberOf", ecs.PlacementConstraints[0].Type)
			assert.Len(t, ecs.PlacementStrategy, 1)
			assert.Equal(t, "spread", ecs.PlacementStrategy[0].Type)
			require.NotNil(t, ecs.Overrides)
			assert.Equal(t, "512", ecs.Overrides.CPU)
		})
	}
}

// --- Batch target tests ---

// TestBatch_DependsOn verifies Batch DependsOn persists through HTTP.
func TestBatch_DependsOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantType  string
		dependsOn []map[string]any
		wantLen   int
	}{
		{
			name:      "sequential_dependency",
			dependsOn: []map[string]any{{"JobId": "job-aaa", "Type": "SEQUENTIAL"}},
			wantLen:   1,
			wantType:  "SEQUENTIAL",
		},
		{
			name:      "n_to_n_dependency",
			dependsOn: []map[string]any{{"JobId": "job-bbb", "Type": "N_TO_N"}},
			wantLen:   1,
			wantType:  "N_TO_N",
		},
		{
			name: "multiple_dependencies",
			dependsOn: []map[string]any{
				{"JobId": "job-aaa", "Type": "SEQUENTIAL"},
				{"JobId": "job-bbb", "Type": "N_TO_N"},
			},
			wantLen:  2,
			wantType: "SEQUENTIAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				"TargetParameters": map[string]any{
					"BatchJobParameters": map[string]any{
						"JobDefinition": "arn:aws:batch:us-east-1:123456789012:job-definition/jd:1",
						"JobName":       "my-job",
						"DependsOn":     tt.dependsOn,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			batch := tp["BatchJobParameters"].(map[string]any)
			deps, ok := batch["DependsOn"].([]any)
			require.True(t, ok, "DependsOn should be array")
			assert.Len(t, deps, tt.wantLen)

			first := deps[0].(map[string]any)
			assert.Equal(t, tt.wantType, first["Type"])
		})
	}
}

// TestBatch_ContainerOverrides verifies Batch container overrides.
func TestBatch_ContainerOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		instanceType string
		command      []any
		environment  []any
	}{
		{
			name:    "command_override",
			command: []any{"echo", "hello"},
		},
		{
			name:         "instance_type_override",
			instanceType: "c5.xlarge",
		},
		{
			name: "env_override",
			environment: []any{
				map[string]any{"Name": "KEY", "Value": "VALUE"},
				map[string]any{"Name": "ENV", "Value": "prod"},
			},
		},
		{
			name:         "full_override",
			command:      []any{"bash", "-c", "echo hi"},
			instanceType: "m5.large",
			environment: []any{
				map[string]any{"Name": "LOG_LEVEL", "Value": "DEBUG"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			co := map[string]any{}
			if tt.command != nil {
				co["Command"] = tt.command
			}
			if tt.instanceType != "" {
				co["InstanceType"] = tt.instanceType
			}
			if tt.environment != nil {
				co["Environment"] = tt.environment
			}

			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				"TargetParameters": map[string]any{
					"BatchJobParameters": map[string]any{
						"JobDefinition":      "arn:aws:batch:us-east-1:123456789012:job-definition/jd:1",
						"JobName":            "my-job",
						"ContainerOverrides": co,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			batch := tp["BatchJobParameters"].(map[string]any)
			got, ok := batch["ContainerOverrides"].(map[string]any)
			require.True(t, ok, "ContainerOverrides should be object")

			if tt.instanceType != "" {
				assert.Equal(t, tt.instanceType, got["InstanceType"])
			}
			if tt.command != nil {
				cmd, cmdOK := got["Command"].([]any)
				require.True(t, cmdOK)
				assert.Len(t, cmd, len(tt.command))
			}
			if tt.environment != nil {
				env, envOK := got["Environment"].([]any)
				require.True(t, envOK, "Environment should be a list, not a bare map")
				assert.Len(t, env, len(tt.environment))
			}
		})
	}
}

// TestBatch_ArrayProperties verifies BatchArrayProperties persists.
func TestBatch_ArrayProperties(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		size   float64
		wantSz float64
	}{
		{name: "size_2", size: 2, wantSz: 2},
		{name: "size_100", size: 100, wantSz: 100},
		{name: "size_10000", size: 10000, wantSz: 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				"TargetParameters": map[string]any{
					"BatchJobParameters": map[string]any{
						"JobDefinition":   "arn:aws:batch:us-east-1:123456789012:job-definition/jd:1",
						"JobName":         "my-job",
						"ArrayProperties": map[string]any{"Size": tt.size},
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sz := nestedFloat(t, resp,
				"TargetParameters", "BatchJobParameters", "ArrayProperties", "Size")
			assert.InEpsilon(t, tt.wantSz, sz, 0.01)
		})
	}
}

// TestBatch_RetryStrategy verifies BatchRetryStrategy persists.
func TestBatch_RetryStrategy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		attempts     float64
		wantAttempts float64
	}{
		{name: "retry_once", attempts: 1, wantAttempts: 1},
		{name: "retry_three", attempts: 3, wantAttempts: 3},
		{name: "retry_ten", attempts: 10, wantAttempts: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				"TargetParameters": map[string]any{
					"BatchJobParameters": map[string]any{
						"JobDefinition": "arn:aws:batch:us-east-1:123456789012:job-definition/jd:1",
						"JobName":       "my-job",
						"RetryStrategy": map[string]any{"Attempts": tt.attempts},
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			got := nestedFloat(t, resp,
				"TargetParameters", "BatchJobParameters", "RetryStrategy", "Attempts")
			assert.InEpsilon(t, tt.wantAttempts, got, 0.01)
		})
	}
}

// TestBatch_FullParams verifies all Batch params combined via backend API.
func TestBatch_FullParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobName string
	}{
		{name: "batch_full_a", jobName: "job-alpha"},
		{name: "batch_full_b", jobName: "job-beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  b2SQSSource,
				Target:  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				TargetParameters: &pipes.TargetParameters{
					BatchJobParameters: &pipes.BatchJobTargetParameters{
						JobDefinition:   "arn:aws:batch:us-east-1:123456789012:job-definition/jd:1",
						JobName:         tt.jobName,
						ArrayProperties: &pipes.BatchArrayProperties{Size: 5},
						RetryStrategy:   &pipes.BatchRetryStrategy{Attempts: 3},
						Parameters:      map[string]string{"k1": "v1", "k2": "v2"},
						DependsOn: []pipes.BatchJobDependency{
							{JobID: "job-parent", Type: "SEQUENTIAL"},
						},
						ContainerOverrides: &pipes.BatchContainerOverrides{
							Command:      []string{"bash", "-c", "echo hi"},
							InstanceType: "m5.large",
							Environment:  []pipes.BatchEnvironmentVariable{{Name: "ENV", Value: "test"}},
						},
					},
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)

			batch := p.TargetParameters.BatchJobParameters
			assert.Equal(t, tt.jobName, batch.JobName)
			assert.Equal(t, 5, batch.ArrayProperties.Size)
			assert.Equal(t, 3, batch.RetryStrategy.Attempts)
			assert.Equal(t, "v1", batch.Parameters["k1"])
			require.Len(t, batch.DependsOn, 1)
			assert.Equal(t, "SEQUENTIAL", batch.DependsOn[0].Type)
			assert.Equal(t, "job-parent", batch.DependsOn[0].JobID)
			require.NotNil(t, batch.ContainerOverrides)
			assert.Equal(t, "m5.large", batch.ContainerOverrides.InstanceType)
			assert.Equal(t, []string{"bash", "-c", "echo hi"}, batch.ContainerOverrides.Command)
		})
	}
}

// --- Immutability / clone isolation tests ---

// TestClone_ECSNetworkIsolation verifies ECS network config clone isolates slices.
func TestClone_ECSNetworkIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "ecs_clone_subnets"},
		{name: "ecs_clone_secgroups"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  b2SQSSource,
				Target:  b2ECSTarget,
				TargetParameters: &pipes.TargetParameters{
					EcsTaskParameters: &pipes.ECSTaskTargetParameters{
						TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						NetworkConfiguration: &pipes.NetworkConfiguration{
							AwsvpcConfiguration: &pipes.AwsVpcConfiguration{
								Subnets:        []string{"subnet-aaa"},
								SecurityGroups: []string{"sg-111"},
							},
						},
						CapacityProviderStrategy: []pipes.CapacityProviderStrategyItem{
							{CapacityProvider: "FARGATE", Weight: 1},
						},
						PlacementConstraints: []pipes.PlacementConstraint{
							{Type: "distinctInstance"},
						},
						PlacementStrategy: []pipes.PlacementStrategy{
							{Type: "spread", Field: "az"},
						},
					},
				},
			})
			require.NoError(t, err)

			p1, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)

			p1.TargetParameters.EcsTaskParameters.NetworkConfiguration.
				AwsvpcConfiguration.Subnets[0] = "mutated"
			p1.TargetParameters.EcsTaskParameters.CapacityProviderStrategy[0].CapacityProvider = "mutated"
			p1.TargetParameters.EcsTaskParameters.PlacementConstraints[0].Type = "mutated"
			p1.TargetParameters.EcsTaskParameters.PlacementStrategy[0].Type = "mutated"

			p2, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, "subnet-aaa",
				p2.TargetParameters.EcsTaskParameters.NetworkConfiguration.AwsvpcConfiguration.Subnets[0])
			assert.Equal(t, "FARGATE",
				p2.TargetParameters.EcsTaskParameters.CapacityProviderStrategy[0].CapacityProvider)
			assert.Equal(t, "distinctInstance",
				p2.TargetParameters.EcsTaskParameters.PlacementConstraints[0].Type)
			assert.Equal(t, "spread",
				p2.TargetParameters.EcsTaskParameters.PlacementStrategy[0].Type)
		})
	}
}

// TestClone_BatchDependsOnIsolation verifies Batch DependsOn clone isolates slices.
func TestClone_BatchDependsOnIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "batch_depends_clone_a"},
		{name: "batch_depends_clone_b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  b2SQSSource,
				Target:  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				TargetParameters: &pipes.TargetParameters{
					BatchJobParameters: &pipes.BatchJobTargetParameters{
						JobDefinition: "jd",
						JobName:       "job",
						DependsOn: []pipes.BatchJobDependency{
							{JobID: "original-job", Type: "SEQUENTIAL"},
						},
						ContainerOverrides: &pipes.BatchContainerOverrides{
							Command:     []string{"echo", "hi"},
							Environment: []pipes.BatchEnvironmentVariable{{Name: "K", Value: "V"}},
						},
					},
				},
			})
			require.NoError(t, err)

			p1, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)

			p1.TargetParameters.BatchJobParameters.DependsOn[0].JobID = "mutated"
			p1.TargetParameters.BatchJobParameters.ContainerOverrides.Command[0] = "mutated"
			p1.TargetParameters.BatchJobParameters.ContainerOverrides.Environment[0].Value = "mutated"

			p2, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, "original-job",
				p2.TargetParameters.BatchJobParameters.DependsOn[0].JobID)
			assert.Equal(t, "echo",
				p2.TargetParameters.BatchJobParameters.ContainerOverrides.Command[0])
			assert.Equal(t, "V",
				p2.TargetParameters.BatchJobParameters.ContainerOverrides.Environment[0].Value)
		})
	}
}

// --- Update path tests for new fields ---

// TestUpdate_ECSParams verifies UpdatePipe handles new ECS fields.
func TestUpdate_ECSParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialGroup  string
		updatedGroup  string
		updatedLaunch string
	}{
		{
			name:          "update_ecs_group",
			initialGroup:  "service:initial",
			updatedGroup:  "service:updated",
			updatedLaunch: "FARGATE",
		},
		{
			name:          "update_ecs_launch_type",
			initialGroup:  "service:svc",
			updatedGroup:  "service:svc",
			updatedLaunch: "EC2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			b2Create(t, h, tt.name, map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2ECSTarget,
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						"Group":             tt.initialGroup,
					},
				},
			})

			updated := b2Update(t, h, tt.name, map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:2",
						"Group":             tt.updatedGroup,
						"LaunchType":        tt.updatedLaunch,
					},
				},
			})

			tp := updated["TargetParameters"].(map[string]any)
			ecs := tp["EcsTaskParameters"].(map[string]any)
			assert.Equal(t, tt.updatedGroup, ecs["Group"])
			assert.Equal(t, tt.updatedLaunch, ecs["LaunchType"])
		})
	}
}

// TestUpdate_BatchDependsOn verifies UpdatePipe propagates Batch DependsOn.
func TestUpdate_BatchDependsOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initialDeps []map[string]any
		updatedDeps []map[string]any
		wantLen     int
	}{
		{
			name:        "add_dependency",
			initialDeps: nil,
			updatedDeps: []map[string]any{{"JobId": "job-new", "Type": "SEQUENTIAL"}},
			wantLen:     1,
		},
		{
			name:        "change_dependency",
			initialDeps: []map[string]any{{"JobId": "job-old", "Type": "SEQUENTIAL"}},
			updatedDeps: []map[string]any{
				{"JobId": "job-a", "Type": "SEQUENTIAL"},
				{"JobId": "job-b", "Type": "N_TO_N"},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			initialBatch := map[string]any{
				"JobDefinition": "jd",
				"JobName":       "job",
			}
			if tt.initialDeps != nil {
				initialBatch["DependsOn"] = tt.initialDeps
			}
			b2Create(t, h, tt.name, map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				"TargetParameters": map[string]any{
					"BatchJobParameters": initialBatch,
				},
			})

			updated := b2Update(t, h, tt.name, map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"TargetParameters": map[string]any{
					"BatchJobParameters": map[string]any{
						"JobDefinition": "jd",
						"JobName":       "job",
						"DependsOn":     tt.updatedDeps,
					},
				},
			})

			tp := updated["TargetParameters"].(map[string]any)
			batch := tp["BatchJobParameters"].(map[string]any)
			deps, ok := batch["DependsOn"].([]any)
			require.True(t, ok)
			assert.Len(t, deps, tt.wantLen)
		})
	}
}
