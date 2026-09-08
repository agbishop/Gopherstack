package lambda

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// validateQualifier validates a function qualifier. A valid qualifier is either
// "$LATEST", an alias name (letters, digits, hyphens, underscores), or a version
// number (digits only). Returns true if valid; writes an error response and returns
// false if the qualifier is non-empty but malformed.
func (h *Handler) validateQualifier(c *echo.Context, qualifier string) bool {
	if qualifier == "" || qualifier == versionLatest {
		return true
	}

	// Version number: digits only.
	isVersion := true
	for _, ch := range qualifier {
		if ch < '0' || ch > '9' {
			isVersion = false

			break
		}
	}

	if isVersion {
		return true
	}

	// Alias name: letters, digits, hyphens, underscores.
	if !isValidAliasName(qualifier) {
		_ = h.writeError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValueException",
			fmt.Sprintf(
				"invalid qualifier %q: must be $LATEST, a version number, or a valid alias name",
				qualifier,
			),
		)

		return false
	}

	return true
}

// validateCreateFunctionInput checks required fields and package-type-specific constraints.
// It normalizes PackageType to Image when omitted. Returns true if validation passes.
// If validation fails, it writes the HTTP error response and returns false.
func (h *Handler) validateCreateFunctionInput(c *echo.Context, input *CreateFunctionInput) bool {
	if input.FunctionName == "" {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "FunctionName is required")

		return false
	}

	if !h.validateMemoryAndTimeout(c, input.MemorySize, input.Timeout) {
		return false
	}

	if input.PackageType == "" {
		input.PackageType = PackageTypeImage
	}

	if input.PackageType != PackageTypeImage && input.PackageType != PackageTypeZip {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			"PackageType must be Image or Zip")

		return false
	}

	if !h.validateCreateFunctionCode(c, input) {
		return false
	}

	if !h.validateSnapStartInput(c, input.SnapStart) {
		return false
	}

	return h.validateEphemeralStorageInput(c, input.EphemeralStorage)
}

// validateSnapStartInput checks the optional SnapStart.ApplyOn value. AWS only
// accepts "None" or "PublishedVersions"; anything else is rejected with
// InvalidParameterValueException. A nil config (omitted) is valid.
func (h *Handler) validateSnapStartInput(c *echo.Context, s *SnapStart) bool {
	if s == nil || s.ApplyOn == "" {
		return true
	}

	if s.ApplyOn != SnapStartApplyOnNone && s.ApplyOn != SnapStartApplyOnPublishedVersions {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			"SnapStart.ApplyOn must be one of [PublishedVersions, None]")

		return false
	}

	return true
}

// validateEphemeralStorageInput checks the optional EphemeralStorage field and writes an error
// response when the supplied size is outside the allowed range. Returns true when valid.
func (h *Handler) validateEphemeralStorageInput(c *echo.Context, es *EphemeralStorageConfig) bool {
	if es == nil {
		return true
	}

	if es.Size < minEphemeralStorageSize || es.Size > maxEphemeralStorageSize {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			fmt.Sprintf("EphemeralStorage.Size must be between %d and %d MB",
				minEphemeralStorageSize, maxEphemeralStorageSize))

		return false
	}

	return true
}

// validateMemoryAndTimeout validates MemorySize and Timeout values (both 0 means use defaults).
func (h *Handler) validateMemoryAndTimeout(c *echo.Context, memorySize, timeout int) bool {
	if memorySize != 0 && (memorySize < minMemorySize || memorySize > maxMemorySize) {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			fmt.Sprintf("MemorySize must be between %d and %d MB", minMemorySize, maxMemorySize))

		return false
	}

	if timeout != 0 && (timeout < minTimeout || timeout > maxTimeout) {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			fmt.Sprintf("Timeout must be between %d and %d seconds", minTimeout, maxTimeout))

		return false
	}

	return true
}

// validateImageURIResolves reports whether imageURI resolves against a real
// ECR backend, writing an InvalidParameterValueException matching AWS's
// CreateFunction/UpdateFunctionCode message ("Source image <uri> does not
// exist. Provide a valid source image.") and returning false when it does
// not. When h.Backend has no ImageURIResolver (or no ECRResolver is wired
// in via cli.go's wireLambdaECR), every ImageUri is accepted.
func (h *Handler) validateImageURIResolves(c *echo.Context, imageURI string) bool {
	ir, ok := h.Backend.(ImageURIResolver)
	if !ok || ir.ResolveImageURI(imageURI) {
		return true
	}

	_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
		fmt.Sprintf("Source image %s does not exist. Provide a valid source image.", imageURI))

	return false
}

