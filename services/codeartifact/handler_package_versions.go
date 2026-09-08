package codeartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func packageVersionToMap(pv *PackageVersion) map[string]any {
	m := map[string]any{
		keyVersion:      pv.Version,
		keyStatusField:  pv.Status,
		"format":        pv.Format,
		"packageName":   pv.PackageName,
		"publishedTime": epochSeconds(pv.PublishedAt),
		keyRevision:     pv.Revision,
	}
	if pv.Namespace != "" {
		m["namespace"] = pv.Namespace
	}

	return m
}

// packageVersionSummaryToMap builds the types.PackageVersionSummary shape
// (types.go:547) -- no format, packageName, publishedTime, or namespace,
// all of which are Get-only (types.PackageVersionDescription, not
// types.PackageVersionSummary; confirmed against
// awsRestjson1_deserializeDocumentPackageVersionSummary, which recognises
// only origin/revision/status/version). origin is a real Summary member
// but the backend's PackageVersion model has no source for it, so it stays
// absent rather than fabricated.
func packageVersionSummaryToMap(pv *PackageVersion) map[string]any {
	return map[string]any{
		keyVersion:     pv.Version,
		keyStatusField: pv.Status,
		keyRevision:    pv.Revision,
	}
}

func (h *Handler) handleDescribePackageVersion(
	c *echo.Context,
	domainName, repoName, format, namespace, name, version string,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}
	if version == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "version is required"))
	}

	pv, err := h.Backend.DescribePackageVersion(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		version,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"packageVersion": packageVersionToMap(pv),
	})
}

// packageVersionOutcomesToWire builds the failedVersions/successfulVersions wire
// values shared by DeletePackageVersions/CopyPackageVersions/DisposePackageVersions/
// UpdatePackageVersionsStatus -- both are real JSON *objects* keyed by version
// string (map[string]types.PackageVersionError / map[string]types.SuccessfulPackageVersionInfo),
// confirmed against aws-sdk-go-v2 deserializers.go's
// ...PackageVersionErrorMap/...SuccessfulPackageVersionInfoMap -- NOT an array.
func packageVersionOutcomesToWire(
	successful map[string]PackageVersionOutcome, failed map[string]string,
) (map[string]any, map[string]any) {
	successList := make(map[string]any, len(successful))
	for v, outcome := range successful {
		successList[v] = map[string]any{"revision": outcome.Revision, keyStatusField: outcome.Status}
	}

	failedList := make(map[string]any, len(failed))
	for v, code := range failed {
		failedList[v] = map[string]any{"errorCode": code}
	}

	return successList, failedList
}

type deletePackageVersionsBody struct {
	Versions []string `json:"versions"`
}

func (h *Handler) handleDeletePackageVersions(
	c *echo.Context,
	domainName, repoName, format, namespace, name string,
	body []byte,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}

	var in deletePackageVersionsBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	successful, failed, err := h.Backend.DeletePackageVersions(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		in.Versions,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	successList, failedList := packageVersionOutcomesToWire(successful, failed)

	return c.JSON(http.StatusOK, map[string]any{
		keyFailedVersions:     failedList,
		keySuccessfulVersions: successList,
	})
}

type copyPackageVersionsBody struct {
	Versions []string `json:"versions"`
}

func (h *Handler) handleCopyPackageVersions(
	c *echo.Context,
	domainName, srcRepo, dstRepo, format, namespace, name string,
	body []byte,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if srcRepo == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "sourceRepository is required"))
	}
	if dstRepo == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "destinationRepository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}

	var in copyPackageVersionsBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	successful, failed, err := h.Backend.CopyPackageVersions(
		c.Request().Context(),
		domainName,
		srcRepo,
		dstRepo,
		format,
		namespace,
		name,
		in.Versions,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	successList, failedList := packageVersionOutcomesToWire(successful, failed)

	return c.JSON(http.StatusOK, map[string]any{
		keyFailedVersions:     failedList,
		keySuccessfulVersions: successList,
	})
}

type disposeVersionsBody struct {
	Versions []string `json:"versions"`
}

func (h *Handler) handleDisposePackageVersions(
	c *echo.Context, domainName, repoName, format, namespace, name string, body []byte,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}

	var in disposeVersionsBody
	if len(body) > 0 {
		_ = json.Unmarshal(body, &in)
	}

	successful, failed, err := h.Backend.DisposePackageVersions(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		in.Versions,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	successList, failedList := packageVersionOutcomesToWire(successful, failed)

	return c.JSON(http.StatusOK, map[string]any{keySuccessfulVersions: successList, keyFailedVersions: failedList})
}

func (h *Handler) handleGetPackageVersionAsset(
	c *echo.Context, domainName, repoName, format, namespace, name, version, asset string,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}
	if version == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "version is required"))
	}
	if asset == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "asset is required"))
	}

	data, err := h.Backend.GetPackageVersionAsset(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		version,
		asset,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Blob(http.StatusOK, "application/octet-stream", data)
}

