package kms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	opCreateGrant                         = "CreateGrant"
	opDecrypt                             = "Decrypt"
	opDeriveSharedSecret                  = "DeriveSharedSecret"
	opDescribeKey                         = "DescribeKey"
	opEncrypt                             = "Encrypt"
	opGenerateDataKey                     = "GenerateDataKey"
	opGenerateDataKeyPair                 = "GenerateDataKeyPair"
	opGenerateDataKeyPairWithoutPlaintext = "GenerateDataKeyPairWithoutPlaintext"
	opGenerateDataKeyWithoutPlaintext     = "GenerateDataKeyWithoutPlaintext"
	opGenerateMac                         = "GenerateMac"
	opGetPublicKey                        = "GetPublicKey"
	opRetireGrant                         = "RetireGrant"
	opSign                                = "Sign"
	opVerify                              = "Verify"
	opVerifyMac                           = "VerifyMac"
)

// ErrUnknownOperation is returned when the requested KMS operation is not supported.
var ErrUnknownOperation = errors.New("UnknownOperationException")

// Handler is the Echo HTTP handler for KMS operations.
type Handler struct {
	Backend       StorageBackend
	actions       map[string]kmsActionFn
	janitor       *Janitor
	tags          map[string]*tags.Tags
	tagsMu        *lockmetrics.RWMutex
	DefaultRegion string
}

// NewHandler creates a new KMS handler with the given storage backend and logger.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{
		Backend: backend,
		tags:    make(map[string]*tags.Tags),
		tagsMu:  lockmetrics.New("kms.tags"),
	}
	h.actions = h.buildDispatchTable()

	return h
}

// WithJanitor attaches a background key-deletion janitor to the handler.
// If the backend is not an *InMemoryBackend, this is a no-op.
func (h *Handler) WithJanitor(interval time.Duration, taskTimeout ...time.Duration) *Handler {
	if mem, ok := h.Backend.(*InMemoryBackend); ok {
		j := NewJanitor(mem, interval)
		if len(taskTimeout) > 0 {
			j.TaskTimeout = taskTimeout[0]
		}
		j.OnKeyPurged = h.purgeTags
		h.janitor = j
	}

	return h
}

// StartWorker starts the background janitor if one is configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Reset clears all state in the backend and the handler's tag store.
// It is used by the POST /_gopherstack/reset endpoint for CI pipelines.
func (h *Handler) Reset() {
	type resetter interface{ Reset() }
	if r, ok := h.Backend.(resetter); ok {
		r.Reset()
	}

	h.tagsMu.Lock("Reset")
	defer h.tagsMu.Unlock()

	for _, t := range h.tags {
		if t != nil {
			t.Close()
		}
	}

	h.tags = make(map[string]*tags.Tags)
}

// Name returns the service name.
func (h *Handler) Name() string {
	return "KMS"
}

// GetSupportedOperations returns the list of supported KMS operations (sorted alphabetically).
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CancelKeyDeletion",
		"ConnectCustomKeyStore",
		"CreateAlias",
		"CreateCustomKeyStore",
		opCreateGrant,
		"CreateKey",
		opDecrypt,
		"DeleteAlias",
		"DeleteCustomKeyStore",
		"DeleteImportedKeyMaterial",
		opDeriveSharedSecret,
		"DescribeCustomKeyStores",
		opDescribeKey,
		"DisableKey",
		"DisableKeyRotation",
		"DisconnectCustomKeyStore",
		"EnableKey",
		"EnableKeyRotation",
		opEncrypt,
		opGenerateDataKey,
		opGenerateDataKeyPair,
		opGenerateDataKeyPairWithoutPlaintext,
		opGenerateDataKeyWithoutPlaintext,
		opGenerateMac,
		"GenerateRandom",
		"GetKeyLastUsage",
		"GetKeyPolicy",
		"GetKeyRotationStatus",
		"GetParametersForImport",
		opGetPublicKey,
		"ImportKeyMaterial",
		"ListAliases",
		"ListGrants",
		"ListKeyPolicies",
		"ListKeyRotations",
		"ListKeys",
		"ListResourceTags",
		"ListRetirableGrants",
		"PutKeyPolicy",
		"ReEncrypt",
		"ReplicateKey",
		opRetireGrant,
		"RevokeGrant",
		"RotateKeyOnDemand",
		"ScheduleKeyDeletion",
		opSign,
		"TagResource",
		"UntagResource",
		"UpdateAlias",
		"UpdateCustomKeyStore",
		"UpdateKeyDescription",
		"UpdatePrimaryRegion",
		opVerify,
		opVerifyMac,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "kms" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this KMS instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.DefaultRegion} }

