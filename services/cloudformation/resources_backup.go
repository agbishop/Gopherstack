package cloudformation

import (
	"fmt"
	"strings"
)

// ---- Backup Vault ----

func (rc *ResourceCreator) createBackupVault(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.Backup == nil {
		return logicalID + "-stub", nil
	}

	name := strProp(props, "BackupVaultName", params, physicalIDs)
	if name == "" {
		name = logicalID
	}

	vault, err := rc.backends.Backup.Backend.CreateBackupVault(name, "", "", nil)
	if err != nil {
		return "", fmt.Errorf("create Backup Vault %s: %w", name, err)
	}

	return vault.BackupVaultArn, nil
}

func (rc *ResourceCreator) deleteBackupVault(arn string) error {
	if rc.backends.Backup == nil {
		return nil
	}

	name := resourceNameFromARN(arn)

	return rc.backends.Backup.Backend.DeleteBackupVault(name)
}

// ---- Backup Plan ----

func (rc *ResourceCreator) createBackupPlan(
	logicalID string,
	props map[string]any,
	_, _ map[string]string,
) (string, error) {
	if rc.backends.Backup == nil {
		return logicalID + "-stub", nil
	}

	name := logicalID
	if planMap, ok := props["BackupPlan"].(map[string]any); ok {
		if n, nOK := planMap["BackupPlanName"].(string); nOK && n != "" {
			name = n
		}
	}

	plan, err := rc.backends.Backup.Backend.CreateBackupPlan(name, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("create Backup Plan %s: %w", name, err)
	}

	return plan.BackupPlanID, nil
}

func (rc *ResourceCreator) deleteBackupPlan(id string) error {
	if rc.backends.Backup == nil {
		return nil
	}

	return rc.backends.Backup.Backend.DeleteBackupPlan(id)
}

// ---- Backup Selection ----

// createBackupSelection reads the request shape CreateBackupSelectionInput actually models
// (backup@v1.59.4 api_op_CreateBackupSelection.go): BackupPlanId is top-level, but
// SelectionName/IamRoleArn/Resources/NotResources live nested under a BackupSelection struct
// (types/types.go:653), matching the CloudFormation resource's own Properties shape.
func (rc *ResourceCreator) createBackupSelection(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.Backup == nil {
		return logicalID + "-stub", nil
	}

	planID := strProp(props, "BackupPlanId", params, physicalIDs)
	sel, _ := props["BackupSelection"].(map[string]any)

	selectionName := strProp(sel, "SelectionName", params, physicalIDs)
	if selectionName == "" {
		selectionName = logicalID
	}

	iamRoleArn := strProp(sel, "IamRoleArn", params, physicalIDs)
	resources := strSliceProp(sel["Resources"], params, physicalIDs)
	notResources := strSliceProp(sel["NotResources"], params, physicalIDs)

	created, err := rc.backends.Backup.Backend.CreateBackupSelection(
		planID, selectionName, iamRoleArn, resources, notResources, nil, nil,
	)
	if err != nil {
		return "", fmt.Errorf("create Backup Selection %s: %w", selectionName, err)
	}

	// Composite physical ID: DeleteBackupSelection needs both IDs (backup/selections.go),
	// and CloudFormation never stores creation-time Properties in a resolved, Ref-free form
	// for Delete to re-derive BackupPlanId from later.
	return planID + "|" + created.SelectionID, nil
}

func (rc *ResourceCreator) deleteBackupSelection(physicalID string) error {
	if rc.backends.Backup == nil {
		return nil
	}

	planID, selectionID, ok := strings.Cut(physicalID, "|")
	if !ok {
		return nil
	}

	return rc.backends.Backup.Backend.DeleteBackupSelection(planID, selectionID)
}