// validatePackageVersionParams returns the raw, unwritten validation error so
// callers can map and write it exactly once via h.handleError. It used to
// write its own 400 body via c.JSON and return that call's (always-nil, on a
// successful write) result directly; callers stored that nil in err and
// tested it before continuing, so the rejection was silently treated as
// success and the real (read-only) Backend method still ran, writing a
// second body on top of the committed one (gopherstack-7opw, the
// gopherstack-8haq shape).
func (h *Handler) validatePackageVersionParams(
	domainName, repoName, format, name, version string,
) error {
	if domainName == "" {
		return fmt.Errorf("%w: domain is required", ErrValidation)
	}
	if repoName == "" {
		return fmt.Errorf("%w: repository is required", ErrValidation)
	}
	if format == "" {
		return fmt.Errorf("%w: format is required", ErrValidation)
	}
	if name == "" {
		return fmt.Errorf("%w: package is required", ErrValidation)
	}
	if version == "" {
		return fmt.Errorf("%w: version is required", ErrValidation)
	}

	return nil
}

// handleGetPackageVersionReadme builds GetPackageVersionReadmeOutput's wire
// shape -- verified against aws-sdk-go-v2 deserializers.go's
// awsRestjson1_deserializeOpDocumentGetPackageVersionReadmeOutput
// ({"format","namespace","package","readme","version","versionRevision"}).
func (h *Handler) handleGetPackageVersionReadme(
	c *echo.Context, domainName, repoName, format, namespace, name, version string,
) error {
	if err := h.validatePackageVersionParams(domainName, repoName, format, name, version); err != nil {
		return h.handleError(c, err)
	}

	readme, pv, err := h.Backend.GetPackageVersionReadme(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		version,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	resp := map[string]any{
		keyFormat:         pv.Format,
		keyPackageKey:     pv.PackageName,
		keyVersion:        pv.Version,
		"readme":          readme,
		"versionRevision": pv.Revision,
	}
	if pv.Namespace != "" {
		resp["namespace"] = pv.Namespace
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleListPackageVersionAssets(
	c *echo.Context, domainName, repoName, format, namespace, name, version string,
) error {
	if err := h.validatePackageVersionParams(domainName, repoName, format, name, version); err != nil {
		return h.handleError(c, err)
	}

	assets, err := h.Backend.ListPackageVersionAssets(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		version,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	items := make([]map[string]any, 0, len(assets))
	for _, a := range assets {
		items = append(items, assetSummaryToMap(a))
	}

	return c.JSON(http.StatusOK, map[string]any{"assets": items})
}

// assetSummaryToMap builds the wire shape of AssetSummary -- verified against
// aws-sdk-go-v2 deserializers.go's awsRestjson1_deserializeDocumentAssetSummary.

func assetSummaryToMap(a AssetInfo) map[string]any {
	return map[string]any{
		"name": a.Name,
		"size": a.Size,
		"hashes": map[string]string{
			"SHA256": a.SHA256,
		},
	}
}

// packageDependencyToMap builds a PackageDependency wire object -- verified
// against aws-sdk-go-v2 types.PackageDependency
// ({"dependencyType","namespace","package","versionRequirement"}).
func packageDependencyToMap(d PackageDependencyInfo) map[string]any {
	m := map[string]any{
		"dependencyType":     d.DependencyType,
		keyPackageKey:        d.PackageName,
		"versionRequirement": d.VersionRequirement,
	}
	if d.Namespace != "" {
		m["namespace"] = d.Namespace
	}

	return m
}

// handleListPackageVersionDependencies builds
// ListPackageVersionDependenciesOutput's wire shape -- verified against
// aws-sdk-go-v2 deserializers.go's
// awsRestjson1_deserializeOpDocumentListPackageVersionDependenciesOutput
// ({"dependencies","format","namespace","package","version"}).
func (h *Handler) handleListPackageVersionDependencies(
	c *echo.Context, domainName, repoName, format, namespace, name, version string,
) error {
	if err := h.validatePackageVersionParams(domainName, repoName, format, name, version); err != nil {
		return h.handleError(c, err)
	}

	deps, err := h.Backend.ListPackageVersionDependencies(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		version,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	items := make([]map[string]any, 0, len(deps))
	for _, d := range deps {
		items = append(items, packageDependencyToMap(d))
	}

	resp := map[string]any{
		"dependencies": items,
		keyFormat:      format,
		keyPackageKey:  name,
		keyVersion:     version,
	}
	if namespace != "" {
		resp["namespace"] = namespace
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleListPackageVersions(
	c *echo.Context, domainName, repoName, format, namespace, name string,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}

	q := c.Request().URL.Query()
	maxResults := parseMaxResults(q.Get("max-results"))
	nextToken := q.Get("next-token")
	// status/sortBy are real ListPackageVersionsInput filter/ordering members
	// (serializers.go's SetQuery("status")/SetQuery("sortBy")) that were
	// silently discarded -- every call returned every version in
	// Version-ascending order regardless of what was requested. originType
	// is also real but has no backend field to source from -- see PARITY.md.
	status := q.Get("status")
	sortBy := q.Get("sortBy")

	all, err := h.Backend.ListPackageVersions(
		c.Request().Context(), domainName, repoName, format, namespace, name, status, sortBy,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	page, next := paginateSlice(all, maxResults, nextToken, func(pv *PackageVersion) string { return pv.Version })

	items := make([]map[string]any, 0, len(page))
	for _, pv := range page {
		items = append(items, packageVersionSummaryToMap(pv))
	}

	resp := map[string]any{"versions": items, "package": name, "format": format}
	if namespace != "" {
		resp["namespace"] = namespace
	}
	if next != "" {
		resp["nextToken"] = next
	}
	// defaultDisplayVersion is real (api_op_ListPackageVersions.go) -- AWS's
	// doc says "most recently published" for every format except npm with a
	// dist-tag set, and this backend has no dist-tag concept at all, so
	// most-recently-published is the correct fallback in every case here,
	// not an approximation.
	if dv := mostRecentlyPublished(all); dv != "" {
		resp["defaultDisplayVersion"] = dv
	}

	return c.JSON(http.StatusOK, resp)
}

// mostRecentlyPublished returns the Version of the PackageVersion with the
// latest PublishedAt in versions, or "" if versions is empty.
func mostRecentlyPublished(versions []*PackageVersion) string {
	var latest *PackageVersion
	for _, pv := range versions {
		if latest == nil || pv.PublishedAt.After(latest.PublishedAt) {
			latest = pv
		}
	}
	if latest == nil {
		return ""
	}

	return latest.Version
}

func (h *Handler) handlePublishPackageVersion(
	c *echo.Context,
	domainName, repoName, format, namespace, name, version, assetName, assetSHA256 string,
	body []byte,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}
	if version == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "version is required"))
	}
	if assetName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "asset is required"))
	}
	if assetSHA256 == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "X-Amz-Content-Sha256 header is required"))
	}

	sum := sha256.Sum256(body)
	computedSHA256 := hex.EncodeToString(sum[:])

	if !strings.EqualFold(assetSHA256, computedSHA256) {
		// The pinned SDK (codeartifact@v1.41.4) declares no MismatchedSha256Exception
		// for this op -- only AccessDeniedException/ConflictException/
		// InternalServerException/ResourceNotFoundException/
		// ServiceQuotaExceededException/ThrottlingException/ValidationException
		// (verified against deserializers.go's awsRestjson1_deserializeOpErrorPublishPackageVersion).
		return c.JSON(
			http.StatusBadRequest,
			errResp("ValidationException", "assetSHA256 does not match the computed SHA256 of assetContent"),
		)
	}

	asset := AssetInfo{
		Name:    assetName,
		Size:    int64(len(body)),
		SHA256:  computedSHA256,
		Content: body,
	}

	pv, err := h.Backend.PublishPackageVersion(
		c.Request().Context(),
		domainName,
		repoName,
		format,
		namespace,
		name,
		version,
		asset,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, publishPackageVersionToMap(pv, asset))
}