// RouteMatcher returns a matcher that identifies KMS requests by the X-Amz-Target header.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, "TrentService")
	}
}

// MatchPriority returns the routing priority for the KMS handler.
func (h *Handler) MatchPriority() int {
	return service.PriorityHeaderPartial
}

// ExtractOperation extracts the KMS operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")

	const targetParts = 2
	if len(parts) == targetParts {
		return parts[1]
	}

	return "Unknown"
}

// ExtractResource returns the key ID from the request body when present.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	if keyID, ok := data["KeyId"].(string); ok {
		return keyID
	}

	return ""
}

// Handler returns the Echo handler function for KMS operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"KMS", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			func(ctx context.Context, action string, body []byte) ([]byte, error) {
				return h.dispatch(ctx, c.Request(), action, body)
			},
			h.handleError,
		)
	}
}

type kmsActionFn func(ctx context.Context, body []byte) (any, error)

// buildDispatchTable merges key lifecycle, crypto, alias/rotation, and tag actions into a single lookup map.
func (h *Handler) buildDispatchTable() map[string]kmsActionFn {
	table := h.buildKeyLifecycleActions()
	maps.Copy(table, h.buildCryptoActions())
	maps.Copy(table, h.buildAliasRotationActions())
	maps.Copy(table, h.buildGrantPolicyActions())
	maps.Copy(table, h.buildTagActions())
	maps.Copy(table, h.buildNewOpsActions())
	table["GetKeyLastUsage"] = unmarshalAction(func(ctx context.Context, i *GetKeyLastUsageInput) (any, error) {
		return h.Backend.GetKeyLastUsage(ctx, i)
	})

	return table
}

// unmarshalAction is a generic helper that creates a kmsActionFn from a strongly-typed backend call.
func unmarshalAction[T any](fn func(context.Context, *T) (any, error)) kmsActionFn {
	return func(ctx context.Context, b []byte) (any, error) {
		var input T
		if err := json.Unmarshal(b, &input); err != nil {
			return nil, err
		}

		return fn(ctx, &input)
	}
}

// buildNewOpsActions returns dispatch entries for newly implemented KMS operations.
// Delegates to sub-builders to stay within gocognit limits.
func (h *Handler) buildNewOpsActions() map[string]kmsActionFn {
	m := make(map[string]kmsActionFn)
	maps.Copy(m, h.buildCustomKeyStoreActions())
	maps.Copy(m, h.buildGenerateAndMacActions())
	maps.Copy(m, h.buildReplicationAndMaintenanceActions())

	return m
}

// dispatch routes the KMS operation to the appropriate backend method.
func (h *Handler) dispatch(ctx context.Context, r *http.Request, action string, body []byte) ([]byte, error) {
	region := httputils.ExtractRegionFromRequest(r, h.DefaultRegion)
	ctx = context.WithValue(ctx, regionContextKey{}, region)

	fn, ok := h.actions[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownOperation, action)
	}

	response, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(response)
}

// handleError writes a structured error response for a KMS operation failure.
const (
	awsErrNotFound          = "NotFoundException"
	awsErrValidation        = "ValidationException"
	awsErrInvalidCiphertext = "InvalidCiphertextException"
)

// kmsErrorEntry maps a sentinel error to its AWS error type string and HTTP status.
// httpStatus is 0 for the common case (400 Bad Request, i.e. a client-fault exception);
// set it explicitly only for server-fault exceptions (ErrorFault: Server in the real SDK).
type kmsErrorEntry struct {
	sentinel   error
	awsType    string
	httpStatus int
}

