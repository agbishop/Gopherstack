// Package cloudfrontkeyvaluestore implements the AWS CloudFront KeyValueStore
// data-plane API (GetKey/PutKey/DeleteKey/ListKeys/UpdateKeys/
// DescribeKeyValueStore). AWS models this as a separate SDK client and
// protocol from the main cloudfront.Client -- see Handler's doc comment for
// why this package exists instead of living inside services/cloudfront.
package cloudfrontkeyvaluestore

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	cloudfrontbackend "github.com/blackbirdworks/gopherstack/services/cloudfront"
)

const (
	// kvsMatchPriority sits alongside apigatewaymanagementapi's 87: a literal,
	// unversioned REST path with no header/host to disambiguate on, but no
	// collision risk since no other service in this repo routes under
	// "/key-value-stores/" (verified by grep).
	kvsMatchPriority = 87
	kvsPathPrefix    = "/key-value-stores/"

	ifMatchHeader = "If-Match"
	etagHeader    = "ETag"

	opGetKey                = "GetKey"
	opPutKey                = "PutKey"
	opDeleteKey             = "DeleteKey"
	opListKeys              = "ListKeys"
	opUpdateKeys            = "UpdateKeys"
	opDescribeKeyValueStore = "DescribeKeyValueStore"

	defaultListKeysLimit = 10
	maxListKeysLimit     = 50
)

// Handler handles HTTP requests for CloudFront KeyValueStore data-plane
// operations.
//
// Handler intentionally owns no independent state and therefore does not
// implement the Snapshot/Restore persistable shape that cli.go's
// setupPersistence auto-registers. It is wired (in cli.go's
// wireCloudFrontKeyValueStore) directly to the CloudFront service's
// *InMemoryBackend, which already owns and persists every KVS store's
// metadata (keyValueStores) and key/value data
// (keyValueStoreData/keyValueDataETags, see services/cloudfront/persistence.go)
// -- the same real state services/cloudfront's own KVS *control-plane* ops
// (CreateKeyValueStore etc.) act on. Implementing Snapshot/Restore here would
// register a second entry that duplicates and re-restores that same shared
// backend object. Mirrors services/dynamodbstreams's relationship to
// services/dynamodb (see that package's handler.go doc comment and
// persistence_test.go for the guard test on this invariant).
type Handler struct {
	Backend *cloudfrontbackend.InMemoryBackend
}

// NewHandler creates a new CloudFront KeyValueStore handler with the given backend.
func NewHandler(backend *cloudfrontbackend.InMemoryBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service identifier, matching the real SDK's ServiceID
// ("CloudFront KeyValueStore", cloudfrontkeyvaluestore@v1.15.4 api_client.go).
func (h *Handler) Name() string { return "CloudFront KeyValueStore" }

// GetSupportedOperations returns the operations this handler supports.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opDeleteKey,
		opDescribeKeyValueStore,
		opGetKey,
		opListKeys,
		opPutKey,
		opUpdateKeys,
	}
}

// ChaosServiceName returns the lowercase AWS-style service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "cloudfrontkeyvaluestore" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles. KVS data is not
// region-partitioned in this emulator (mirrors the real service's per-store,
// not per-region, scoping), so this always returns empty.
func (h *Handler) ChaosRegions() []string { return []string{} }

// RouteMatcher matches the real client's unversioned "/key-value-stores/..."
// path family -- distinct from services/cloudfront's "/2020-05-31/..."
// prefix, which is exactly why these ops need their own service (see
// services/cloudfront/PARITY.md).
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().URL.Path, kvsPathPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return kvsMatchPriority }

// ExtractOperation returns the operation name for the request's method+path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _, _ := route(c.Request())

	return op
}

// ExtractResource returns the target Key Value Store's ARN.
func (h *Handler) ExtractResource(c *echo.Context) string {
	_, kvsARN, _ := route(c.Request())

	return kvsARN
}

// rawPathSegments splits the raw (or decoded) URL path into non-empty
// segments, URL-decoding each segment individually so that the percent-encoded
// ARN and key path params (which themselves may contain "/") are preserved as
// single segments rather than fragmented by a naive split. Same approach as
// services/grafana and services/s3tables's helper of the same name -- see
// those packages' handler.go for the "ARN-in-path route-matching trap" this
// guards against.
func rawPathSegments(r *http.Request) []string {
	rawPath := r.URL.EscapedPath()
	rawPath = strings.TrimPrefix(rawPath, "/")

	parts := strings.Split(rawPath, "/")
	segments := make([]string, 0, len(parts))

	for _, p := range parts {
		if p == "" {
			continue
		}

		decoded, err := url.PathUnescape(p)
		if err != nil {
			decoded = p
		}

		segments = append(segments, decoded)
	}

	return segments
}

