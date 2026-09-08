package opensearch

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

func (h *Handler) handleDissociatePackage(
	w http.ResponseWriter,
	r *http.Request,
	packageID, domainName string,
) {
	details, err := h.Backend.DissociatePackage(packageID, domainName)
	if err != nil {
		if errors.Is(err, ErrDomainNotFound) {
			h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", err.Error())
		} else {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", err.Error())
		}

		return
	}

	h.writeJSON(r, w, map[string]any{"DomainPackageDetails": toDomainPackageDetailsJSON(details)})
}

func (h *Handler) handleDissociatePackages(w http.ResponseWriter, r *http.Request) {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return
	}

	var req struct {
		DomainName  string            `json:"DomainName"`
		PackageList []packageForAssoc `json:"PackageList"`
	}

	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	packageIDs := make([]string, 0, len(req.PackageList))

	for _, p := range req.PackageList {
		packageIDs = append(packageIDs, p.PackageID)
	}

	details, dissocErr := h.Backend.DissociatePackages(req.DomainName, packageIDs)
	if dissocErr != nil {
		if errors.Is(dissocErr, ErrDomainNotFound) {
			h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", dissocErr.Error())
		} else {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", dissocErr.Error())
		}

		return
	}

	outList := make([]domainPackageDetailsJSON, 0, len(details))

	for i := range details {
		outList = append(outList, toDomainPackageDetailsJSON(&details[i]))
	}

	h.writeJSON(r, w, map[string]any{"DomainPackageDetailsList": outList})
}

// handlePackageRoutes handles package routes.
func (h *Handler) handlePackageRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, openSearchPackagesPath)

	// Root paths first.
	if rest == "" || rest == "/" {
		h.handlePackageRootRoutes(w, r)

		return
	}

	// Fixed literal-action paths: describe/update/updateScope carry PackageID
	// in the body, not the URL (api_op_DescribePackages.go /
	// api_op_UpdatePackage.go / api_op_UpdatePackageScope.go,
	// opensearch@v1.75.4 serializers.go) -- gopherstack-l5ir.
	if h.handlePackageLiteralActionRoutes(w, r, rest) {
		return
	}

	// Named sub-paths: associate, dissociate.
	if h.handlePackageAssocRoutes(w, r, rest) {
		return
	}

	// Sub-resource paths: history, domains.
	if h.handlePackageSubResourceRoutes(w, r, rest) {
		return
	}

	// Fallback: single-segment package-ID routes.
	h.handlePackageIDRoutes(w, r, rest)
}

// handlePackageLiteralActionRoutes handles POST /packages/describe,
// /packages/update, and /packages/updateScope. Returns true if handled.
func (h *Handler) handlePackageLiteralActionRoutes(w http.ResponseWriter, r *http.Request, rest string) bool {
	if r.Method != http.MethodPost {
		return false
	}

	switch rest {
	case pathSuffixDescribe:
		h.handleDescribePackages(w, r)

		return true
	case pathSuffixUpdate:
		h.handleUpdatePackageRoute(w, r)

		return true
	case "/updateScope":
		h.handleUpdatePackageScopeRoute(w, r)

		return true
	default:
		return false
	}
}

