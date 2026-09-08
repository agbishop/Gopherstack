package dynamodb_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/services/dynamodb"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

func TestHandler_MalformedEAV_Rejected(t *testing.T) {
	t.Parallel()

	badEAV := map[string]any{":bad": "not-a-map"}

	tests := []struct {
		body   any
		name   string
		action string
	}{
		{
			name:   "putitem",
			action: "PutItem",
			body: models.PutItemInput{
				TableName:                 "EAVTable",
				Item:                      map[string]any{"pk": map[string]any{"S": "item1"}},
				ExpressionAttributeValues: badEAV,
			},
		},
		{
			name:   "deleteitem",
			action: "DeleteItem",
			body: models.DeleteItemInput{
				TableName:                 "EAVTable",
				Key:                       map[string]any{"pk": map[string]any{"S": "item1"}},
				ConditionExpression:       "attribute_exists(pk)",
				ExpressionAttributeValues: badEAV,
			},
		},
		{
			name:   "updateitem",
			action: "UpdateItem",
			body: models.UpdateItemInput{
				TableName:                 "EAVTable",
				Key:                       map[string]any{"pk": map[string]any{"S": "item1"}},
				UpdateExpression:          "SET other = :bad",
				ExpressionAttributeValues: badEAV,
			},
		},
		{
			name:   "query",
			action: "Query",
			body: models.QueryInput{
				TableName:                 "EAVTable",
				KeyConditionExpression:    "pk = :bad",
				ExpressionAttributeValues: badEAV,
			},
		},
		{
			name:   "scan",
			action: "Scan",
			body: models.ScanInput{
				TableName:                 "EAVTable",
				FilterExpression:          "pk = :bad",
				ExpressionAttributeValues: badEAV,
			},
		},
		{
			name:   "searchvectors",
			action: "SearchVectors",
			body: models.SearchVectorsInput{
				TableName:                 "EAVTable",
				IndexName:                 "vec-index",
				ExpressionAttributeValues: badEAV,
			},
		},
		{
			name:   "transactwriteitems_put",
			action: "TransactWriteItems",
			body: models.TransactWriteItemsInput{
				TransactItems: []models.TransactWriteItem{
					{
						Put: &models.PutItemInput{
							TableName:                 "EAVTable",
							Item:                      map[string]any{"pk": map[string]any{"S": "item1"}},
							ExpressionAttributeValues: badEAV,
						},
					},
				},
			},
		},
		{
			name:   "transactwriteitems_conditioncheck",
			action: "TransactWriteItems",
			body: models.TransactWriteItemsInput{
				TransactItems: []models.TransactWriteItem{
					{
						ConditionCheck: &models.ConditionCheckInput{
							TableName:                 "EAVTable",
							Key:                       map[string]any{"pk": map[string]any{"S": "item1"}},
							ConditionExpression:       "attribute_exists(pk)",
							ExpressionAttributeValues: badEAV,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := dynamodb.NewInMemoryDB()
			handler := dynamodb.NewHandler(backend)
			createTableHelper(t, backend, "EAVTable", "pk")

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(mustMarshal(t, tt.body)))
			req.Header.Set("X-Amz-Target", "DynamoDB_20120810."+tt.action)
			w := httptest.NewRecorder()

			_ = serveEchoHandler(handler.Handler(), w, req)

			resp := w.Result()
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.Contains(t, w.Body.String(), "ValidationException")
			assert.Contains(t, w.Body.String(), "ExpressionAttributeValues")
		})
	}
}

func TestHandler_ValidEAVShapes_StillAccepted(t *testing.T) {
	t.Parallel()

	backend := dynamodb.NewInMemoryDB()
	handler := dynamodb.NewHandler(backend)
	createTableHelper(t, backend, "EAVValidTable", "pk", "sk")

	for _, item := range []struct{ pk, sk string }{{"pk1", "sk1"}} {
		putInput := models.PutItemInput{
			TableName: "EAVValidTable",
			Item: map[string]any{
				"pk": map[string]any{"S": item.pk},
				"sk": map[string]any{"S": item.sk},
			},
		}
		sdkPut, convErr := models.ToSDKPutItemInput(&putInput)
		if convErr != nil {
			t.Fatalf("seed put conversion: %v", convErr)
		}

		if _, putErr := backend.PutItem(t.Context(), sdkPut); putErr != nil {
			t.Fatalf("seed put: %v", putErr)
		}
	}

	body := mustMarshal(t, models.QueryInput{
		TableName:              "EAVValidTable",
		KeyConditionExpression: "pk = :pk",
		ExpressionAttributeValues: map[string]any{
			":pk":   map[string]any{"S": "pk1"},
			":m":    map[string]any{"M": map[string]any{"nested": map[string]any{"N": "1"}}},
			":l":    map[string]any{"L": []any{map[string]any{"S": "a"}, map[string]any{"N": "2"}}},
			":null": map[string]any{"NULL": true},
			":bool": map[string]any{"BOOL": false},
			":ss":   map[string]any{"SS": []any{"a", "b"}},
			":ns":   map[string]any{"NS": []any{"1", "2"}},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.Query")
	w := httptest.NewRecorder()

	_ = serveEchoHandler(handler.Handler(), w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, w.Body.String(), `"Count":1`)
}
