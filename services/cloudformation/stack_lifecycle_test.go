package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// ---- EnableTerminationProtection -----------------------------------------------

func TestUpdateTerminationProtection_StoresValue(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"prot-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	// Enable protection.
	err = b.UpdateTerminationProtection("prot-stack", true)
	require.NoError(t, err)

	stack, err := b.DescribeStack("prot-stack")
	require.NoError(t, err)
	assert.True(t, stack.EnableTerminationProtection)

	// Disable protection.
	err = b.UpdateTerminationProtection("prot-stack", false)
	require.NoError(t, err)

	stack, err = b.DescribeStack("prot-stack")
	require.NoError(t, err)
	assert.False(t, stack.EnableTerminationProtection)
}

func TestUpdateTerminationProtection_StackNotFound(t *testing.T) {
	t.Parallel()

	b := newBackend()
	err := b.UpdateTerminationProtection("no-such-stack", true)
	require.ErrorIs(t, err, cloudformation.ErrStackNotFound)
}

// ---- DeleteStack: termination protection blocks deletion ------------------------

func TestDeleteStack_TerminationProtectionBlocks(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"protected",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	require.NoError(t, b.UpdateTerminationProtection("protected", true))

	err = b.DeleteStack(t.Context(), "protected")
	require.ErrorIs(t, err, cloudformation.ErrTerminationProtectionEnabled)

	// Stack still exists and status unchanged.
	stack, err := b.DescribeStack("protected")
	require.NoError(t, err)
	assert.Equal(t, "CREATE_COMPLETE", stack.StackStatus)
}

func TestDeleteStack_TerminationProtectionDisabledAllowsDeletion(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"unprotected",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	require.NoError(t, b.UpdateTerminationProtection("unprotected", true))
	require.NoError(t, b.UpdateTerminationProtection("unprotected", false))
	require.NoError(t, b.DeleteStack(t.Context(), "unprotected"))
}

// ---- CreateStack: StackOptions fields stored on Stack --------------------------

func TestCreateStack_CapabilitiesStored(t *testing.T) {
	t.Parallel()

	b := newBackend()
	caps := []string{"CAPABILITY_IAM", "CAPABILITY_NAMED_IAM"}
	stack, err := b.CreateStack(
		t.Context(),
		"cap-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			Capabilities: caps,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, caps, stack.Capabilities)

	// Persisted after describe.
	s2, err := b.DescribeStack("cap-stack")
	require.NoError(t, err)
	assert.Equal(t, caps, s2.Capabilities)
}

func TestCreateStack_RoleARNStored(t *testing.T) {
	t.Parallel()

	b := newBackend()
	roleARN := "arn:aws:iam::123456789012:role/MyCFNRole"
	stack, err := b.CreateStack(
		t.Context(),
		"role-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			RoleARN: roleARN,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, roleARN, stack.RoleARN)
}

func TestCreateStack_NotificationARNsStored(t *testing.T) {
	t.Parallel()

	b := newBackend()
	notifARNs := []string{"arn:aws:sns:us-east-1:123:MyTopic"}
	stack, err := b.CreateStack(
		t.Context(),
		"notif-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			NotificationARNs: notifARNs,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, notifARNs, stack.NotificationARNs)
}

func TestCreateStack_TimeoutStored(t *testing.T) {
	t.Parallel()

	b := newBackend()
	stack, err := b.CreateStack(
		t.Context(),
		"timeout-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			TimeoutInMinutes: 30,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 30, stack.TimeoutInMinutes)
}

func TestCreateStack_DisableRollbackStored(t *testing.T) {
	t.Parallel()

	b := newBackend()
	stack, err := b.CreateStack(
		t.Context(),
		"noroll-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			DisableRollback: true,
		},
	)
	require.NoError(t, err)
	assert.True(t, stack.DisableRollback)
}

func TestCreateStack_RollbackConfigurationStored(t *testing.T) {
	t.Parallel()

	b := newBackend()
	rc := &cloudformation.RollbackConfiguration{
		MonitoringTimeInMinutes: 10,
		RollbackTriggers: []cloudformation.RollbackTrigger{
			{ARN: "arn:aws:cloudwatch:us-east-1:123:alarm/MyAlarm", Type: "AWS::CloudWatch::Alarm"},
		},
	}
	stack, err := b.CreateStack(
		t.Context(),
		"rc-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			RollbackConfiguration: rc,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, stack.RollbackConfiguration)
	assert.Equal(t, 10, stack.RollbackConfiguration.MonitoringTimeInMinutes)
	assert.Len(t, stack.RollbackConfiguration.RollbackTriggers, 1)
	assert.Equal(
		t,
		"arn:aws:cloudwatch:us-east-1:123:alarm/MyAlarm",
		stack.RollbackConfiguration.RollbackTriggers[0].ARN,
	)
}

// ---- OnFailure=DELETE behavior --------------------------------------------------

func TestCreateStack_OnFailureDelete_RemovesStack(t *testing.T) {
	t.Parallel()

	b := newBackend()
	// Template with invalid import to trigger failure.
	failTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Bucket": {
				"Type": "AWS::S3::Bucket",
				"Properties": {"BucketName": {"Fn::ImportValue": "nonexistent-export"}}
			}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(),
		"del-on-fail",
		failTemplate,
		nil,
		cloudformation.StackOptions{
			OnFailure: "DELETE",
		},
	)
	require.NoError(t, err)

	// Stack should be in DELETE_COMPLETE state.
	assert.Equal(t, "DELETE_COMPLETE", stack.StackStatus)
}

func TestCreateStack_OnFailureRollback_LeavesRollbackComplete(t *testing.T) {
	t.Parallel()

	b := newBackend()
	failTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Bucket": {
				"Type": "AWS::S3::Bucket",
				"Properties": {"BucketName": {"Fn::ImportValue": "nonexistent-export"}}
			}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(),
		"rollback-on-fail",
		failTemplate,
		nil,
		cloudformation.StackOptions{
			OnFailure: "ROLLBACK",
		},
	)
	require.NoError(t, err)

	// No OnFailure=DELETE so stack stays in ROLLBACK_COMPLETE or CREATE_FAILED.
	assert.NotEqual(t, "DELETE_COMPLETE", stack.StackStatus)
}

