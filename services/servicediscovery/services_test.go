package servicediscovery_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/servicediscovery"
)

func TestHandler_CreateService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		wantBody   string
		bodyRaw    []byte
		wantStatus int
	}{
		{
			name:       "success",
			body:       map[string]any{"Name": "my-service"},
			wantStatus: http.StatusOK,
			wantBody:   "Service",
		},
		{
			name:       "invalid_json",
			bodyRaw:    []byte("not-json"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_name",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var rec *httptest.ResponseRecorder
			if tt.bodyRaw != nil {
				rec = doSDRawRequest(t, h, "CreateService", tt.bodyRaw)
			} else {
				rec = doSDRequest(t, h, "CreateService", tt.body)
			}

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandler_GetService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		wantBody   string
		wantStatus int
		createSvc  bool
	}{
		{
			name:       "success",
			createSvc:  true,
			wantStatus: http.StatusOK,
			wantBody:   "Service",
		},
		{
			name:       "not_found",
			body:       map[string]any{"Id": "svc-does-not-exist"},
			wantStatus: http.StatusBadRequest,
			wantBody:   "ServiceNotFound",
		},
		{
			name:       "missing_id",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var rec *httptest.ResponseRecorder

			if tt.createSvc {
				createRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "my-svc"})
				require.Equal(t, http.StatusOK, createRec.Code)

				var createResp map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
				svcData := createResp["Service"].(map[string]any)
				svcID := svcData["Id"].(string)

				rec = doSDRequest(t, h, "GetService", map[string]any{"Id": svcID})
			} else {
				rec = doSDRequest(t, h, "GetService", tt.body)
			}

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandler_DeleteService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		wantStatus int
		createSvc  bool
	}{
		{
			name:       "success",
			createSvc:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			body:       map[string]any{"Id": "svc-does-not-exist"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_id",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var rec *httptest.ResponseRecorder

			if tt.createSvc {
				createRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "del-svc"})
				require.Equal(t, http.StatusOK, createRec.Code)

				var createResp map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
				svcData := createResp["Service"].(map[string]any)
				svcID := svcData["Id"].(string)

				rec = doSDRequest(t, h, "DeleteService", map[string]any{"Id": svcID})
			} else {
				rec = doSDRequest(t, h, "DeleteService", tt.body)
			}

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_ListServices(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-alpha"})

	rec := doSDRequest(t, h, "ListServices", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Services")
	assert.Contains(t, rec.Body.String(), "svc-alpha")
}

func TestBackend_ListServices_FilterByNamespace(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")

	opID, err := b.CreateHTTPNamespace("ns-filter", "", nil)
	require.NoError(t, err)

	op, err := b.GetOperation(opID)
	require.NoError(t, err)

	nsID := op.Targets["NAMESPACE"]

	_, err = b.CreateService("svc-in-ns", nsID, "", "", nil, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateService("svc-no-ns", "", "", "", nil, nil, nil, nil)
	require.NoError(t, err)

	all := b.ListServices(servicediscovery.ListServicesFilter{})
	assert.Len(t, all, 2)

	filtered := b.ListServices(servicediscovery.ListServicesFilter{
		NamespaceID: servicediscovery.FilterValue{Values: []string{nsID}},
	})
	assert.Len(t, filtered, 1)
	assert.Equal(t, "svc-in-ns", filtered[0].Name)
}

// TestBackend_ListServices_FilterByNamespaceARN verifies ListServices'
// NAMESPACE_ID filter accepts the namespace ARN form, not just the bare ID.
// aws-sdk-go-v2/service/servicediscovery's types.ServiceFilter doc comment:
// "NAMESPACE_ID: Specify one namespace ID or ARN. Specify the namespace ARN
// for namespaces that are shared with your Amazon Web Services account".
func TestBackend_ListServices_FilterByNamespaceARN(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")

	opID, err := b.CreateHTTPNamespace("ns-arn-filter", "", nil)
	require.NoError(t, err)

	op, err := b.GetOperation(opID)
	require.NoError(t, err)

	nsID := op.Targets["NAMESPACE"]

	ns, err := b.GetNamespace(nsID)
	require.NoError(t, err)
	require.NotEmpty(t, ns.ARN)

	_, err = b.CreateService("svc-in-ns", nsID, "", "", nil, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateService("svc-no-ns", "", "", "", nil, nil, nil, nil)
	require.NoError(t, err)

	filtered := b.ListServices(servicediscovery.ListServicesFilter{
		NamespaceID: servicediscovery.FilterValue{Values: []string{ns.ARN}},
	})
	require.Len(t, filtered, 1)
	assert.Equal(t, "svc-in-ns", filtered[0].Name)
}

// TestHandler_ServiceTagsViaListTagsForResource verifies that CreateDate is
// included in GetService/CreateService responses, that neither ever returns a
// Tags field (matching real Cloud Map's types.Service shape), and that tags
// set at creation are retrievable via ListTagsForResource.
func TestHandler_ServiceTagsViaListTagsForResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doSDRequest(t, h, "CreateService", map[string]any{
		"Name": "tagged-svc",
		"Tags": []map[string]any{
			{"Key": "version", "Value": "v2"},
		},
	})
	require.Equal(t, 200, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))

	svc := createResp["Service"].(map[string]any)
	assert.NotContains(t, svc, "Tags", "real CreateService never returns a Tags field")
	assert.NotZero(t, svc["CreateDate"], "CreateDate must be in CreateService response")

	svcID := svc["Id"].(string)
	svcARN := svc["Arn"].(string)
	getRec := doSDRequest(t, h, "GetService", map[string]any{"Id": svcID})
	require.Equal(t, 200, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))

	svcGet := getResp["Service"].(map[string]any)
	assert.NotContains(t, svcGet, "Tags", "real GetService never returns a Tags field")

	tagsRec := doSDRequest(t, h, "ListTagsForResource", map[string]any{"ResourceARN": svcARN})
	require.Equal(t, 200, tagsRec.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(tagsRec.Body.Bytes(), &tagsResp))

	tags := tagsResp["Tags"].([]any)
	assert.Len(t, tags, 1)
	assert.Equal(t, "version", tags[0].(map[string]any)["Key"])
}

// TestHandler_ListServicesNamespaceFilter verifies that ListServices filters by NAMESPACE_ID.
func TestHandler_ListServicesNamespaceFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create two namespaces.
	nsA := createNamespaceHelper(t, h, "ns-A")
	nsB := createNamespaceHelper(t, h, "ns-B")

	// Create services in each namespace.
	doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-A1", "NamespaceId": nsA})
	doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-A2", "NamespaceId": nsA})
	doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-B1", "NamespaceId": nsB})

	// List without filter - should return all 3.
	recAll := doSDRequest(t, h, "ListServices", map[string]any{})
	var respAll map[string]any
	require.NoError(t, json.Unmarshal(recAll.Body.Bytes(), &respAll))
	assert.Len(t, respAll["Services"].([]any), 3)

	// List with NAMESPACE_ID filter - should return only 2.
	recFiltered := doSDRequest(t, h, "ListServices", map[string]any{
		"Filters": []map[string]any{
			{"Name": "NAMESPACE_ID", "Values": []string{nsA}, "Condition": "EQ"},
		},
	})
	require.Equal(t, 200, recFiltered.Code)

	var respFiltered map[string]any
	require.NoError(t, json.Unmarshal(recFiltered.Body.Bytes(), &respFiltered))
	assert.Len(t, respFiltered["Services"].([]any), 2)
}

// TestHandler_CreateService_DuplicateName verifies CreateService's documented
// name-collision rule: within a DNS namespace, names that differ only by case
// collide (ServiceAlreadyExists); within an HTTP namespace, they don't
// (api_op_CreateService.go doc comment). Services in different namespaces (or
// with no namespace at all) never collide.
func TestHandler_CreateService_DuplicateName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		createNs      string // "http" = HTTP, "private" = DNS_PRIVATE
		firstName     string
		secondName    string
		wantSecondErr bool
	}{
		{
			name:          "http_namespace_exact_duplicate_collides",
			createNs:      "http",
			firstName:     "svc-x",
			secondName:    "svc-x",
			wantSecondErr: true,
		},
		{
			name:          "http_namespace_case_variant_allowed",
			createNs:      "http",
			firstName:     "svc-x",
			secondName:    "SVC-X",
			wantSecondErr: false,
		},
		{
			name:          "dns_namespace_exact_duplicate_collides",
			createNs:      "private",
			firstName:     "svc-y",
			secondName:    "svc-y",
			wantSecondErr: true,
		},
		{
			name:          "dns_namespace_case_variant_collides",
			createNs:      "private",
			firstName:     "svc-y",
			secondName:    "SVC-Y",
			wantSecondErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var nsID string
			if tt.createNs == "private" {
				nsID = createPrivateDNSNamespaceHelper(t, h, "ns-dup-"+tt.name)
			} else {
				nsID = createNamespaceHelper(t, h, "ns-dup-"+tt.name)
			}

			rec1 := doSDRequest(t, h, "CreateService", map[string]any{"Name": tt.firstName, "NamespaceId": nsID})
			require.Equal(t, http.StatusOK, rec1.Code, "first create should succeed: %s", rec1.Body.String())

			rec2 := doSDRequest(t, h, "CreateService", map[string]any{"Name": tt.secondName, "NamespaceId": nsID})
			if tt.wantSecondErr {
				assert.Equal(t, http.StatusBadRequest, rec2.Code)
				assert.Contains(t, rec2.Body.String(), "ServiceAlreadyExists")
			} else {
				assert.Equal(t, http.StatusOK, rec2.Code, "second create should succeed: %s", rec2.Body.String())
			}
		})
	}
}

