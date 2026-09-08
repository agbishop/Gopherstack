package elasticsearch

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// packageSourceJSON is the JSON representation of a package's S3 source
// location (types.PackageSource).
type packageSourceJSON struct {
	S3BucketName string `json:"S3BucketName"`
	S3Key        string `json:"S3Key"`
}

// createPackageRequest is the JSON body for CreatePackage.
type createPackageRequest struct {
	PackageSource      *packageSourceJSON `json:"PackageSource"`
	PackageName        string             `json:"PackageName"`
	PackageType        string             `json:"PackageType"`
	PackageDescription string             `json:"PackageDescription"`
}

// packageErrorDetailsJSON mirrors types.ErrorDetails.
type packageErrorDetailsJSON struct {
	ErrorMessage string `json:"ErrorMessage,omitempty"`
	ErrorType    string `json:"ErrorType,omitempty"`
}

// packageJSON is the JSON representation of an Elasticsearch package
// (types.PackageDetails). CreatedAt/LastUpdatedAt are epoch-seconds,
// matching restjson1's unixTimestamp wire format. ErrorDetails is always
// omitted: this backend has no COPYING/COPY_FAILED state machine (packages
// always transition straight to AVAILABLE), and real AWS only populates
// ErrorDetails when a package is in COPY_FAILED.
type packageJSON struct {
	ErrorDetails       *packageErrorDetailsJSON `json:"ErrorDetails,omitempty"`
	PackageID          string                   `json:"PackageID"`
	PackageName        string                   `json:"PackageName"`
	PackageType        string                   `json:"PackageType"`
	PackageDescription string                   `json:"PackageDescription"`
	PackageStatus      string                   `json:"PackageStatus"`
	CreatedAt          float64                  `json:"CreatedAt,omitempty"`
	LastUpdatedAt      float64                  `json:"LastUpdatedAt,omitempty"`
}

// createPackageOutput is the response for CreatePackage.
type createPackageOutput struct {
	PackageDetails packageJSON `json:"PackageDetails"`
}

func (h *Handler) handleCreatePackage(w http.ResponseWriter, r *http.Request) {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return
	}

	var req createPackageRequest
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "invalid JSON body")

		return
	}

	var source PackageSource
	if req.PackageSource != nil {
		source = PackageSource{S3BucketName: req.PackageSource.S3BucketName, S3Key: req.PackageSource.S3Key}
	}

	pkg, createErr := h.Backend.CreatePackage(
		h.reqContext(r), req.PackageName, req.PackageType, req.PackageDescription, source,
	)
	if createErr != nil {
		if errors.Is(createErr, ErrDomainAlreadyExists) {
			h.writeError(r, w, http.StatusConflict, "ResourceAlreadyExistsException", createErr.Error())
		} else {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", createErr.Error())
		}

		return
	}

	h.writeJSON(r, w, createPackageOutput{PackageDetails: toPackageJSON(pkg)})
}

func toPackageJSON(p *Package) packageJSON {
	out := packageJSON{
		PackageID:          p.ID,
		PackageName:        p.Name,
		PackageType:        p.PackageType,
		PackageDescription: p.Description,
		PackageStatus:      p.Status,
		CreatedAt:          awstime.Epoch(p.CreatedAt),
		LastUpdatedAt:      awstime.Epoch(p.LastUpdatedAt),
	}

	if p.ErrorDetails != nil {
		out.ErrorDetails = &packageErrorDetailsJSON{
			ErrorMessage: p.ErrorDetails.ErrorMessage,
			ErrorType:    p.ErrorDetails.ErrorType,
		}
	}

	return out
}

// associatePackageOutput is the response for AssociatePackage.
type associatePackageOutput struct {
	DomainPackageDetails domainPackageJSON `json:"DomainPackageDetails"`
}

type domainPackageJSON struct {
	PackageID           string `json:"PackageID"`
	PackageName         string `json:"PackageName,omitempty"`
	DomainName          string `json:"DomainName"`
	PackageType         string `json:"PackageType,omitempty"`
	DomainPackageStatus string `json:"DomainPackageStatus"`
}

// associatePackagePathParts is the expected number of path segments after /associate/.
const associatePackagePathParts = 2

func (h *Handler) handleAssociatePackage(w http.ResponseWriter, r *http.Request) {
	// Path: /2015-01-01/packages/associate/{packageID}/{domainName}
	rest := strings.TrimPrefix(r.URL.Path, elasticsearchPackages+"/associate/")
	parts := strings.SplitN(rest, "/", associatePackagePathParts)

	if len(parts) != associatePackagePathParts {
		h.writeError(
			r,
			w,
			http.StatusBadRequest,
			"ValidationException",
			"invalid path: expected /associate/{packageID}/{domainName}",
		)

		return
	}

	packageID, domainName := parts[0], parts[1]

	if assocErr := h.Backend.AssociatePackage(h.reqContext(r), packageID, domainName); assocErr != nil {
		switch {
		case errors.Is(assocErr, ErrDomainNotFound) || errors.Is(assocErr, ErrPackageNotFound):
			h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", assocErr.Error())
		case errors.Is(assocErr, ErrPackageAlreadyAssociated):
			h.writeError(r, w, http.StatusConflict, "ConflictException", assocErr.Error())
		default:
			h.writeError(r, w, http.StatusBadRequest, "ValidationException", assocErr.Error())
		}

		return
	}

	var out associatePackageOutput
	out.DomainPackageDetails.PackageID = packageID
	out.DomainPackageDetails.DomainName = domainName
	out.DomainPackageDetails.DomainPackageStatus = "ACTIVE"

	h.writeJSON(r, w, &out)
}

