package cloudformation

import (
	"fmt"
	"strings"
)

// ---- Redshift ----

func (rc *ResourceCreator) createRedshiftCluster(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.Redshift == nil {
		return logicalID + "-stub", nil
	}

	id := strProp(props, "ClusterIdentifier", params, physicalIDs)
	if id == "" {
		id = strings.ToLower(logicalID)
	}

	nodeType := strProp(props, "NodeType", params, physicalIDs)
	dbName := strProp(props, "DBName", params, physicalIDs)
	masterUser := strProp(props, "MasterUsername", params, physicalIDs)

	cluster, err := rc.backends.Redshift.Backend.CreateCluster(id, nodeType, dbName, masterUser, nil, "")
	if err != nil {
		return "", fmt.Errorf("create Redshift cluster %s: %w", id, err)
	}

	return cluster.ClusterIdentifier, nil
}

func (rc *ResourceCreator) deleteRedshiftCluster(id string) error {
	if rc.backends.Redshift == nil {
		return nil
	}

	_, err := rc.backends.Redshift.Backend.DeleteCluster(id)

	return err
}