// ---- AllowedValues parameter validation ----------------------------------------

func TestCreateStack_AllowedValues_ValidValue_Accepted(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {
				"Type": "String",
				"AllowedValues": ["dev", "staging", "prod"]
			}
		},
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(), "av-ok", tmpl,
		[]cloudformation.Parameter{{ParameterKey: "Env", ParameterValue: "dev"}},
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "CREATE_COMPLETE", stack.StackStatus)
}

func TestCreateStack_AllowedValues_InvalidValue_Rejected(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {
				"Type": "String",
				"AllowedValues": ["dev", "staging", "prod"]
			}
		},
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(), "av-bad", tmpl,
		[]cloudformation.Parameter{{ParameterKey: "Env", ParameterValue: "qa"}},
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	// AWS rolls back to ROLLBACK_COMPLETE on parameter validation failure.
	assert.Equal(t, "ROLLBACK_COMPLETE", stack.StackStatus)
	assert.Contains(t, stack.StackStatusReason, "parameter validation failed")
}

func TestCreateStack_AllowedValues_ConstraintDescription_InError(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {
				"Type": "String",
				"AllowedValues": ["dev", "prod"],
				"ConstraintDescription": "must be dev or prod"
			}
		},
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(), "av-constraint", tmpl,
		[]cloudformation.Parameter{{ParameterKey: "Env", ParameterValue: "qa"}},
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "ROLLBACK_COMPLETE", stack.StackStatus)
	assert.Contains(t, stack.StackStatusReason, "must be dev or prod")
}

func TestCreateStack_AllowedValues_DefaultUsed_MustBeInList(t *testing.T) {
	t.Parallel()

	b := newBackend()
	// Default is in AllowedValues — should pass.
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {
				"Type": "String",
				"Default": "dev",
				"AllowedValues": ["dev", "prod"]
			}
		},
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(),
		"av-default-ok",
		tmpl,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "CREATE_COMPLETE", stack.StackStatus)
}

