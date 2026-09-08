package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// TestCreateBackupSelection_ReadsNestedProperties confirms SelectionName/IamRoleArn/Resources
// are read from the nested BackupSelection property, matching CreateBackupSelectionInput's real
// wire shape (backup@v1.59.4 api_op_CreateBackupSelection.go: BackupPlanId top-level,
// BackupSelection *types.BackupSelection nested) instead of a flat top-level layout.
func TestCreateBackupSelection_ReadsNestedProperties(t *testing.T) {
	t.Parallel()

	backends := newMoreTypesServiceBackends(t)
	rc := cloudformation.NewResourceCreator(backends)

	plan, err := backends.Backup.Backend.CreateBackupPlan("unit-cfn-plan", nil, nil, nil)
	require.NoError(t, err)

	props := map[string]any{
		"BackupPlanId": plan.BackupPlanID,
		"BackupSelection": map[string]any{
			"SelectionName": "unit-cfn-selection",
			"IamRoleArn":    "arn:aws:iam::000000000000:role/UnitCFNBackupRole",
			"Resources":     []any{"arn:aws:s3:::unit-cfn-bucket"},
		},
	}

	physID, err := rc.Create(t.Context(), "MySelection", "AWS::Backup::BackupSelection", props, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, physID)

	selections, err := backends.Backup.Backend.ListBackupSelections(plan.BackupPlanID)
	require.NoError(t, err)
	require.Len(t, selections, 1)

	sel := selections[0]
	assert.Equal(t, "unit-cfn-selection", sel.SelectionName)
	assert.Equal(t, "arn:aws:iam::000000000000:role/UnitCFNBackupRole", sel.IAMRoleArn)
	assert.Equal(t, []string{"arn:aws:s3:::unit-cfn-bucket"}, sel.Resources)
}

// TestDeleteBackupSelection_RemovesSelection confirms DeleteStack's resource teardown actually
// removes a BackupSelection instead of leaving it as a ghost row (previously a completely
// unhandled resource type in the delete dispatch, so the selection survived every delete).
func TestDeleteBackupSelection_RemovesSelection(t *testing.T) {
	t.Parallel()

	backends := newMoreTypesServiceBackends(t)
	rc := cloudformation.NewResourceCreator(backends)

	plan, err := backends.Backup.Backend.CreateBackupPlan("unit-cfn-plan-del", nil, nil, nil)
	require.NoError(t, err)

	props := map[string]any{
		"BackupPlanId": plan.BackupPlanID,
		"BackupSelection": map[string]any{
			"SelectionName": "unit-cfn-selection-del",
			"IamRoleArn":    "arn:aws:iam::000000000000:role/UnitCFNBackupRole",
		},
	}

	physID, err := rc.Create(t.Context(), "MySelection", "AWS::Backup::BackupSelection", props, nil, nil)
	require.NoError(t, err)

	before, err := backends.Backup.Backend.ListBackupSelections(plan.BackupPlanID)
	require.NoError(t, err)
	require.Len(t, before, 1)

	err = rc.Delete(t.Context(), "AWS::Backup::BackupSelection", physID, props)
	require.NoError(t, err)

	after, err := backends.Backup.Backend.ListBackupSelections(plan.BackupPlanID)
	require.NoError(t, err)
	assert.Empty(t, after, "selection must not survive DeleteStack as a ghost row")
}