// TestHandler_CreateService_NoNamespaceNeverCollides verifies that services
// with no NamespaceId are never subject to the name-collision check (there is
// no namespace to scope uniqueness to).
func TestHandler_CreateService_NoNamespaceNeverCollides(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec1 := doSDRequest(t, h, "CreateService", map[string]any{"Name": "no-ns-svc"})
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doSDRequest(t, h, "CreateService", map[string]any{"Name": "no-ns-svc"})
	assert.Equal(t, http.StatusOK, rec2.Code, "services with no namespace never collide")
}

// TestHandler_ListServicesResourceOwnerFilter verifies the RESOURCE_OWNER
// filter: this backend is single-account, so every service is always "SELF"
// -- SELF (or no filter) matches everything, OTHER_ACCOUNTS matches nothing.
func TestHandler_ListServicesResourceOwnerFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-owner-test"})

	tests := []struct {
		name    string
		values  []string
		wantLen int
	}{
		{name: "self_matches_all", values: []string{"SELF"}, wantLen: 1},
		{name: "other_accounts_matches_none", values: []string{"OTHER_ACCOUNTS"}, wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doSDRequest(t, h, "ListServices", map[string]any{
				"Filters": []map[string]any{
					{"Name": "RESOURCE_OWNER", "Values": tt.values},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out["Services"].([]any), tt.wantLen)
		})
	}
}

