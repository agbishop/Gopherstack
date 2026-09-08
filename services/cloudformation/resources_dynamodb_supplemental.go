package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	ddbbackend "github.com/blackbirdworks/gopherstack/services/dynamodb"
)

func (rc *ResourceCreator) createDynamoDBSupplementalResource(
	ctx context.Context,
	logicalID, resourceType string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, bool, error) {
	switch resourceType {
	case "AWS::DynamoDB::GlobalTable":
		id, err := rc.createDynamoDBGlobalTable(ctx, logicalID, props, params, physicalIDs)

		return id, true, err
	default:
		return "", false, nil
	}
}

func (rc *ResourceCreator) createDynamoDBGlobalTable(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.DynamoDB == nil {
		return logicalID + "-stub", nil
	}

	tableName := strProp(props, "TableName", params, physicalIDs)
	if tableName == "" {
		tableName = logicalID
	}

	// Build replication group from Replicas prop.
	var replicas []ddbtypes.Replica
	if replicaList, hasReplicas := props["Replicas"].([]any); hasReplicas {
		for _, r := range replicaList {
			if rm, isMap := r.(map[string]any); isMap {
				if region, hasRegion := rm["Region"].(string); hasRegion && region != "" {
					regionCopy := region
					replicas = append(replicas, ddbtypes.Replica{RegionName: &regionCopy})
				}
			}
		}
	}

	if len(replicas) == 0 {
		region := "us-east-1"
		replicas = []ddbtypes.Replica{{RegionName: &region}}
	}

	out, err := rc.backends.DynamoDB.Backend.CreateGlobalTable(ctx, &awsddb.CreateGlobalTableInput{
		GlobalTableName:  &tableName,
		ReplicationGroup: replicas,
	})
	if err != nil {
		return "", fmt.Errorf("create DynamoDB global table %s: %w", tableName, err)
	}

	if out.GlobalTableDescription != nil && out.GlobalTableDescription.GlobalTableArn != nil {
		return *out.GlobalTableDescription.GlobalTableArn, nil
	}

	return tableName, nil
}

// deleteDynamoDBSupplementalResource handles deletion for DynamoDB supplemental resource types.
func (rc *ResourceCreator) deleteDynamoDBSupplementalResource(
	ctx context.Context,
	resourceType, physicalID string,
) (bool, error) {
	if resourceType != "AWS::DynamoDB::GlobalTable" {
		return false, nil
	}

	return true, rc.deleteDynamoDBGlobalTable(ctx, physicalID)
}

// deleteDynamoDBGlobalTable tears down every replica table CreateGlobalTable provisioned.
// A global table has no single-call delete API in real AWS either: deleting it means
// deleting each regional replica via DeleteTable, which is what real CloudFormation does
// when a stack containing AWS::DynamoDB::GlobalTable is deleted.
func (rc *ResourceCreator) deleteDynamoDBGlobalTable(ctx context.Context, physicalID string) error {
	if rc.backends.DynamoDB == nil {
		return nil
	}

	name := resourceNameFromARN(physicalID)

	out, err := rc.backends.DynamoDB.Backend.DescribeGlobalTable(ctx, &awsddb.DescribeGlobalTableInput{
		GlobalTableName: &name,
	})
	if err != nil {
		// Already gone (e.g. a retry after a partial prior delete) — idempotent no-op,
		// matching DeleteStack's treatment of every other already-deleted resource.
		return nil //nolint:nilerr // intentional idempotent no-op, see comment above
	}

	for _, replica := range out.GlobalTableDescription.ReplicationGroup {
		region := aws.ToString(replica.RegionName)

		_, derr := rc.backends.DynamoDB.Backend.DeleteTable(
			ddbbackend.WithRegion(ctx, region),
			&awsddb.DeleteTableInput{TableName: &name},
		)
		if derr != nil {
			return fmt.Errorf("delete DynamoDB global table %s replica in %s: %w", name, region, derr)
		}
	}

	return nil
}