// handleDescribePackages serves DescribePackages: PackageID values come from
// a DescribePackagesFilter{Name: "PackageID"} entry in the request body.
func (h *Handler) handleDescribePackages(w http.ResponseWriter, r *http.Request) {
	body, _ := httputils.ReadBody(r)

	var req struct {
		NextToken string `json:"NextToken"`
		Filters   []struct {
			Name  string   `json:"Name"`
			Value []string `json:"Value"`
		} `json:"Filters"`
		MaxResults int32 `json:"MaxResults"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	filters := make(map[string][]string, len(req.Filters))
	for _, f := range req.Filters {
		filters[f.Name] = append(filters[f.Name], f.Value...)
	}

	p, err := h.Backend.DescribePackages(filters, req.NextToken, int(req.MaxResults))
	if err != nil {
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", err.Error())

		return
	}

	pkgs := p.Data
	if pkgs == nil {
		pkgs = []*Package{}
	}

	out := map[string]any{"PackageDetailsList": pkgs}
	if p.Next != "" {
		out["NextToken"] = p.Next
	}

	h.writeJSON(r, w, out)
}

// handleUpdatePackageRoute serves UpdatePackage: POST /packages/update, PackageID in the body.
func (h *Handler) handleUpdatePackageRoute(w http.ResponseWriter, r *http.Request) {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return
	}

	var req struct {
		PackageID          string `json:"PackageID"`
		PackageDescription string `json:"PackageDescription"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	pkg, updateErr := h.Backend.UpdatePackage(req.PackageID, req.PackageDescription)
	if updateErr != nil {
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", updateErr.Error())

		return
	}

	h.writeJSON(r, w, map[string]any{jsonKeyPackageDetails: pkg})
}

// handleUpdatePackageScopeRoute serves UpdatePackageScope: POST
// /packages/updateScope, all fields carried in the body. Field set matches
// UpdatePackageScopeInput/Output (api_op_UpdatePackageScope.go:29-65 in the
// pinned SDK): PackageUserList is a top-level member, not nested under a
// "PackageScopeOperationConfig" wrapper.
func (h *Handler) handleUpdatePackageScopeRoute(w http.ResponseWriter, r *http.Request) {
	body, _ := httputils.ReadBody(r)

	var req struct {
		PackageID       string   `json:"PackageID"`
		Operation       string   `json:"Operation"`
		PackageUserList []string `json:"PackageUserList"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	pkg, err := h.Backend.UpdatePackageScope(req.PackageID, req.Operation, req.PackageUserList)
	if err != nil {
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", err.Error())

		return
	}

	h.writeJSON(r, w, map[string]any{
		jsonKeyPackageID:  pkg.PackageID,
		"Operation":       req.Operation,
		"PackageUserList": pkg.PackageUserList,
	})
}

// handlePackageAssocRoutes handles associate/dissociate package routes.
// Returns true if the request was handled.
func (h *Handler) handlePackageAssocRoutes(
	w http.ResponseWriter,
	r *http.Request,
	rest string,
) bool {
	switch {
	// POST /packages/associate/{PackageID}/{DomainName} → AssociatePackage
	case strings.HasPrefix(rest, "/associate/") && r.Method == http.MethodPost:
		parts := strings.SplitN(strings.TrimPrefix(rest, "/associate/"), "/", pkgPathParts)
		if len(parts) != pkgPathParts {
			h.writeError(
				r,
				w,
				http.StatusBadRequest,
				"ValidationException",
				"invalid associate package path",
			)

			return true
		}

		h.handleAssociatePackage(w, r, parts[0], parts[1])

		return true
	// POST /packages/associateMultiple → AssociatePackages
	case rest == "/associateMultiple" && r.Method == http.MethodPost:
		h.handleAssociatePackages(w, r)

		return true
	// POST /packages/dissociate/{PackageID}/{DomainName} → DissociatePackage.
	// Real clients POST here (api_op_DissociatePackage.go, opensearch@v1.75.4
	// serializers.go); DELETE is never sent -- gopherstack-l5ir.
	case strings.HasPrefix(rest, "/dissociate/") && r.Method == http.MethodPost:
		parts := strings.SplitN(strings.TrimPrefix(rest, "/dissociate/"), "/", pkgPathParts)
		if len(parts) != pkgPathParts {
			h.writeError(
				r,
				w,
				http.StatusBadRequest,
				"ValidationException",
				"invalid dissociate package path",
			)

			return true
		}

		h.handleDissociatePackage(w, r, parts[0], parts[1])

		return true
	// POST /packages/dissociateMultiple → DissociatePackages
	case rest == "/dissociateMultiple" && r.Method == http.MethodPost:
		h.handleDissociatePackages(w, r)

		return true
	}

	return false
}

// handlePackageSubResourceRoutes handles package sub-resource routes (history, domains).
// Returns true if the request was handled.
func (h *Handler) handlePackageSubResourceRoutes(
	w http.ResponseWriter,
	r *http.Request,
	rest string,
) bool {
	switch {
	// GET /packages/{packageId}/history → GetPackageVersionHistory
	case strings.HasSuffix(rest, "/history") && r.Method == http.MethodGet:
		pkgID := strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/history")
		history, err := h.Backend.GetPackageVersionHistory(pkgID)
		if err != nil {
			history = []*PackageVersionHistory{}
		}
		h.writeJSON(r, w, map[string]any{"PackageVersionHistoryList": history})

		return true
	// GET /packages/{packageId}/domains → ListDomainsForPackage
	case strings.HasSuffix(rest, "/domains") && r.Method == http.MethodGet:
		pkgID := strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/domains")

		var pkgName, pkgType string
		if p, err := h.Backend.DescribePackages(
			map[string][]string{jsonKeyPackageID: {pkgID}}, "", 0,
		); err == nil && len(p.Data) == 1 {
			pkgName, pkgType = p.Data[0].PackageName, p.Data[0].PackageType
		}

		domainNames := h.Backend.ListDomainsForPackage(pkgID)
		outList := make([]domainPackageDetailsJSON, 0, len(domainNames))
		for _, domainName := range domainNames {
			outList = append(outList, domainPackageDetailsJSON{
				PackageID:           pkgID,
				DomainName:          domainName,
				DomainPackageStatus: pkgStateActive,
				PackageName:         pkgName,
				PackageType:         pkgType,
			})
		}

		h.writeJSON(r, w, map[string]any{jsonKeyPkgDetailsList: outList})

		return true
	}

	return false
}

// handlePackageRootRoutes handles /packages and /packages/ requests.
func (h *Handler) handlePackageRootRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	// POST /packages → CreatePackage
	case http.MethodPost:
		body, err := httputils.ReadBody(r)
		if err != nil {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

			return
		}
		var req struct {
			PackageSource            *packageSourceJSON            `json:"PackageSource,omitempty"`
			PackageEncryptionOptions *packageEncryptionOptionsJSON `json:"PackageEncryptionOptions,omitempty"`
			PackageName              string                        `json:"PackageName"`
			PackageType              string                        `json:"PackageType"`
			PackageDescription       string                        `json:"PackageDescription"`
		}
		if len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
		var pkgSource *PackageSource
		if req.PackageSource != nil {
			pkgSource = &PackageSource{
				S3BucketName: req.PackageSource.S3BucketName,
				S3Key:        req.PackageSource.S3Key,
			}
		}
		var pkgEncOpts *PackageEncryptionOptions
		if req.PackageEncryptionOptions != nil {
			pkgEncOpts = &PackageEncryptionOptions{
				KmsKeyIdentifier:  req.PackageEncryptionOptions.KmsKeyIdentifier,
				EncryptionEnabled: req.PackageEncryptionOptions.EncryptionEnabled,
			}
		}
		pkg, createErr := h.Backend.CreatePackage(
			req.PackageName,
			req.PackageType,
			req.PackageDescription,
			pkgSource,
			pkgEncOpts,
		)
		if createErr != nil {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", createErr.Error())

			return
		}
		h.writeJSON(r, w, map[string]any{jsonKeyPackageDetails: pkg})
	default:
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", "route not found")
	}
}

// handlePackageIDRoutes handles /packages/{packageId} requests.
func (h *Handler) handlePackageIDRoutes(w http.ResponseWriter, r *http.Request, rest string) {
	pkgID := strings.TrimPrefix(rest, "/")
	if strings.Contains(pkgID, "/") {
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", "route not found")

		return
	}

	switch r.Method {
	// DELETE /packages/{packageId} → DeletePackage
	case http.MethodDelete:
		pkg, err := h.Backend.DeletePackage(pkgID)
		if err != nil {
			switch {
			case errors.Is(err, ErrPackageAssociated):
				h.writeError(r, w, http.StatusConflict, "ConflictException", err.Error())
			default:
				h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", err.Error())
			}

			return
		}
		h.writeJSON(r, w, map[string]any{jsonKeyPackageDetails: pkg})
	default:
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", "route not found")
	}
}

// associatePackageOutput is the JSON response for AssociatePackage.
type associatePackageOutput struct {
	DomainPackageDetails domainPackageDetailsJSON `json:"DomainPackageDetails"`
}

// domainPackageDetailsJSON is the JSON representation of package domain details
// (types.DomainPackageDetails).
type domainPackageDetailsJSON struct {
	PackageID           string  `json:"PackageID"`
	DomainName          string  `json:"DomainName"`
	DomainPackageStatus string  `json:"DomainPackageStatus"`
	PackageName         string  `json:"PackageName,omitempty"`
	PackageType         string  `json:"PackageType,omitempty"`
	LastUpdated         float64 `json:"LastUpdated,omitempty"`
}

// Converts a DomainPackageDetails into the wire-shape types.DomainPackageDetails
// object.
func toDomainPackageDetailsJSON(d *DomainPackageDetails) domainPackageDetailsJSON {
	return domainPackageDetailsJSON{
		PackageID:           d.PackageID,
		DomainName:          d.DomainName,
		DomainPackageStatus: d.State,
		PackageName:         d.PackageName,
		PackageType:         d.PackageType,
		LastUpdated:         d.LastUpdated,
	}
}

func (h *Handler) handleAssociatePackage(
	w http.ResponseWriter,
	r *http.Request,
	packageID, domainName string,
) {
	details, err := h.Backend.AssociatePackage(packageID, domainName)
	if err != nil {
		if errors.Is(err, ErrDomainNotFound) || errors.Is(err, ErrPackageNotFound) {
			h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", err.Error())
		} else {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", err.Error())
		}

		return
	}

	h.writeJSON(r, w, associatePackageOutput{
		DomainPackageDetails: toDomainPackageDetailsJSON(details),
	})
}

// associatePackagesRequest is the JSON request body for AssociatePackages.
type associatePackagesRequest struct {
	DomainName  string            `json:"DomainName"`
	PackageList []packageForAssoc `json:"PackageList"`
}

// packageForAssoc is a package entry in AssociatePackages request.
type packageForAssoc struct {
	PackageID string `json:"PackageID"`
}

// associatePackagesOutput is the JSON response for AssociatePackages.
type associatePackagesOutput struct {
	DomainPackageDetailsList []domainPackageDetailsJSON `json:"DomainPackageDetailsList"`
}

func (h *Handler) handleAssociatePackages(w http.ResponseWriter, r *http.Request) {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return
	}

	var req associatePackagesRequest
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "invalid JSON body")

		return
	}

	packageIDs := make([]string, 0, len(req.PackageList))
	for _, p := range req.PackageList {
		packageIDs = append(packageIDs, p.PackageID)
	}

	details, assocErr := h.Backend.AssociatePackages(req.DomainName, packageIDs)
	if assocErr != nil {
		if errors.Is(assocErr, ErrDomainNotFound) {
			h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", assocErr.Error())
		} else {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", assocErr.Error())
		}

		return
	}

	outList := make([]domainPackageDetailsJSON, 0, len(details))
	for i := range details {
		outList = append(outList, toDomainPackageDetailsJSON(&details[i]))
	}

	h.writeJSON(r, w, associatePackagesOutput{DomainPackageDetailsList: outList})
}