// validateCreateFunctionCode validates the Code field based on PackageType.
func (h *Handler) validateCreateFunctionCode(c *echo.Context, input *CreateFunctionInput) bool {
	if input.Code == nil {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "Code is required")

		return false
	}

	if input.PackageType == PackageTypeImage {
		if input.Code.ImageURI == "" {
			_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
				"Code.ImageUri is required for Image package type")

			return false
		}

		if !h.validateImageURIResolves(c, input.Code.ImageURI) {
			return false
		}
	}

	if input.PackageType == PackageTypeZip {
		if input.Runtime == "" {
			_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
				"Runtime is required for Zip package type")

			return false
		}

		if !isValidRuntime(input.Runtime) {
			_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
				fmt.Sprintf(
					"Value %q at 'runtime' failed to satisfy constraint: "+
						"Member must satisfy enum value set", input.Runtime,
				))

			return false
		}

		if input.Code.ZipFile == nil && (input.Code.S3Bucket == "" || input.Code.S3Key == "") {
			_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
				"Code.ZipFile or Code.S3Bucket+Code.S3Key is required for Zip package type")

			return false
		}
	}

	return true
}

// applyImageConfig sets fn.ImageConfigResponse from input when the package
// type is Image, wrapping ImageConfig the way the real wire response does.
func applyImageConfig(fn *FunctionConfiguration, input *CreateFunctionInput) {
	if input.PackageType == PackageTypeImage && input.ImageConfig != nil {
		fn.ImageConfigResponse = &ImageConfigResponse{ImageConfig: input.ImageConfig}
	}
}

// applyZipDigest computes CodeSize and CodeSha256 from fn.ZipData when present.
func applyZipDigest(fn *FunctionConfiguration) {
	if len(fn.ZipData) > 0 {
		fn.CodeSize = int64(len(fn.ZipData))
		sum := sha256.Sum256(fn.ZipData)
		fn.CodeSha256 = base64.StdEncoding.EncodeToString(sum[:])
	}
}

func (h *Handler) handleCreateFunction(c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", "failed to read request")
	}

	var input CreateFunctionInput
	if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "invalid request body")
	}

	if !h.validateCreateFunctionInput(c, &input) {
		return nil
	}

	memorySize := input.MemorySize
	if memorySize <= 0 {
		memorySize = defaultMemorySize
	}

	timeout := input.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	now := time.Now().UTC()
	fn := &FunctionConfiguration{
		FunctionName:      input.FunctionName,
		FunctionArn:       buildARN(h.DefaultRegion, h.AccountID, input.FunctionName),
		Description:       input.Description,
		ImageURI:          input.Code.ImageURI,
		PackageType:       input.PackageType,
		Runtime:           input.Runtime,
		Handler:           input.Handler,
		Role:              input.Role,
		MemorySize:        memorySize,
		Timeout:           timeout,
		Environment:       input.Environment,
		VpcConfig:         input.VpcConfig,
		TracingConfig:     input.TracingConfig,
		FileSystemConfigs: input.FileSystemConfigs,
		DeadLetterConfig:  input.DeadLetterConfig,
		EphemeralStorage:  input.EphemeralStorage,
		DurableConfig:     input.DurableConfig,
		Layers:            layerARNsToFunctionLayers(input.Layers),
		Tags:              input.Tags,
		State:             FunctionStateActive,
		LastUpdateStatus:  LastUpdateStatusSuccessful,
		CreatedAt:         now,
		LastModified:      now.Format(time.RFC3339),
		RevisionID:        uuid.New().String(),
		ZipData:           input.Code.ZipFile,
		S3BucketCode:      input.Code.S3Bucket,
		S3KeyCode:         input.Code.S3Key,
	}

	applyImageConfig(fn, &input)
	applyZipDigest(fn)
	applySnapStart(fn, input.SnapStart)

	if len(input.Architectures) > 0 {
		fn.Architectures = input.Architectures
	}

	if createErr := h.Backend.CreateFunction(fn); createErr != nil {
		if errors.Is(createErr, ErrFunctionAlreadyExists) {
			return h.writeError(c, http.StatusConflict, "ResourceConflictException", createErr.Error())
		}

		if errors.Is(createErr, ErrInvalidParameterValue) {
			return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", createErr.Error())
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", createErr.Error())
	}

	// h.tags is a separate store from fn.Tags (see handler_tags.go); TaggedFunctions,
	// used by the Resource Groups Tagging API, reads only h.tags, so tags supplied at
	// creation must be mirrored here or they're invisible to cross-service tag listing.
	if len(input.Tags) > 0 {
		h.setTags(fn.FunctionArn, input.Tags)
	}

	// When Publish: true, immediately publish version 1 so that the caller can
	// reference aws_lambda_function.this.version (used by provisioned concurrency etc.).
	// Return a response copy with the numbered version; the stored live fn keeps "$LATEST".
	if input.Publish {
		if publishedVersion := h.maybePublishVersion(
			c.Request().Context(), fn, "CreateFunction",
		); publishedVersion != "" {
			resp := *fn
			resp.Version = publishedVersion

			return c.JSON(http.StatusCreated, &resp)
		}
	}

	return c.JSON(http.StatusCreated, fn)
}