func (h *Handler) handleDissociatePackage(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, elasticsearchPackages+"/dissociate/")
	parts := strings.SplitN(rest, "/", associatePackagePathParts)
	if len(parts) != associatePackagePathParts {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "invalid package dissociation path")

		return
	}

	if err := h.Backend.DissociatePackage(h.reqContext(r), parts[0], parts[1]); err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	h.writeJSON(r, w, map[string]any{"DomainPackageDetails": map[string]any{
		"PackageID":           parts[0],
		"DomainName":          parts[1],
		"DomainPackageStatus": "DISSOCIATING",
	}})
}

// describePackagesFilter is the wire shape of types.DescribePackagesFilter --
// Name/Value, not the Name/Values shape nameValuesFilter covers for the
// cross-cluster-connection Describe ops (verified against DescribePackages's
// own serializeOpDocument, api_op_DescribePackages.go's Input doc comment).
type describePackagesFilter struct {
	Name  string   `json:"Name"`
	Value []string `json:"Value"`
}

func (h *Handler) handleDescribePackages(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NextToken  string                   `json:"NextToken"`
		Filters    []describePackagesFilter `json:"Filters"`
		MaxResults int                      `json:"MaxResults"`
	}
	if !h.decodeRequest(w, r, &req) {
		return
	}

	packages := h.Backend.DescribePackages(h.reqContext(r), nil)
	matched := make([]*Package, 0, len(packages))
	for _, pkg := range packages {
		if matchesDescribePackagesFilters(req.Filters, pkg) {
			matched = append(matched, pkg)
		}
	}

	pg := page.New(matched, req.NextToken, req.MaxResults, defaultCrossClusterPageSize)
	result := make([]packageJSON, 0, len(pg.Data))
	for _, pkg := range pg.Data {
		result = append(result, toPackageJSON(pkg))
	}

	resp := map[string]any{"PackageDetailsList": result}
	if pg.Next != "" {
		resp["NextToken"] = pg.Next
	}

	h.writeJSON(r, w, resp)
}

// matchesDescribePackagesFilters applies DescribePackages's Filters
// parameter -- Name is one of PackageID/PackageName/PackageStatus
// (types.DescribePackagesFilterName's three enum values), matched against
// any of Value's entries.
func matchesDescribePackagesFilters(filters []describePackagesFilter, pkg *Package) bool {
	for _, f := range filters {
		var value string

		switch f.Name {
		case "PackageID":
			value = pkg.ID
		case "PackageName":
			value = pkg.Name
		case "PackageStatus":
			value = pkg.Status
		default:
			return false
		}

		if !slices.Contains(f.Value, value) {
			return false
		}
	}

	return true
}

func (h *Handler) handleUpdatePackage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PackageID          string `json:"PackageID"`
		PackageDescription string `json:"PackageDescription"`
	}
	if !h.decodeRequest(w, r, &req) {
		return
	}

	pkg, err := h.Backend.UpdatePackage(h.reqContext(r), req.PackageID, req.PackageDescription)
	if err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	h.writeJSON(r, w, map[string]any{"PackageDetails": toPackageJSON(pkg)})
}

func (h *Handler) handleDeletePackage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, elasticsearchPackages+"/")
	pkg, err := h.Backend.DeletePackage(h.reqContext(r), id)
	if err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	h.writeJSON(r, w, map[string]any{"PackageDetails": toPackageJSON(pkg)})
}

func (h *Handler) handleGetPackageVersionHistory(w http.ResponseWriter, r *http.Request) {
	id := pathID(r.URL.Path, elasticsearchPackages+"/", "/history")
	packages, err := h.Backend.GetPackageVersionHistory(h.reqContext(r), id)
	if err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	history := make([]packageJSON, 0, len(packages))
	for _, pkg := range packages {
		history = append(history, toPackageJSON(pkg))
	}

	h.writeJSON(r, w, map[string]any{"PackageVersionHistoryList": history})
}

func (h *Handler) handleListDomainsForPackage(w http.ResponseWriter, r *http.Request) {
	id := pathID(r.URL.Path, elasticsearchPackages+"/", "/domains")
	domains, err := h.Backend.ListDomainsForPackage(h.reqContext(r), id)
	if err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	result := make([]domainPackageJSON, 0, len(domains))
	for _, domainName := range domains {
		result = append(result, domainPackageJSON{
			PackageID: id, DomainName: domainName, DomainPackageStatus: statusActive,
		})
	}

	h.writeJSON(r, w, map[string]any{"DomainPackageDetailsList": result})
}

func (h *Handler) handleListPackagesForDomain(w http.ResponseWriter, r *http.Request) {
	domainName := pathID(r.URL.Path, elasticsearchDomainPackages+"/", "/packages")

	packages, err := h.Backend.ListPackagesForDomain(h.reqContext(r), domainName)
	if err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	result := make([]domainPackageJSON, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, domainPackageJSON{
			PackageID:           pkg.ID,
			PackageName:         pkg.Name,
			PackageType:         pkg.PackageType,
			DomainName:          domainName,
			DomainPackageStatus: statusActive,
		})
	}

	h.writeJSON(r, w, map[string]any{"DomainPackageDetailsList": result})
}