func TestUpdateStack_AllowedValues_InvalidValue_Rejected(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {
				"Type": "String",
				"AllowedValues": ["dev", "prod"]
			}
		},
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		}
	}`

	_, err := b.CreateStack(
		t.Context(), "upd-av", tmpl,
		[]cloudformation.Parameter{{ParameterKey: "Env", ParameterValue: "dev"}},
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	updated, err := b.UpdateStack(
		t.Context(), "upd-av", tmpl,
		[]cloudformation.Parameter{{ParameterKey: "Env", ParameterValue: "qa"}},
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "UPDATE_ROLLBACK_COMPLETE", updated.StackStatus)
}

// ---- GetTemplateSummary: AllowedValues and NoEcho surfaced ---------------------

func TestGetTemplateSummary_AllowedValuesAndNoEcho(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {
				"Type": "String",
				"AllowedValues": ["dev", "prod"],
				"ConstraintDescription": "must be dev or prod"
			},
			"DBPassword": {
				"Type": "String",
				"NoEcho": true
			}
		},
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		}
	}`

	summary, err := b.GetTemplateSummary(tmpl, "")
	require.NoError(t, err)

	paramMap := make(map[string]cloudformation.ParameterDeclaration, len(summary.Parameters))
	for _, p := range summary.Parameters {
		paramMap[p.ParameterKey] = p
	}

	env := paramMap["Env"]
	assert.Equal(t, []string{"dev", "prod"}, env.AllowedValues)
	assert.Equal(t, "must be dev or prod", env.ConstraintDescription)

	dbp := paramMap["DBPassword"]
	assert.True(t, dbp.NoEcho)
}

// ---- UpdateStack: RoleARN and Capabilities updated ----------------------------

func TestUpdateStack_CapabilitiesUpdated(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"ucap-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			Capabilities: []string{"CAPABILITY_IAM"},
		},
	)
	require.NoError(t, err)

	updated, err := b.UpdateStack(
		t.Context(),
		"ucap-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{
			Capabilities: []string{"CAPABILITY_IAM", "CAPABILITY_AUTO_EXPAND"},
		},
	)
	require.NoError(t, err)
	assert.Contains(t, updated.Capabilities, "CAPABILITY_AUTO_EXPAND")
}

// ---- Nested stack: AWS::CloudFormation::Stack ---------------------------------