func (h *Handler) handleGetFunction(c *echo.Context, name string) error {
	qualifier := c.Request().URL.Query().Get("Qualifier")
	if !h.validateQualifier(c, qualifier) {
		return nil
	}

	fn, err := h.resolveFunctionForRead(name, qualifier)
	if err != nil {
		return h.writeQualifiedReadError(c, name, qualifier, err)
	}

	return c.JSON(http.StatusOK, &GetFunctionOutput{
		Configuration: fn,
		Code:          buildCodeLocation(fn),
		Tags:          fn.Tags,
	})
}

// resolveFunctionForRead returns the function configuration for the given
// qualifier. When the qualifier is empty or "$LATEST", or the backend does not
// support qualifier resolution, it falls back to the live configuration.
func (h *Handler) resolveFunctionForRead(name, qualifier string) (*FunctionConfiguration, error) {
	if qualifier != "" && qualifier != versionLatest {
		if qr, ok := h.Backend.(QualifierResolver); ok {
			return qr.GetFunctionByQualifier(name, qualifier)
		}
	}

	return h.Backend.GetFunction(name)
}

// writeQualifiedReadError maps a qualifier-resolution error to the AWS response.
func (h *Handler) writeQualifiedReadError(c *echo.Context, name, qualifier string, err error) error {
	switch {
	case errors.Is(err, ErrFunctionNotFound):
		return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found: "+name)
	case errors.Is(err, ErrVersionNotFound):
		return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
			fmt.Sprintf("Function not found: %s:%s", name, qualifier))
	default:
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", err.Error())
	}
}

// parsePaginationParams extracts Marker and MaxItems from the request query string.
func parsePaginationParams(r *http.Request) (string, int) {
	marker := r.URL.Query().Get("Marker")
	maxItems := 0

	if v := r.URL.Query().Get("MaxItems"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxItems = n
		}
	}

	return marker, maxItems
}

func (h *Handler) handleListFunctions(c *echo.Context) error {
	marker, maxItems := parsePaginationParams(c.Request())

	// ?FunctionVersion=ALL returns all published versions in addition to $LATEST.
	if c.Request().URL.Query().Get("FunctionVersion") == "ALL" {
		if bk, ok := h.Backend.(*InMemoryBackend); ok {
			p := bk.ListFunctionsAll(marker, maxItems)

			return c.JSON(http.StatusOK, &ListFunctionsOutput{
				Functions:  p.Data,
				NextMarker: p.Next,
			})
		}
	}

	p := h.Backend.ListFunctions(marker, maxItems)

	return c.JSON(http.StatusOK, &ListFunctionsOutput{
		Functions:  p.Data,
		NextMarker: p.Next,
	})
}