// route resolves an operation name, KVS ARN, and key (empty for
// store-level/list/batch ops) from a request's method and path. Mirrors each
// op's httpbinding.SplitURI template in cloudfrontkeyvaluestore@v1.15.4
// serializers.go:
//
//	GET    /key-value-stores/{KvsARN}             DescribeKeyValueStore
//	GET    /key-value-stores/{KvsARN}/keys        ListKeys
//	POST   /key-value-stores/{KvsARN}/keys        UpdateKeys
//	GET    /key-value-stores/{KvsARN}/keys/{Key}  GetKey
//	PUT    /key-value-stores/{KvsARN}/keys/{Key}  PutKey
//	DELETE /key-value-stores/{KvsARN}/keys/{Key}  DeleteKey
const (
	// segCountStore is "/key-value-stores/{KvsARN}".
	segCountStore = 2
	// segCountKeys is "/key-value-stores/{KvsARN}/keys".
	segCountKeys = 3
	// segCountKey is "/key-value-stores/{KvsARN}/keys/{Key}".
	segCountKey = 4
)

func route(r *http.Request) (string, string, string) {
	segs := rawPathSegments(r)
	if len(segs) < segCountStore || segs[0] != "key-value-stores" {
		return "", "", ""
	}

	kvsARN := segs[1]

	switch len(segs) {
	case segCountStore:
		if r.Method == http.MethodGet {
			return opDescribeKeyValueStore, kvsARN, ""
		}
	case segCountKeys:
		if segs[2] != "keys" {
			return "", "", ""
		}

		switch r.Method {
		case http.MethodGet:
			return opListKeys, kvsARN, ""
		case http.MethodPost:
			return opUpdateKeys, kvsARN, ""
		}
	case segCountKey:
		if segs[2] != "keys" {
			return "", "", ""
		}

		key := segs[3]

		switch r.Method {
		case http.MethodGet:
			return opGetKey, kvsARN, key
		case http.MethodPut:
			return opPutKey, kvsARN, key
		case http.MethodDelete:
			return opDeleteKey, kvsARN, key
		}
	}

	return "", "", ""
}

// Handler returns the Echo handler function for CloudFront KeyValueStore requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		op, kvsARN, key := route(c.Request())
		if op == "" {
			return writeAWSError(c, http.StatusNotFound, exceptionResourceNotFound, "no matching operation")
		}

		log.DebugContext(ctx, "CloudFront KeyValueStore request", "operation", op, "kvsArn", kvsARN)

		kvs, err := h.Backend.GetKeyValueStore(kvsARN)
		if err != nil {
			status, exType := classifyError(err)

			return writeAWSError(c, status, exType, err.Error())
		}

		switch op {
		case opDescribeKeyValueStore:
			return h.handleDescribeKeyValueStore(c, kvs)
		case opGetKey:
			return h.handleGetKey(c, kvs, key)
		case opPutKey:
			return h.handlePutKey(c, kvs, key)
		case opDeleteKey:
			return h.handleDeleteKey(c, kvs, key)
		case opListKeys:
			return h.handleListKeys(c, kvs)
		case opUpdateKeys:
			return h.handleUpdateKeys(c, kvs)
		default:
			return writeAWSError(c, http.StatusNotFound, exceptionResourceNotFound, "no matching operation")
		}
	}
}

// storeStats computes the aggregate ItemCount/TotalSizeInBytes every mutating
// and read op reports alongside its result. TotalSizeInBytes sums each item's
// key+value byte length -- a reasonable, deterministic approximation of AWS's
// real accounting (which also counts internal per-item overhead that isn't
// documented and can't be replicated exactly); see PARITY.md.
// computeStats sums each item's key+value byte length -- a reasonable,
// deterministic approximation of AWS's real accounting (which also counts
// internal per-item overhead that isn't documented and can't be replicated
// exactly); see PARITY.md.
func computeStats(items []*cloudfrontbackend.KVSItem) (int32, int64) {
	var totalBytes int64
	for _, item := range items {
		totalBytes += int64(len(item.Key)) + int64(len(item.Value))
	}

	return int32(len(items)), totalBytes //nolint:gosec // G115: emulator item counts never approach int32 range
}