func TestCreateStack_NestedStack_Provisioned(t *testing.T) {
	t.Parallel()

	b := newBackend()
	childTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		}
	}`

	parentTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Child": {
				"Type": "AWS::CloudFormation::Stack",
				"Properties": {
					"TemplateBody": ` + quoteJSON(childTemplate) + `
				}
			}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(),
		"parent-stack",
		parentTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "CREATE_COMPLETE", stack.StackStatus)

	// Child stack was created.
	childStack, err := b.DescribeStack("Child")
	require.NoError(t, err)
	assert.Equal(t, "CREATE_COMPLETE", childStack.StackStatus)
}

func TestDeleteStack_NestedStack_DeletesChild(t *testing.T) {
	t.Parallel()

	b := newBackend()
	childTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Queue": {"Type": "AWS::SQS::Queue"}
		}
	}`

	parentTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"ChildStack": {
				"Type": "AWS::CloudFormation::Stack",
				"Properties": {
					"TemplateBody": ` + quoteJSON(childTemplate) + `
				}
			}
		}
	}`

	_, err := b.CreateStack(
		t.Context(),
		"parent-del",
		parentTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	// Verify child exists.
	_, err = b.DescribeStack("ChildStack")
	require.NoError(t, err)

	// Delete parent — child should be deleted too.
	err = b.DeleteStack(t.Context(), "parent-del")
	require.NoError(t, err)

	// Child transitions to DELETE_COMPLETE (not removed from index).
	child, err := b.DescribeStack("ChildStack")
	require.NoError(t, err)
	assert.Equal(t, "DELETE_COMPLETE", child.StackStatus)
}

// ---- Stack lifecycle state machine completeness --------------------------------

func TestStackLifecycle_CreateUpdateDelete(t *testing.T) {
	t.Parallel()

	b := newBackend()

	// Create.
	stack, err := b.CreateStack(
		t.Context(),
		"lifecycle",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "CREATE_COMPLETE", stack.StackStatus)
	assert.False(t, stack.CreationTime.IsZero())

	// Update.
	updated, err := b.UpdateStack(
		t.Context(),
		"lifecycle",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "UPDATE_COMPLETE", updated.StackStatus)
	assert.NotNil(t, updated.LastUpdatedTime)

	// Delete.
	err = b.DeleteStack(t.Context(), "lifecycle")
	require.NoError(t, err)

	final, err := b.DescribeStack("lifecycle")
	require.NoError(t, err)
	assert.Equal(t, "DELETE_COMPLETE", final.StackStatus)
	assert.NotNil(t, final.DeletionTime)
}

func TestStackLifecycle_RollbackOnCreateFailure(t *testing.T) {
	t.Parallel()

	b := newBackend()
	failTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Bucket": {
				"Type": "AWS::S3::Bucket",
				"Properties": {"BucketName": {"Fn::ImportValue": "no-such-export"}}
			}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(),
		"fail-stack",
		failTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	// AWS rolls back to ROLLBACK_COMPLETE when import fails.
	assert.Equal(t, "ROLLBACK_COMPLETE", stack.StackStatus)

	// Events contain ROLLBACK_IN_PROGRESS.
	evtPage, err := b.DescribeStackEvents("fail-stack", "")
	require.NoError(t, err)
	events := evtPage.Data
	statuses := make([]string, len(events))
	for i, e := range events {
		statuses[i] = e.ResourceStatus
	}
	assert.Contains(t, statuses, "ROLLBACK_IN_PROGRESS")
	assert.Contains(t, statuses, "ROLLBACK_COMPLETE")
}

func TestStackLifecycle_UpdateRollback(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"upd-rb",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	// Bad update (broken import).
	badTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Bucket": {
				"Type": "AWS::S3::Bucket",
				"Properties": {"BucketName": {"Fn::ImportValue": "no-such-export"}}
			}
		}
	}`

	updated, err := b.UpdateStack(
		t.Context(),
		"upd-rb",
		badTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "UPDATE_ROLLBACK_COMPLETE", updated.StackStatus)
}

// ---- ExportName + ListExports cross-stack refs ----------------------------------

func TestExports_CrossStackReference(t *testing.T) {
	t.Parallel()

	b := newBackend()

	exporterTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Queue": {"Type": "AWS::SQS::Queue"}
		},
		"Outputs": {
			"QueueURL": {
				"Value": {"Ref": "Queue"},
				"Export": {"Name": "shared-queue-url"}
			}
		}
	}`

	_, err := b.CreateStack(
		t.Context(),
		"exporter",
		exporterTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	exports, err := b.ListExports("")
	require.NoError(t, err)
	require.Len(t, exports.Data, 1)
	assert.Equal(t, "shared-queue-url", exports.Data[0].Name)

	// Importer can reference the export.
	importerTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Queue": {"Type": "AWS::SQS::Queue"}
		},
		"Outputs": {
			"ImportedURL": {
				"Value": {"Fn::ImportValue": "shared-queue-url"}
			}
		}
	}`

	importerStack, err := b.CreateStack(
		t.Context(),
		"importer",
		importerTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "CREATE_COMPLETE", importerStack.StackStatus)
	require.Len(t, importerStack.Outputs, 1)
	assert.NotEmpty(t, importerStack.Outputs[0].OutputValue)
}

// ---- Tags stored and returned --------------------------------------------------

func TestCreateStack_TagsStored(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tags := []cloudformation.Tag{{Key: "Team", Value: "platform"}, {Key: "Env", Value: "prod"}}
	stack, err := b.CreateStack(
		t.Context(),
		"tagged",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{Tags: tags},
	)
	require.NoError(t, err)
	assert.Len(t, stack.Tags, 2)

	s2, err := b.DescribeStack("tagged")
	require.NoError(t, err)
	assert.Len(t, s2.Tags, 2)
}

// ---- ChangeSet lifecycle -------------------------------------------------------

