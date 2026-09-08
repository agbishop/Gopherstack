package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/services/neptune"
)

// ---- Neptune ----

func (rc *ResourceCreator) createNeptuneCluster(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.Neptune == nil {
		return logicalID + "-stub", nil
	}

	id := strProp(props, "DBClusterIdentifier", params, physicalIDs)
	if id == "" {
		id = strings.ToLower(logicalID)
	}

	paramGroupName := strProp(props, "DBClusterParameterGroupName", params, physicalIDs)

	cluster, err := rc.backends.Neptune.Backend.CreateDBCluster(
		ctx, id, paramGroupName, 0, neptune.DBClusterCreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("create Neptune cluster %s: %w", id, err)
	}

	return cluster.DBClusterIdentifier, nil
}

func (rc *ResourceCreator) deleteNeptuneCluster(ctx context.Context, arn string) error {
	if rc.backends.Neptune == nil {
		return nil
	}

	id := resourceNameFromARN(arn)

	_, err := rc.backends.Neptune.Backend.DeleteDBCluster(
		ctx,
		id,
		neptune.DBClusterDeleteOptions{SkipFinalSnapshot: true},
	)

	return err
}

func (rc *ResourceCreator) createNeptuneInstance(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.Neptune == nil {
		return logicalID + "-stub", nil
	}

	id := strProp(props, "DBInstanceIdentifier", params, physicalIDs)
	if id == "" {
		id = strings.ToLower(logicalID)
	}

	clusterID := strProp(props, "DBClusterIdentifier", params, physicalIDs)
	instanceClass := strProp(props, "DBInstanceClass", params, physicalIDs)

	instance, err := rc.backends.Neptune.Backend.CreateDBInstance(
		ctx, id, clusterID, instanceClass, neptune.DBInstanceCreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("create Neptune instance %s: %w", id, err)
	}

	return instance.DBInstanceIdentifier, nil
}

func (rc *ResourceCreator) deleteNeptuneInstance(ctx context.Context, arn string) error {
	if rc.backends.Neptune == nil {
		return nil
	}

	id := resourceNameFromARN(arn)

	// Neptune refuses to delete a cluster's only instance. During stack teardown
	// the cluster is deleted next and cascades to its instances, so treat that
	// rejection as satisfied rather than failing the stack.
	if _, err := rc.backends.Neptune.Backend.DeleteDBInstance(ctx, id); err != nil &&
		!errors.Is(err, neptune.ErrInvalidDBInstanceStateFault) {
		return err
	}

	return nil
}