// createPrivateDNSNamespaceHelper creates a private DNS namespace and returns its ID.
func createPrivateDNSNamespaceHelper(t *testing.T, h *servicediscovery.Handler, name string) string {
	t.Helper()

	nsRec := doSDRequest(t, h, "CreatePrivateDnsNamespace", map[string]any{"Name": name, "Vpc": "vpc-1"})
	require.Equal(t, http.StatusOK, nsRec.Code)

	var nsResp map[string]any
	require.NoError(t, json.Unmarshal(nsRec.Body.Bytes(), &nsResp))

	opID := nsResp["OperationId"].(string)
	opRec := doSDRequest(t, h, "GetOperation", map[string]any{"OperationId": opID})

	var opResp map[string]any
	require.NoError(t, json.Unmarshal(opRec.Body.Bytes(), &opResp))

	return opResp["Operation"].(map[string]any)["Targets"].(map[string]any)["NAMESPACE"].(string)
}

// TestCascadeDeleteUsesCorrectPrefix verifies two things: (1) real
// Cloud Map's DeleteService fails while the service still has registered
// instances (no silent auto-deregister), and (2) once the blocking instance is
// deregistered and the delete retried, it does NOT incorrectly touch instances
// belonging to a different service with a common ID prefix.
func TestCascadeDeleteUsesCorrectPrefix(t *testing.T) {
	t.Parallel()

	b, h := newBackendAndHandler(t)

	// Seed two services with IDs that share a prefix.
	svc1 := servicediscovery.NewServiceForTest("svc-0000001", "svc-one", "")
	svc2 := servicediscovery.NewServiceForTest("svc-00000010", "svc-ten", "") // prefix of svc1

	servicediscovery.AddServiceInternal(b, svc1)
	servicediscovery.AddServiceInternal(b, svc2)

	// Seed instance under svc-0000001 only.
	inst1 := servicediscovery.NewInstanceForTest("i-1", "svc-0000001", map[string]string{})
	servicediscovery.AddInstanceInternal(b, inst1)

	// Seed instance under svc-00000010.
	inst2 := servicediscovery.NewInstanceForTest("i-2", "svc-00000010", map[string]string{})
	servicediscovery.AddInstanceInternal(b, inst2)

	assert.Equal(t, 2, servicediscovery.InstanceCount(b))

	// DeleteService must fail (ResourceInUse) while svc-0000001 still has a
	// registered instance -- matching real AWS, which never auto-deregisters.
	rec := doSDRequest(t, h, "DeleteService", map[string]any{"Id": "svc-0000001"})
	require.Equal(t, 400, rec.Code)
	assert.Equal(t, 2, servicediscovery.InstanceCount(b), "a rejected delete must not remove any instance")

	// Deregister the blocking instance, then retry the delete.
	deregRec := doSDRequest(t, h, "DeregisterInstance", map[string]any{
		"ServiceId": "svc-0000001", "InstanceId": "i-1",
	})
	require.Equal(t, 200, deregRec.Code)

	rec = doSDRequest(t, h, "DeleteService", map[string]any{"Id": "svc-0000001"})
	require.Equal(t, 200, rec.Code)

	// svc-00000010's instance should survive despite the shared ID prefix.
	assert.Equal(t, 1, servicediscovery.InstanceCount(b), "instance from svc-00000010 must not be deleted")
}