func (h *Handler) handleDeleteFunction(c *echo.Context, name string) error {
	qualifier := c.Request().URL.Query().Get("Qualifier")

	var err error
	if qualifier != "" {
		qd, ok := h.Backend.(QualifierDeleter)
		if !ok {
			return h.writeError(c, http.StatusInternalServerError, "ServiceException",
				"backend does not support qualified delete")
		}

		err = qd.DeleteFunctionVersion(name, qualifier)
	} else {
		err = h.Backend.DeleteFunction(name)
	}

	if err != nil {
		switch {
		case errors.Is(err, ErrFunctionNotFound), errors.Is(err, ErrVersionNotFound):
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Function not found: "+name)
		case errors.Is(err, ErrVersionReferencedByAlias):
			return h.writeError(c, http.StatusConflict, "ResourceConflictException",
				"An alias is pointing to the version that you are trying to delete")
		case errors.Is(err, ErrInvalidParameterValue):
			return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
				"Cannot delete $LATEST as a version; omit Qualifier to delete the whole function")
		default:
			return h.writeError(c, http.StatusInternalServerError, "ServiceException", err.Error())
		}
	}

	// Deleting a single version leaves the function (and its tags) in place.
	if qualifier == "" {
		h.deleteTags(buildARN(h.DefaultRegion, h.AccountID, name))
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleUpdateFunctionCode(c *echo.Context, name string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", "failed to read request")
	}

	var input UpdateFunctionCodeInput
	if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "invalid request body")
	}

	fn, getFnErr := h.Backend.GetFunction(name)
	if getFnErr != nil {
		if errors.Is(getFnErr, ErrFunctionNotFound) {
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Function not found: "+name)
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", getFnErr.Error())
	}

	if !h.checkRevisionID(c, fn.RevisionID, input.RevisionID) {
		return nil
	}

	if !h.applyFunctionCodeUpdate(c, fn, &input) {
		return nil
	}

	fn.LastModified = time.Now().UTC().Format(time.RFC3339)
	fn.RevisionID = uuid.New().String()
	fn.LastUpdateStatus = LastUpdateStatusSuccessful

	if updateErr := h.Backend.UpdateFunction(fn); updateErr != nil {
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", updateErr.Error())
	}

	// Return a response copy with the numbered version when Publish=true;
	// the stored live fn keeps Version="$LATEST".
	if input.Publish {
		if publishedVersion := h.maybePublishVersion(
			c.Request().Context(), fn, "UpdateFunctionCode",
		); publishedVersion != "" {
			resp := *fn
			resp.Version = publishedVersion

			return c.JSON(http.StatusOK, &resp)
		}
	}

	return c.JSON(http.StatusOK, fn)
}

// maybePublishVersion publishes a new numbered version for fn when the InMemoryBackend is
// active and returns the published version string (e.g. "1"). Returns "" on failure or when
// the backend does not support versioning. The caller must NOT store the returned version on
// the live fn — the live function always keeps Version="$LATEST".
func (h *Handler) maybePublishVersion(ctx context.Context, fn *FunctionConfiguration, op string) string {
	lambdaBk, ok := h.Backend.(*InMemoryBackend)
	if !ok {
		return ""
	}

	v, pubErr := lambdaBk.PublishVersion(fn.FunctionName, "")
	if pubErr != nil {
		logger.Load(ctx).WarnContext(
			ctx,
			"lambda: Publish=true but PublishVersion failed",
			"op", op, "function", fn.FunctionName, "error", pubErr,
		)

		return ""
	}

	return v.Version
}

// applyFunctionCodeUpdate applies the code fields from input onto fn, validating
// package type constraints. Returns true on success (caller should continue);
// false when it wrote a 400 error response and the caller must stop and
// return nil, matching checkRevisionID/validateMemoryAndTimeout's bool-return
// convention. This is deliberately NOT "return writeError's return value" —
// see checkRevisionID's doc comment for why that signal can never be
// distinguished from success and previously let a validation failure here
// fall through to a second, conflicting 200 write.
func (h *Handler) applyFunctionCodeUpdate(
	c *echo.Context,
	fn *FunctionConfiguration,
	input *UpdateFunctionCodeInput,
) bool {
	var ok bool
	if fn.PackageType == PackageTypeImage || fn.PackageType == "" {
		ok = h.applyImageCodeUpdate(c, fn, input)
	} else {
		ok = h.applyZipCodeUpdate(c, fn, input)
	}

	if !ok {
		return false
	}

	if len(input.Architectures) > 0 {
		fn.Architectures = input.Architectures
	} else if len(fn.Architectures) == 0 {
		fn.Architectures = []string{"x86_64"}
	}

	return true
}