// kmsErrorTable returns the ordered error-to-AWS-type mapping for KMS error classification.
// Defined as a function to satisfy gochecknoglobals.
func kmsErrorTable() []kmsErrorEntry {
	return []kmsErrorEntry{
		{sentinel: ErrKeyNotFound, awsType: awsErrNotFound},
		{sentinel: ErrMalformedPolicyDocument, awsType: "MalformedPolicyDocumentException"},
		{sentinel: ErrAliasNotFound, awsType: awsErrNotFound},
		{sentinel: ErrGrantNotFound, awsType: awsErrNotFound},
		{sentinel: ErrCustomKeyStoreNotFound, awsType: "CustomKeyStoreNotFoundException"},
		{sentinel: ErrCustomKeyStoreInvalidState, awsType: "CustomKeyStoreInvalidStateException"},
		{sentinel: ErrCustomKeyStoreHasKeys, awsType: "CustomKeyStoreHasCMKsException"},
		{sentinel: ErrKeyDisabled, awsType: "DisabledException"},
		{sentinel: ErrKeyInvalidState, awsType: "KMSInvalidStateException"},
		{sentinel: ErrInvalidKeyUsage, awsType: "InvalidKeyUsageException"},
		{sentinel: ErrAliasAlreadyExists, awsType: "AlreadyExistsException"},
		{sentinel: ErrInvalidAliasName, awsType: "InvalidAliasNameException"},
		{sentinel: ErrCustomKeyStoreAlreadyExists, awsType: "CustomKeyStoreNameInUseException"},
		{sentinel: ErrIncorrectKey, awsType: "IncorrectKeyException"},
		{sentinel: ErrInvalidCiphertext, awsType: awsErrInvalidCiphertext},
		{sentinel: ErrCiphertextTooShort, awsType: awsErrInvalidCiphertext},
		{sentinel: ErrInvalidSignature, awsType: "KMSInvalidSignatureException"},
		{sentinel: ErrInvalidMac, awsType: "KMSInvalidMacException"},
		{sentinel: ErrUnsupportedOrigin, awsType: "UnsupportedOperationException"},
		{sentinel: ErrValidation, awsType: awsErrValidation},
		{sentinel: ErrInvalidDataKeySize, awsType: awsErrValidation},
		{sentinel: ErrInvalidGrantToken, awsType: "InvalidGrantTokenException"},
		{sentinel: ErrAccessDenied, awsType: "AccessDeniedException"},
		{sentinel: ErrLimitExceeded, awsType: "LimitExceededException"},
		{sentinel: ErrInvalidAlgorithm, awsType: "InvalidAlgorithmException"},
		{sentinel: ErrUnknownOperation, awsType: "UnknownOperationException"},
		{sentinel: ErrExpiredKeyMaterial, awsType: "ExpiredImportTokenException"},
		{sentinel: ErrInvalidTag, awsType: "TagException"},
		{sentinel: ErrUnsupportedParameter, awsType: "UnsupportedOperationException"},
		{sentinel: ErrInvalidImportToken, awsType: "InvalidImportTokenException"},
		{sentinel: ErrInvalidArn, awsType: "InvalidArnException"},
		// KeyUnavailableException is a server-fault exception in the real SDK
		// (ErrorFault: Server) — real AWS returns it with a 500 status, unlike the
		// client-fault exceptions above which are all 400.
		{
			sentinel:   ErrKeyMaterialUnavailable,
			awsType:    "KeyUnavailableException",
			httpStatus: http.StatusInternalServerError,
		},
	}
}

func (h *Handler) handleError(ctx context.Context, c *echo.Context, action string, reqErr error) error {
	log := logger.Load(ctx)
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.1")

	errorType, statusCode := classifyKMSError(reqErr)

	if statusCode == http.StatusInternalServerError {
		log.ErrorContext(ctx, "KMS internal error", "error", reqErr, "action", action)
	} else {
		log.WarnContext(ctx, "KMS request error", "error", reqErr, "action", action)
	}

	payload, _ := json.Marshal(ErrorResponse{
		Type:    errorType,
		Message: reqErr.Error(),
	})

	return c.JSONBlob(statusCode, payload)
}

// classifyKMSError returns the AWS error type string and HTTP status code for reqErr.
func classifyKMSError(reqErr error) (string, int) {
	for _, m := range kmsErrorTable() {
		if errors.Is(reqErr, m.sentinel) {
			if m.httpStatus != 0 {
				return m.awsType, m.httpStatus
			}

			return m.awsType, http.StatusBadRequest
		}
	}

	// KMSInternalException is the real AWS KMS type for an unclassified server-side
	// failure; "InternalServiceError" is not a KMS exception name at all and would
	// deserialize client-side as an opaque *smithy.GenericAPIError instead of the
	// real types.KMSInternalException.
	return "KMSInternalException", http.StatusInternalServerError
}