// publishPackageVersionToMap builds PublishPackageVersionOutput's wire shape -- a FLAT
// object (no "packageVersion" envelope), with field names "package"/"versionRevision"
// (not "packageName"/"revision") and an "asset" summary. Verified against aws-sdk-go-v2
// deserializers.go's awsRestjson1_deserializeOpDocumentPublishPackageVersionOutput.

func publishPackageVersionToMap(pv *PackageVersion, asset AssetInfo) map[string]any {
	m := map[string]any{
		keyFormat:         pv.Format,
		keyPackageKey:     pv.PackageName,
		keyStatusField:    pv.Status,
		keyVersion:        pv.Version,
		"versionRevision": pv.Revision,
		"asset":           assetSummaryToMap(asset),
	}
	if pv.Namespace != "" {
		m["namespace"] = pv.Namespace
	}

	return m
}

// putPackageOriginConfigurationBody is PutPackageOriginConfigurationInput's request
// shape -- verified against aws-sdk-go-v2 serializers.go's
// awsRestjson1_serializeOpDocumentPutPackageOriginConfigurationInput.

type updateVersionsStatusBody struct {
	TargetStatus string   `json:"targetStatus"`
	Versions     []string `json:"versions"`
}

func (h *Handler) handleUpdatePackageVersionsStatus(
	c *echo.Context, domainName, repoName, format, namespace, name string, body []byte,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "format is required"))
	}
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "package is required"))
	}

	var in updateVersionsStatusBody
	if len(body) > 0 {
		_ = json.Unmarshal(body, &in)
	}

	if in.TargetStatus == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "targetStatus is required"))
	}

	successful, failed, err := h.Backend.UpdatePackageVersionsStatus(
		c.Request().Context(), domainName, repoName, format, namespace, name, in.TargetStatus, in.Versions,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	successList, failedList := packageVersionOutcomesToWire(successful, failed)

	return c.JSON(http.StatusOK, map[string]any{keySuccessfulVersions: successList, keyFailedVersions: failedList})
}

// updateRepositoryBody's Upstreams field uses the wire key "upstreams", same as
// createRepositoryBody -- see its comment for the verified source.