func TestChangeSet_CreateExecuteDelete(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"cs-base",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	cs, err := b.CreateChangeSet(
		t.Context(),
		"cs-base",
		"my-cs",
		modifiedTemplate,
		"test changeset",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "cs-base", cs.StackName)
	assert.Equal(t, "CREATE_COMPLETE", cs.Status)

	// Describe.
	described, err := b.DescribeChangeSet("cs-base", "my-cs")
	require.NoError(t, err)
	assert.Equal(t, "my-cs", described.ChangeSetName)

	// List.
	list, err := b.ListChangeSets("cs-base", "")
	require.NoError(t, err)
	assert.Len(t, list.Data, 1)

	// Execute.
	err = b.ExecuteChangeSet(t.Context(), "cs-base", "my-cs")
	require.NoError(t, err)

	// Delete should fail — already executed.
	err = b.DeleteChangeSet("cs-base", "my-cs")
	require.ErrorIs(t, err, cloudformation.ErrChangeSetNotFound)
}

func TestChangeSet_Delete(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"cs-del",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	_, err = b.CreateChangeSet(t.Context(), "cs-del", "del-cs", simpleTemplate, "", nil, nil, nil)
	require.NoError(t, err)

	err = b.DeleteChangeSet("cs-del", "del-cs")
	require.NoError(t, err)

	_, err = b.DescribeChangeSet("cs-del", "del-cs")
	require.ErrorIs(t, err, cloudformation.ErrChangeSetNotFound)
}

// ---- StackSet multi-account multi-region ---------------------------------------

func TestStackSet_CreateUpdateDeleteWithInstances(t *testing.T) {
	t.Parallel()

	b := newBackend()

	ss, err := b.CreateStackSet("my-ss", "test set", simpleTemplate, cloudformation.StackSetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "my-ss", ss.StackSetName)
	assert.Equal(t, "ACTIVE", ss.Status)

	// List.
	list, err := b.ListStackSets("", "")
	require.NoError(t, err)
	assert.Len(t, list.Data, 1)

	// Create instances.
	accounts := []string{"111111111111", "222222222222"}
	regions := []string{"us-east-1", "us-west-2"}
	_, err = b.CreateStackInstances(t.Context(), "my-ss", accounts, nil, regions)
	require.NoError(t, err)

	instances, err := b.ListStackInstances("my-ss", "", cloudformation.ListStackInstancesFilter{})
	require.NoError(t, err)
	assert.Len(t, instances.Data, 4) // 2 accounts × 2 regions

	// Describe a specific instance.
	inst, err := b.DescribeStackInstance("my-ss", "111111111111", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "CURRENT", inst.Status)

	// Update set.
	updated, _, err := b.UpdateStackSet("my-ss", "", simpleTemplate, cloudformation.StackSetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", updated.Status)

	// Delete instances.
	_, err = b.DeleteStackInstances(t.Context(), "my-ss", accounts, nil, regions)
	require.NoError(t, err)

	remaining, err := b.ListStackInstances("my-ss", "", cloudformation.ListStackInstancesFilter{})
	require.NoError(t, err)
	assert.Empty(t, remaining.Data)

	// Delete set.
	err = b.DeleteStackSet("my-ss")
	require.NoError(t, err)

	_, err = b.DescribeStackSet("my-ss")
	require.ErrorIs(t, err, cloudformation.ErrStackSetNotFound)
}

// ---- Drift detection ----------------------------------------------------------

func TestDriftDetection_FullCycle(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"drift-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	detectionID, err := b.DetectStackDrift("drift-stack")
	require.NoError(t, err)
	assert.NotEmpty(t, detectionID)

	status, err := b.DescribeStackDriftDetectionStatus(detectionID)
	require.NoError(t, err)
	assert.Equal(t, "DETECTION_COMPLETE", status.DetectionStatus)

	drifts, err := b.DescribeStackResourceDrifts("drift-stack")
	require.NoError(t, err)
	assert.NotNil(t, drifts) // may be empty slice
}

func TestDriftDetection_ResourceLevel(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Q": {"Type": "AWS::SQS::Queue"}
		}
	}`
	_, err := b.CreateStack(t.Context(), "rdrift", tmpl, nil, cloudformation.StackOptions{})
	require.NoError(t, err)

	drift, err := b.DetectStackResourceDrift("rdrift", "Q")
	require.NoError(t, err)
	assert.Equal(t, "Q", drift.LogicalResourceID)
	assert.Equal(t, "AWS::SQS::Queue", drift.ResourceType)
	assert.Equal(t, "IN_SYNC", drift.StackResourceDriftStatus)
}

// ---- Resource event history ----------------------------------------------------

func TestDescribeStackEvents_ReverseChrono(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"events-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	evtPage, err := b.DescribeStackEvents("events-stack", "")
	require.NoError(t, err)
	events := evtPage.Data
	assert.NotEmpty(t, events)

	// Most recent first.
	for i := 1; i < len(events); i++ {
		assert.False(t, events[i].Timestamp.After(events[i-1].Timestamp),
			"events not in reverse chrono order at index %d", i)
	}
}

// ---- Parameters with outputs ---------------------------------------------------

func TestCreateStack_ParametersResolvedInOutputs(t *testing.T) {
	t.Parallel()

	b := newBackend()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"EnvName": {"Type": "String"}
		},
		"Resources": {
			"Bucket": {"Type": "AWS::S3::Bucket"}
		},
		"Outputs": {
			"EnvOut": {"Value": {"Ref": "EnvName"}}
		}
	}`

	stack, err := b.CreateStack(
		t.Context(), "param-out", tmpl,
		[]cloudformation.Parameter{{ParameterKey: "EnvName", ParameterValue: "production"}},
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)
	require.Len(t, stack.Outputs, 1)
	assert.Equal(t, "production", stack.Outputs[0].OutputValue)
}