// TestUpdateService_ReturnsOperationId verifies UpdateService returns an OperationId,
// not a Service body, matching real AWS Cloud Map behavior.
func TestUpdateService_ReturnsOperationId(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantCode  int
		wantOpID  bool
		wantNoSvc bool
	}{
		{
			name:      "success_returns_operation_id",
			wantCode:  http.StatusOK,
			wantOpID:  true,
			wantNoSvc: true,
		},
		{
			name:     "not_found_returns_error",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			switch tt.name {
			case "success_returns_operation_id":
				createRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-parity"})
				require.Equal(t, http.StatusOK, createRec.Code)
				var created map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
				svcID := created["Service"].(map[string]any)["Id"].(string)

				rec := doSDRequest(t, h, "UpdateService", map[string]any{
					"Id":      svcID,
					"Service": map[string]any{"Description": "updated"},
				})
				require.Equal(t, tt.wantCode, rec.Code)

				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				if tt.wantOpID {
					assert.NotEmpty(t, out["OperationId"], "should contain OperationId")
				}
				if tt.wantNoSvc {
					assert.Nil(t, out["Service"], "should NOT contain Service body")
				}
			case "not_found_returns_error":
				rec := doSDRequest(t, h, "UpdateService", map[string]any{
					"Id":      "nonexistent",
					"Service": map[string]any{"Description": "x"},
				})
				assert.Equal(t, tt.wantCode, rec.Code)
			}
		})
	}
}