// storeStats computes the aggregate ItemCount/TotalSizeInBytes every mutating
// and read op reports alongside its result.
func (h *Handler) storeStats(kvsID string) (int32, int64) {
	items, _, err := h.Backend.ListKVSValues(kvsID)
	if err != nil {
		return 0, 0
	}

	return computeStats(items)
}

func (h *Handler) handleDescribeKeyValueStore(c *echo.Context, kvs *cloudfrontbackend.KeyValueStore) error {
	// The real DescribeKeyValueStore's ETag is the *data-plane* ETag used for
	// If-Match on Put/Delete/UpdateKeys (PutKeyInput's IfMatch doc comment says
	// so explicitly), not the KeyValueStore resource's own control-plane ETag
	// -- ListKVSValues is the one Backend method that returns it.
	items, dataETag, err := h.Backend.ListKVSValues(kvs.ID)
	if err != nil {
		status, exType := classifyError(err)

		return writeAWSError(c, status, exType, err.Error())
	}

	itemCount, totalBytes := computeStats(items)

	c.Response().Header().Set(etagHeader, dataETag)

	return writeJSON(c, describeKeyValueStoreOutput{
		KvsARN:           kvs.ARN,
		Status:           kvs.Status,
		Created:          epochSeconds(kvs.CreatedTime),
		LastModified:     epochSeconds(kvs.LastModifiedTime),
		ItemCount:        itemCount,
		TotalSizeInBytes: totalBytes,
	})
}

func (h *Handler) handleGetKey(c *echo.Context, kvs *cloudfrontbackend.KeyValueStore, key string) error {
	value, _, err := h.Backend.GetKVSValue(kvs.ID, key)
	if err != nil {
		status, exType := classifyError(err)

		return writeAWSError(c, status, exType, err.Error())
	}

	itemCount, totalBytes := h.storeStats(kvs.ID)

	return writeJSON(c, getKeyOutput{
		Key:              key,
		Value:            value,
		ItemCount:        itemCount,
		TotalSizeInBytes: totalBytes,
	})
}

func (h *Handler) handlePutKey(c *echo.Context, kvs *cloudfrontbackend.KeyValueStore, key string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeAWSError(c, http.StatusBadRequest, exceptionValidation, "failed to read request body")
	}

	var req putKeyInput
	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
			return writeAWSError(c, http.StatusBadRequest, exceptionValidation, "invalid JSON body")
		}
	}

	ifMatch := c.Request().Header.Get(ifMatchHeader)
	if ok, ifMatchErr := requireIfMatch(c, ifMatch); !ok {
		return ifMatchErr
	}

	newETag, putErr := h.Backend.PutKVSValue(kvs.ID, key, req.Value, ifMatch)
	if putErr != nil {
		status, exType := classifyError(putErr)

		return writeAWSError(c, status, exType, putErr.Error())
	}

	itemCount, totalBytes := h.storeStats(kvs.ID)

	c.Response().Header().Set(etagHeader, newETag)

	return writeJSON(c, mutateKeyOutput{ItemCount: itemCount, TotalSizeInBytes: totalBytes})
}

func (h *Handler) handleDeleteKey(c *echo.Context, kvs *cloudfrontbackend.KeyValueStore, key string) error {
	ifMatch := c.Request().Header.Get(ifMatchHeader)
	if ok, ifMatchErr := requireIfMatch(c, ifMatch); !ok {
		return ifMatchErr
	}

	newETag, delErr := h.Backend.DeleteKVSValue(kvs.ID, key, ifMatch)
	if delErr != nil {
		status, exType := classifyError(delErr)

		return writeAWSError(c, status, exType, delErr.Error())
	}

	itemCount, totalBytes := h.storeStats(kvs.ID)

	c.Response().Header().Set(etagHeader, newETag)

	return writeJSON(c, mutateKeyOutput{ItemCount: itemCount, TotalSizeInBytes: totalBytes})
}