// applyImageCodeUpdate is applyFunctionCodeUpdate's Image-package-type branch.
func (h *Handler) applyImageCodeUpdate(
	c *echo.Context, fn *FunctionConfiguration, input *UpdateFunctionCodeInput,
) bool {
	if input.ImageURI == "" {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			"ImageUri is required for Image package type")

		return false
	}

	if !h.validateImageURIResolves(c, input.ImageURI) {
		return false
	}

	fn.ImageURI = input.ImageURI

	return true
}

// applyZipCodeUpdate is applyFunctionCodeUpdate's Zip-package-type branch.
func (h *Handler) applyZipCodeUpdate(c *echo.Context, fn *FunctionConfiguration, input *UpdateFunctionCodeInput) bool {
	if input.ZipFile == nil && (input.S3Bucket == "" || input.S3Key == "") {
		_ = h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			"ZipFile or S3Bucket+S3Key is required for Zip package type")

		return false
	}

	fn.ZipData = input.ZipFile
	fn.S3BucketCode = input.S3Bucket
	fn.S3KeyCode = input.S3Key

	if len(fn.ZipData) > 0 {
		fn.CodeSize = int64(len(fn.ZipData))
		sum := sha256.Sum256(fn.ZipData)
		fn.CodeSha256 = base64.StdEncoding.EncodeToString(sum[:])
	}

	return true
}

func (h *Handler) handleUpdateFunctionConfiguration(c *echo.Context, name string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", "failed to read request")
	}

	var input UpdateFunctionConfigurationInput
	if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "invalid request body")
	}

	if !h.validateMemoryAndTimeout(c, input.MemorySize, input.Timeout) {
		return nil
	}

	if input.Runtime != "" && !isValidRuntime(input.Runtime) {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			fmt.Sprintf(
				"Value %q at 'runtime' failed to satisfy constraint: "+
					"Member must satisfy enum value set", input.Runtime,
			))
	}

	if input.EphemeralStorage != nil {
		if input.EphemeralStorage.Size < minEphemeralStorageSize ||
			input.EphemeralStorage.Size > maxEphemeralStorageSize {
			return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
				fmt.Sprintf("EphemeralStorage.Size must be between %d and %d MB",
					minEphemeralStorageSize, maxEphemeralStorageSize))
		}
	}

	fn, getFnErr := h.Backend.GetFunction(name)
	if getFnErr != nil {
		if errors.Is(getFnErr, ErrFunctionNotFound) {
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Function not found: "+name)
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", getFnErr.Error())
	}

	if !h.checkRevisionID(c, fn.RevisionID, input.RevisionID) {
		return nil
	}

	applyFunctionConfigurationUpdate(fn, &input)

	fn.LastModified = time.Now().UTC().Format(time.RFC3339)
	fn.RevisionID = uuid.New().String()
	fn.LastUpdateStatus = LastUpdateStatusSuccessful

	if updateErr := h.Backend.UpdateFunction(fn); updateErr != nil {
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", updateErr.Error())
	}

	return c.JSON(http.StatusOK, fn)
}

// applySnapStart sets the SnapStart field on fn based on the input.
func applySnapStart(fn *FunctionConfiguration, s *SnapStart) {
	if s == nil {
		return
	}

	applyOn := s.ApplyOn
	if applyOn == "" {
		applyOn = "None"
	}

	optimizationStatus := "Off"
	if applyOn == "PublishedVersions" {
		optimizationStatus = "On"
	}

	fn.SnapStart = &SnapStartResponse{
		ApplyOn:            applyOn,
		OptimizationStatus: optimizationStatus,
	}
}

// applyFunctionConfigurationUpdate applies non-zero fields from input onto fn.
// Split into two passes (core scalar/identity fields, then structured configs)
// to keep cyclomatic complexity under the repo's cyclop threshold — each half
// is a flat run of independent single-field updates, so splitting the list is
// purely a complexity-budget decomposition, not a behavior change.
func applyFunctionConfigurationUpdate(fn *FunctionConfiguration, input *UpdateFunctionConfigurationInput) {
	applyFunctionConfigurationCoreFields(fn, input)
	applyFunctionConfigurationStructuredFields(fn, input)
}