// TestService_InstanceCount verifies GetService and ListServices include
// an accurate InstanceCount field.
func TestService_InstanceCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		instancesBefore int
		wantCount       int
	}{
		{name: "empty_service_count_zero", instancesBefore: 0, wantCount: 0},
		{name: "two_instances_count_two", instancesBefore: 2, wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createSvcRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-count"})
			require.Equal(t, http.StatusOK, createSvcRec.Code)
			var svcOut map[string]any
			require.NoError(t, json.Unmarshal(createSvcRec.Body.Bytes(), &svcOut))
			svcID := svcOut["Service"].(map[string]any)["Id"].(string)

			for i := range tt.instancesBefore {
				rec := doSDRequest(t, h, "RegisterInstance", map[string]any{
					"ServiceId":  svcID,
					"InstanceId": fmt.Sprintf("inst-%d", i),
					"Attributes": map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.1"},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			// Verify via GetService
			getRec := doSDRequest(t, h, "GetService", map[string]any{"Id": svcID})
			require.Equal(t, http.StatusOK, getRec.Code)
			var getOut map[string]any
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
			svc := getOut["Service"].(map[string]any)
			assert.Equal(t, tt.wantCount, int(svc["InstanceCount"].(float64)), "GetService InstanceCount")

			// Verify via ListServices
			listRec := doSDRequest(t, h, "ListServices", map[string]any{})
			require.Equal(t, http.StatusOK, listRec.Code)
			var listOut map[string]any
			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
			svcs := listOut["Services"].([]any)
			require.Len(t, svcs, 1)
			gotListCount := int(svcs[0].(map[string]any)["InstanceCount"].(float64))
			assert.Equal(t, tt.wantCount, gotListCount, "ListServices InstanceCount")
		})
	}
}

// TestListServices_Pagination verifies NextToken/MaxResults pagination on ListServices.
func TestListServices_Pagination(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")
	servicediscovery.SetDeterministicIDs(b)
	h := servicediscovery.NewHandler(b)

	for i := range 4 {
		rec := doSDRequest(t, h, "CreateService", map[string]any{
			"Name": fmt.Sprintf("svc-%02d", i),
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	tests := []struct {
		req           map[string]any
		name          string
		wantLen       int
		wantNextToken bool
	}{
		{
			name:          "no_limit_returns_all",
			req:           map[string]any{},
			wantLen:       4,
			wantNextToken: false,
		},
		{
			name:          "page1_two_items",
			req:           map[string]any{"MaxResults": 2},
			wantLen:       2,
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doSDRequest(t, h, "ListServices", tt.req)
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			svcs := out["Services"].([]any)
			assert.Len(t, svcs, tt.wantLen)
			if tt.wantNextToken {
				assert.NotEmpty(t, out["NextToken"])
			} else {
				assert.Empty(t, out["NextToken"])
			}
		})
	}
}

// TestUpdateService_CreatesOperation verifies UpdateService creates a retrievable operation.
func TestUpdateService_CreatesOperation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-op-check"})
	require.Equal(t, http.StatusOK, createRec.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	svcID := created["Service"].(map[string]any)["Id"].(string)

	updateRec := doSDRequest(t, h, "UpdateService", map[string]any{
		"Id":      svcID,
		"Service": map[string]any{"Description": "desc-v2"},
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateOut map[string]string
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateOut))
	opID := updateOut["OperationId"]
	require.NotEmpty(t, opID)

	getOpRec := doSDRequest(t, h, "GetOperation", map[string]any{"OperationId": opID})
	require.Equal(t, http.StatusOK, getOpRec.Code)
	var opOut map[string]any
	require.NoError(t, json.Unmarshal(getOpRec.Body.Bytes(), &opOut))
	op := opOut["Operation"].(map[string]any)
	assert.Equal(t, "UPDATE_SERVICE", op["Type"])
	assert.Equal(t, "SUCCESS", op["Status"])
}

// TestHandler_UpdateService tests UpdateService.
func TestHandler_UpdateService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantKey  string
		wantCode int
	}{
		{name: "success", wantCode: http.StatusOK, wantKey: "OperationId"},
		{name: "missing_id", wantCode: http.StatusBadRequest},
		{name: "not_found", wantCode: http.StatusBadRequest},
		{name: "invalid_json", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			switch tt.name {
			case "success":
				createRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "my-svc"})
				var out map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &out))
				svcID := out["Service"].(map[string]any)["Id"].(string)
				rec := doSDRequest(t, h, "UpdateService", map[string]any{
					"Id":      svcID,
					"Service": map[string]any{"Description": "updated description"},
				})
				assert.Equal(t, tt.wantCode, rec.Code)
				assert.Contains(t, rec.Body.String(), tt.wantKey)

			case "missing_id":
				rec := doSDRequest(t, h, "UpdateService", map[string]any{
					"Service": map[string]any{"Description": "updated"},
				})
				assert.Equal(t, tt.wantCode, rec.Code)

			case "not_found":
				rec := doSDRequest(t, h, "UpdateService", map[string]any{
					"Id":      "no-such-svc",
					"Service": map[string]any{"Description": "updated"},
				})
				assert.Equal(t, tt.wantCode, rec.Code)

			case "invalid_json":
				rec := doSDRawRequest(t, h, "UpdateService", []byte("{bad"))
				assert.Equal(t, tt.wantCode, rec.Code)
			}
		})
	}
}