// ---- Stack policy --------------------------------------------------------------

func TestStackPolicy_SetAndGet(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"policy-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	policy := `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`
	err = b.SetStackPolicy("policy-stack", policy)
	require.NoError(t, err)

	got, err := b.GetStackPolicy("policy-stack")
	require.NoError(t, err)
	assert.Equal(t, policy, got)
}

// ---- ContinueUpdateRollback and CancelUpdateStack ------------------------------

func TestContinueUpdateRollback(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"cur-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	// Should not error even in non-rollback state (permissive implementation).
	err = b.ContinueUpdateRollback(t.Context(), "cur-stack")
	require.NoError(t, err)
}

func TestCancelUpdateStack(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"cancel-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	b.ForceStackStatus("cancel-stack", "UPDATE_IN_PROGRESS")

	err = b.CancelUpdateStack(t.Context(), "cancel-stack")
	require.NoError(t, err)
}

// ---- ValidateTemplate ----------------------------------------------------------

func TestValidateTemplate_ValidJSON(t *testing.T) {
	t.Parallel()

	b := newBackend()
	summary, err := b.ValidateTemplate(simpleTemplate)
	require.NoError(t, err)
	assert.NotNil(t, summary)
}

func TestValidateTemplate_InvalidTemplate(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.ValidateTemplate("not-json-or-yaml{{{")
	require.Error(t, err)
}

// ---- AccountLimits -------------------------------------------------------------

func TestDescribeAccountLimits(t *testing.T) {
	t.Parallel()

	b := newBackend()
	limits := b.DescribeAccountLimits()
	assert.NotEmpty(t, limits)

	for _, l := range limits {
		assert.NotEmpty(t, l.Name)
		assert.Positive(t, l.Value)
	}
}

// ---- RollbackStack ------------------------------------------------------------

func TestRollbackStack(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"rb-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	_, err = b.RollbackStack(t.Context(), "rb-stack")
	require.NoError(t, err)
}

// ---- GetTemplate --------------------------------------------------------------

func TestGetTemplate(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.CreateStack(
		t.Context(),
		"gt-stack",
		simpleTemplate,
		nil,
		cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	body, err := b.GetTemplate("gt-stack")
	require.NoError(t, err)
	assert.NotEmpty(t, body)
}

// ---- EstimateTemplateCost -----------------------------------------------------

func TestEstimateTemplateCost(t *testing.T) {
	t.Parallel()

	b := newBackend()
	url, err := b.EstimateTemplateCost(simpleTemplate, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, url)
}

// ---- helpers ------------------------------------------------------------------

// quoteJSON encodes a string as a JSON string literal (for embedding in template literals).
func quoteJSON(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for _, c := range []byte(s) {
		switch c {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, c)
		}
	}
	out = append(out, '"')

	return string(out)
}