// applyFunctionConfigurationCoreFields applies the scalar/identity fields
// (description, sizing, execution identity, code layers) from input onto fn.
func applyFunctionConfigurationCoreFields(fn *FunctionConfiguration, input *UpdateFunctionConfigurationInput) {
	if input.Description != "" {
		fn.Description = input.Description
	}

	if input.MemorySize > 0 {
		fn.MemorySize = input.MemorySize
	}

	if input.Timeout > 0 {
		fn.Timeout = input.Timeout
	}

	if input.Environment != nil {
		fn.Environment = input.Environment
	}

	if input.Role != "" {
		fn.Role = input.Role
	}

	if input.Runtime != "" {
		fn.Runtime = input.Runtime
	}

	if input.Handler != "" {
		fn.Handler = input.Handler
	}

	if input.Layers != nil {
		fn.Layers = layerARNsToFunctionLayers(input.Layers)
	}
}

// applyFunctionConfigurationStructuredFields applies the remaining structured
// config blocks (networking, observability, storage, durable execution) from
// input onto fn.
func applyFunctionConfigurationStructuredFields(fn *FunctionConfiguration, input *UpdateFunctionConfigurationInput) {
	if input.VpcConfig != nil {
		fn.VpcConfig = input.VpcConfig
	}

	if input.TracingConfig != nil {
		fn.TracingConfig = input.TracingConfig
	}

	if input.FileSystemConfigs != nil {
		fn.FileSystemConfigs = input.FileSystemConfigs
	}

	if input.DeadLetterConfig != nil {
		fn.DeadLetterConfig = input.DeadLetterConfig
	}

	if input.EphemeralStorage != nil {
		fn.EphemeralStorage = input.EphemeralStorage
	}

	if input.SnapStart != nil {
		applySnapStart(fn, input.SnapStart)
	}

	if input.DurableConfig != nil {
		fn.DurableConfig = input.DurableConfig
	}
}

// buildCodeLocation constructs the FunctionCodeLocation response for a function.
func buildCodeLocation(fn *FunctionConfiguration) *FunctionCodeLocation {
	if fn.PackageType == PackageTypeZip {
		loc := &FunctionCodeLocation{RepositoryType: "S3"}
		if fn.S3BucketCode != "" && fn.S3KeyCode != "" {
			loc.Location = fmt.Sprintf("s3://%s/%s", fn.S3BucketCode, fn.S3KeyCode)
		}

		return loc
	}

	return &FunctionCodeLocation{
		ImageURI:       fn.ImageURI,
		RepositoryType: "ECR",
	}
}

// buildARN constructs a Lambda function ARN.
func buildARN(region, accountID, functionName string) string {
	return arn.Build("lambda", region, accountID, "function:"+functionName)
}

// layerARNsToFunctionLayers converts a list of layer ARN strings to FunctionLayer structs.
func layerARNsToFunctionLayers(arns []string) []*FunctionLayer {
	if len(arns) == 0 {
		return nil
	}

	layers := make([]*FunctionLayer, len(arns))
	for i, a := range arns {
		layers[i] = &FunctionLayer{Arn: a}
	}

	return layers
}

// defaultMemorySize is the default Lambda function memory in MB.
const defaultMemorySize = 128

// defaultTimeout is the default Lambda function timeout in seconds.
const defaultTimeout = 3

// minMemorySize is the minimum allowed Lambda function memory in MB.
const minMemorySize = 128

// maxMemorySize is the maximum allowed Lambda function memory in MB.
const maxMemorySize = 10240

// minTimeout is the minimum allowed Lambda function timeout in seconds.
const minTimeout = 1

// maxTimeout is the maximum allowed Lambda function timeout in seconds.
const maxTimeout = 900

// handleGetFunctionConfiguration handles GET /2015-03-31/functions/{name}/configuration.
// Real AWS returns the function configuration without the code location.
func (h *Handler) handleGetFunctionConfiguration(c *echo.Context, name string) error {
	qualifier := c.Request().URL.Query().Get("Qualifier")
	if !h.validateQualifier(c, qualifier) {
		return nil
	}

	fn, err := h.resolveFunctionForRead(name, qualifier)
	if err != nil {
		return h.writeQualifiedReadError(c, name, qualifier, err)
	}

	// GetFunctionConfiguration returns the configuration only (no code location).
	return c.JSON(http.StatusOK, fn)
}