// TestHandler_ServiceAttributes tests GetServiceAttributes, UpdateServiceAttributes, DeleteServiceAttributes.
func TestHandler_ServiceAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "update_and_get", wantCode: http.StatusOK},
		{name: "get_not_found", wantCode: http.StatusBadRequest},
		{name: "get_missing_service_id", wantCode: http.StatusBadRequest},
		{name: "update_missing_id", wantCode: http.StatusBadRequest},
		{name: "update_service_not_found", wantCode: http.StatusBadRequest},
		{name: "delete_and_verify", wantCode: http.StatusOK},
		{name: "delete_missing_id", wantCode: http.StatusBadRequest},
		{name: "delete_not_found_service", wantCode: http.StatusBadRequest},
		{name: "delete_missing_attributes", wantCode: http.StatusBadRequest},
		{name: "get_before_update", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Helper to create a service and return its ID and ARN.
			createSvc := func() (string, string) {
				t.Helper()
				rec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "attrs-svc"})
				require.Equal(t, http.StatusOK, rec.Code)
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				svc := out["Service"].(map[string]any)

				return svc["Id"].(string), svc["Arn"].(string)
			}

			switch tt.name {
			case "update_and_get":
				svcID, svcARN := createSvc()
				updateRec := doSDRequest(t, h, "UpdateServiceAttributes", map[string]any{
					"ServiceId":  svcID,
					"Attributes": map[string]string{"env": "prod", "version": "1.0"},
				})
				assert.Equal(t, http.StatusOK, updateRec.Code, "update should succeed: %s", updateRec.Body.String())

				getRec := doSDRequest(t, h, "GetServiceAttributes", map[string]any{"ServiceId": svcID})
				assert.Equal(t, http.StatusOK, getRec.Code)

				var getOut map[string]any
				require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
				sa := getOut["ServiceAttributes"].(map[string]any)
				attrs := sa["Attributes"].(map[string]any)
				assert.Equal(t, "prod", attrs["env"])
				assert.Equal(t, "1.0", attrs["version"])
				assert.Equal(t, svcARN, sa["ServiceArn"])

			case "get_not_found":
				_, svcARN := createSvc()
				// Update then get a different ID. ServiceId accepts an ARN value too
				// (real AWS: "The ID or Amazon Resource Name (ARN) of the service").
				doSDRequest(t, h, "UpdateServiceAttributes", map[string]any{
					"ServiceId":  svcARN,
					"Attributes": map[string]string{"env": "prod"},
				})
				// No attributes for another service ID
				getRec := doSDRequest(t, h, "GetServiceAttributes", map[string]any{"ServiceId": "no-such-svc"})
				assert.Equal(t, tt.wantCode, getRec.Code)

			case "get_missing_service_id":
				getRec := doSDRequest(t, h, "GetServiceAttributes", map[string]any{})
				assert.Equal(t, tt.wantCode, getRec.Code)

			case "update_missing_id":
				updateRec := doSDRequest(t, h, "UpdateServiceAttributes", map[string]any{
					"Attributes": map[string]string{"env": "prod"},
				})
				assert.Equal(t, tt.wantCode, updateRec.Code)

			case "update_service_not_found":
				updateRec := doSDRequest(t, h, "UpdateServiceAttributes", map[string]any{
					"ServiceId":  "arn:aws:servicediscovery:us-east-1:000000000000:service/svc-99999999",
					"Attributes": map[string]string{"env": "prod"},
				})
				assert.Equal(t, tt.wantCode, updateRec.Code)

			case "delete_and_verify":
				svcID, _ := createSvc()
				doSDRequest(t, h, "UpdateServiceAttributes", map[string]any{
					"ServiceId":  svcID,
					"Attributes": map[string]string{"env": "prod", "keep": "yes"},
				})
				// DeleteServiceAttributes' Attributes field names which keys to
				// remove (real AWS: "A list of keys corresponding to each
				// attribute that you want to delete") -- it must NOT wipe every
				// attribute on the service.
				deleteRec := doSDRequest(t, h, "DeleteServiceAttributes", map[string]any{
					"ServiceId":  svcID,
					"Attributes": []string{"env"},
				})
				assert.Equal(t, http.StatusOK, deleteRec.Code)

				getRec := doSDRequest(t, h, "GetServiceAttributes", map[string]any{"ServiceId": svcID})
				require.Equal(t, http.StatusOK, getRec.Code)

				var getOut map[string]any
				require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
				attrs := getOut["ServiceAttributes"].(map[string]any)["Attributes"].(map[string]any)
				assert.NotContains(t, attrs, "env")
				assert.Equal(t, "yes", attrs["keep"])

			case "delete_missing_id":
				deleteRec := doSDRequest(t, h, "DeleteServiceAttributes", map[string]any{"Attributes": []string{"env"}})
				assert.Equal(t, tt.wantCode, deleteRec.Code)

			case "delete_not_found_service":
				deleteRec := doSDRequest(t, h, "DeleteServiceAttributes", map[string]any{
					"ServiceId":  "no-such-svc",
					"Attributes": []string{"env"},
				})
				assert.Equal(t, tt.wantCode, deleteRec.Code)

			case "delete_missing_attributes":
				svcID, _ := createSvc()
				deleteRec := doSDRequest(t, h, "DeleteServiceAttributes", map[string]any{"ServiceId": svcID})
				assert.Equal(t, tt.wantCode, deleteRec.Code)

			case "get_before_update":
				svcID, _ := createSvc()
				// GetServiceAttributes before any UpdateServiceAttributes should fail
				getRec := doSDRequest(t, h, "GetServiceAttributes", map[string]any{"ServiceId": svcID})
				assert.Equal(t, tt.wantCode, getRec.Code)
			}
		})
	}
}

