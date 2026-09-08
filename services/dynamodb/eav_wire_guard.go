package dynamodb

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

// The functions below run validateEAVTypes on the wire ExpressionAttributeValues
// before handing off to the models wire-to-SDK converters. Without this, a
// malformed EAV shape fails inside ToSDKItem with an unclassified Go error that
// handleError's classifyError falls back to a 500 InternalServerError for,
// instead of the 400 ValidationException DynamoDB actually returns.

func toSDKPutItemInputChecked(input *models.PutItemInput) (*dynamodb.PutItemInput, error) {
	if err := validateEAVTypes(input.ExpressionAttributeValues); err != nil {
		return nil, err
	}

	return models.ToSDKPutItemInput(input)
}

func toSDKDeleteItemInputChecked(input *models.DeleteItemInput) (*dynamodb.DeleteItemInput, error) {
	if err := validateEAVTypes(input.ExpressionAttributeValues); err != nil {
		return nil, err
	}

	return models.ToSDKDeleteItemInput(input)
}

func toSDKUpdateItemInputChecked(input *models.UpdateItemInput) (*dynamodb.UpdateItemInput, error) {
	if err := validateEAVTypes(input.ExpressionAttributeValues); err != nil {
		return nil, err
	}

	return models.ToSDKUpdateItemInput(input)
}

func toSDKQueryInputChecked(input *models.QueryInput) (*dynamodb.QueryInput, error) {
	if err := validateEAVTypes(input.ExpressionAttributeValues); err != nil {
		return nil, err
	}

	return models.ToSDKQueryInput(input)
}

func toSDKScanInputChecked(input *models.ScanInput) (*dynamodb.ScanInput, error) {
	if err := validateEAVTypes(input.ExpressionAttributeValues); err != nil {
		return nil, err
	}

	return models.ToSDKScanInput(input)
}

func toSDKSearchVectorsInputChecked(input *models.SearchVectorsInput) (*dynamodb.SearchVectorsInput, error) {
	if err := validateEAVTypes(input.ExpressionAttributeValues); err != nil {
		return nil, err
	}

	return models.ToSDKSearchVectorsInput(input)
}

func toSDKTransactWriteItemsInputChecked(
	input *models.TransactWriteItemsInput,
) (*dynamodb.TransactWriteItemsInput, error) {
	for _, item := range input.TransactItems {
		var eav map[string]any

		switch {
		case item.Put != nil:
			eav = item.Put.ExpressionAttributeValues
		case item.Delete != nil:
			eav = item.Delete.ExpressionAttributeValues
		case item.Update != nil:
			eav = item.Update.ExpressionAttributeValues
		case item.ConditionCheck != nil:
			eav = item.ConditionCheck.ExpressionAttributeValues
		}

		if err := validateEAVTypes(eav); err != nil {
			return nil, err
		}
	}

	return models.ToSDKTransactWriteItemsInput(input)
}