func (h *Handler) handleListKeys(c *echo.Context, kvs *cloudfrontbackend.KeyValueStore) error {
	limit, limitErr := parseMaxResults(c.Request().URL.Query().Get("MaxResults"))
	if limitErr != nil {
		status, exType := classifyError(limitErr)

		return writeAWSError(c, status, exType, limitErr.Error())
	}

	items, _, err := h.Backend.ListKVSValues(kvs.ID)
	if err != nil {
		status, exType := classifyError(err)

		return writeAWSError(c, status, exType, err.Error())
	}

	pg := page.New(items, c.Request().URL.Query().Get("NextToken"), limit, defaultListKeysLimit)

	out := make([]keyValuePairJSON, 0, len(pg.Data))
	for _, item := range pg.Data {
		out = append(out, keyValuePairJSON{Key: item.Key, Value: item.Value})
	}

	return writeJSON(c, listKeysOutput{Items: out, NextToken: pg.Next})
}

func (h *Handler) handleUpdateKeys(c *echo.Context, kvs *cloudfrontbackend.KeyValueStore) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeAWSError(c, http.StatusBadRequest, exceptionValidation, "failed to read request body")
	}

	var req updateKeysInput
	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
			return writeAWSError(c, http.StatusBadRequest, exceptionValidation, "invalid JSON body")
		}
	}

	puts := make([]*cloudfrontbackend.KVSItem, 0, len(req.Puts))
	for _, p := range req.Puts {
		puts = append(puts, &cloudfrontbackend.KVSItem{Key: p.Key, Value: p.Value})
	}

	deletes := make([]string, 0, len(req.Deletes))
	for _, d := range req.Deletes {
		deletes = append(deletes, d.Key)
	}

	ifMatch := c.Request().Header.Get(ifMatchHeader)
	if ok, ifMatchErr := requireIfMatch(c, ifMatch); !ok {
		return ifMatchErr
	}

	newETag, updateErr := h.Backend.UpdateKVSValues(kvs.ID, ifMatch, puts, deletes)
	if updateErr != nil {
		status, exType := classifyError(updateErr)

		return writeAWSError(c, status, exType, updateErr.Error())
	}

	itemCount, totalBytes := h.storeStats(kvs.ID)

	c.Response().Header().Set(etagHeader, newETag)

	return writeJSON(c, mutateKeyOutput{ItemCount: itemCount, TotalSizeInBytes: totalBytes})
}

// requireIfMatch enforces IfMatch as a required input member on
// PutKey/DeleteKey/UpdateKeys (validators.go's validateOp{PutKey,DeleteKey,
// UpdateKeys}Input in cloudfrontkeyvaluestore@v1.15.4 -- each adds
// smithy.NewErrParamRequired("IfMatch") when it's unset). Without this, an
// absent If-Match header falls through to the backend's ETag comparison,
// which treats "" as "skip the check" rather than "reject the request" --
// silently bypassing the concurrency guard entirely. Returns false only
// after already writing the ValidationException response.
func requireIfMatch(c *echo.Context, ifMatch string) (bool, error) {
	if ifMatch != "" {
		return true, nil
	}

	status, exType := classifyError(errIfMatchRequired)

	return false, writeAWSError(c, status, exType, errIfMatchRequired.Error())
}

// parseMaxResults validates the MaxResults query parameter against the real
// API's documented bounds (default 10, max 50 -- ListKeysInput's doc comment
// in cloudfrontkeyvaluestore@v1.15.4 api_op_ListKeys.go). Zero means unset.
func parseMaxResults(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > maxListKeysLimit {
		return 0, errInvalidMaxResults
	}

	return n, nil
}

// epochSeconds converts a services/cloudfront RFC3339 timestamp string (the
// KeyValueStore struct's own wire format for its REST-XML API) into the epoch
// seconds this protocol's Timestamp shape requires.
func epochSeconds(rfc3339 string) float64 {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return 0
	}

	return float64(t.Unix())
}

// writeJSON marshals v as an HTTP 200 response body -- the status every op in
// this package returns on success (DeleteKey/UpdateKeys included: unlike a
// typical REST delete, their outputs carry a body, so 204 No Content is not
// an option).
func writeJSON(c *echo.Context, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return writeAWSError(c, http.StatusInternalServerError, exceptionInternalServer, "response marshal failed")
	}

	return c.JSONBlob(http.StatusOK, body)
}

// writeAWSError writes a standard restJson1 error response: the
// X-Amzn-ErrorType header plus a {"message": "..."} body.
func writeAWSError(c *echo.Context, status int, exceptionType, message string) error {
	c.Response().Header().Set(errorTypeHeader, exceptionType)

	return c.JSON(status, awsErrorBody{Message: message})
}