// TestHandler_UpdateServiceAttributesQuota verifies the shape constraints from
// the botocore model (servicediscovery/2017-03-14/service-2.json): shape
// ServiceAttributesMap{max:30,min:1}, ServiceAttributeKey{max:255},
// ServiceAttributeValue{max:1024}.
func TestHandler_UpdateServiceAttributesQuota(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attrs    map[string]string
		name     string
		wantType string
		presetTo int
		wantCode int
	}{
		{
			name:     "within_limit_accepted",
			attrs:    map[string]string{"env": "prod"},
			wantCode: http.StatusOK,
		},
		{
			name:     "empty_map_rejected",
			attrs:    map[string]string{},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidInput",
		},
		{
			name:     "key_too_long_rejected",
			attrs:    map[string]string{strings.Repeat("k", 256): "v"},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidInput",
		},
		{
			name:     "value_too_long_rejected",
			attrs:    map[string]string{"k": strings.Repeat("v", 1025)},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidInput",
		},
		{
			name:     "count_at_limit_accepted",
			presetTo: 29,
			attrs:    map[string]string{"new-attr": "v"},
			wantCode: http.StatusOK,
		},
		{
			name:     "count_over_limit_rejected",
			presetTo: 30,
			attrs:    map[string]string{"one-too-many": "v"},
			wantCode: http.StatusBadRequest,
			wantType: "ServiceAttributesLimitExceededException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "quota-svc"})
			require.Equal(t, http.StatusOK, createRec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			svcID := createResp["Service"].(map[string]any)["Id"].(string)

			if tt.presetTo > 0 {
				preset := make(map[string]string, tt.presetTo)
				for i := range tt.presetTo {
					preset[fmt.Sprintf("preset-%d", i)] = "v"
				}

				presetRec := doSDRequest(t, h, "UpdateServiceAttributes", map[string]any{
					"ServiceId":  svcID,
					"Attributes": preset,
				})
				require.Equal(t, http.StatusOK, presetRec.Code, presetRec.Body.String())
			}

			rec := doSDRequest(t, h, "UpdateServiceAttributes", map[string]any{
				"ServiceId":  svcID,
				"Attributes": tt.attrs,
			})
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())

			if tt.wantType != "" {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, tt.wantType, errResp["__type"])
			}
		})
	}
}

// TestHandler_CreateServiceInvalidEnums verifies that CreateService rejects
// RoutingPolicy/DnsRecords[].Type/HealthCheckConfig.Type values outside the
// closed enums documented in the botocore model (shapes RoutingPolicy{enum:
// [MULTIVALUE,WEIGHTED]}, RecordType{enum:[SRV,A,AAAA,CNAME]}, HealthCheckType
// {enum:[HTTP,HTTPS,TCP]}), matching aws-sdk-go-v2's closed string enums
// (types/enums.go).
func TestHandler_CreateServiceInvalidEnums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "valid_routing_policy_and_record_type_accepted",
			body: map[string]any{
				"Name": "svc-valid-enum",
				"DnsConfig": map[string]any{
					"RoutingPolicy": "WEIGHTED",
					"DnsRecords":    []map[string]any{{"Type": "A", "TTL": 60}},
				},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "invalid_routing_policy_rejected",
			body: map[string]any{
				"Name": "svc-bad-routing",
				"DnsConfig": map[string]any{
					"RoutingPolicy": "BOGUS",
					"DnsRecords":    []map[string]any{{"Type": "A", "TTL": 60}},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "invalid_dns_record_type_rejected",
			body: map[string]any{
				"Name": "svc-bad-record-type",
				"DnsConfig": map[string]any{
					"RoutingPolicy": "WEIGHTED",
					"DnsRecords":    []map[string]any{{"Type": "BOGUS", "TTL": 60}},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "invalid_health_check_type_rejected",
			body: map[string]any{
				"Name": "svc-bad-health-check",
				"HealthCheckConfig": map[string]any{
					"Type": "BOGUS",
				},
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doSDRequest(t, h, "CreateService", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
		})
	}
}
