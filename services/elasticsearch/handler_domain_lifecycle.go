package elasticsearch

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// cancelSoftwareUpdateRequest is the JSON body for CancelElasticsearchServiceSoftwareUpdate.
type cancelSoftwareUpdateRequest struct {
	DomainName string `json:"DomainName"`
}

// serviceSoftwareOptionsJSON is the JSON representation of software update options.
type serviceSoftwareOptionsJSON struct {
	CurrentVersion      string `json:"CurrentVersion"`
	NewVersion          string `json:"NewVersion"`
	UpdateStatus        string `json:"UpdateStatus"`
	Description         string `json:"Description"`
	AutomatedUpdateDate string `json:"AutomatedUpdateDate"`
	UpdateAvailable     bool   `json:"UpdateAvailable"`
	Cancellable         bool   `json:"Cancellable"`
	OptionalDeployment  bool   `json:"OptionalDeployment"`
}

// cancelSoftwareUpdateOutput is the response for CancelElasticsearchServiceSoftwareUpdate.
type cancelSoftwareUpdateOutput struct {
	ServiceSoftwareOptions serviceSoftwareOptionsJSON `json:"ServiceSoftwareOptions"`
}

func (h *Handler) handleCancelElasticsearchServiceSoftwareUpdate(w http.ResponseWriter, r *http.Request) {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return
	}

	var req cancelSoftwareUpdateRequest
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "invalid JSON body")

		return
	}

	_, cancelErr := h.Backend.CancelElasticsearchServiceSoftwareUpdate(h.reqContext(r), req.DomainName)
	if cancelErr != nil {
		if errors.Is(cancelErr, ErrDomainNotFound) {
			h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", cancelErr.Error())
		} else {
			h.writeError(r, w, http.StatusInternalServerError, "InternalException", cancelErr.Error())
		}

		return
	}

	h.writeJSON(r, w, cancelSoftwareUpdateOutput{
		ServiceSoftwareOptions: serviceSoftwareOptionsJSON{
			UpdateAvailable: false,
			Cancellable:     false,
			UpdateStatus:    "NOT_ELIGIBLE",
			Description:     "No software update scheduled",
		},
	})
}

func (h *Handler) handleDeleteElasticsearchServiceRole(w http.ResponseWriter, r *http.Request) {
	if err := h.Backend.DeleteElasticsearchServiceRole(); err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleStartElasticsearchServiceSoftwareUpdate(w http.ResponseWriter, r *http.Request) {
	var req cancelSoftwareUpdateRequest
	if !h.decodeRequest(w, r, &req) {
		return
	}

	if _, err := h.Backend.StartElasticsearchServiceSoftwareUpdate(h.reqContext(r), req.DomainName); err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	h.writeJSON(r, w, map[string]any{"ServiceSoftwareOptions": map[string]any{
		"UpdateStatus": "PENDING_UPDATE",
		"Cancellable":  true,
	}})
}

func (h *Handler) handleGetUpgradeHistory(w http.ResponseWriter, r *http.Request) {
	domainName := pathID(r.URL.Path, elasticsearchUpgradeDomain+"/", "/history")
	if err := h.Backend.GetUpgradeHistory(h.reqContext(r), domainName); err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	h.writeJSON(r, w, map[string]any{"UpgradeHistories": []any{}})
}

func (h *Handler) handleGetUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	domainName := pathID(r.URL.Path, elasticsearchUpgradeDomain+"/", "/status")
	if err := h.Backend.GetUpgradeStatus(h.reqContext(r), domainName); err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	h.writeJSON(r, w, map[string]any{"UpgradeStep": "UPGRADE", "StepStatus": "SUCCEEDED"})
}

func (h *Handler) handleUpgradeElasticsearchDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainName       string `json:"DomainName"`
		TargetVersion    string `json:"TargetVersion"`
		PerformCheckOnly bool   `json:"PerformCheckOnly"`
	}
	if !h.decodeRequest(w, r, &req) {
		return
	}

	ctx := h.reqContext(r)
	if !req.PerformCheckOnly {
		if _, err := h.Backend.UpgradeElasticsearchDomain(ctx, req.DomainName, req.TargetVersion); err != nil {
			h.writeOperationError(r, w, err)

			return
		}
	} else if _, err := h.Backend.DescribeDomain(ctx, req.DomainName); err != nil {
		h.writeOperationError(r, w, err)

		return
	}

	h.writeJSON(r, w, req)
}

func (h *Handler) handleDescribeElasticsearchInstanceTypeLimits(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(r, w, map[string]any{"LimitsByRole": map[string]any{
		"data": map[string]any{"InstanceLimits": map[string]any{"InstanceCountLimits": map[string]any{
			"MinimumInstanceCount": minimumInstanceCount,
			"MaximumInstanceCount": maximumInstanceCount,
		}}},
	}})
}

func (h *Handler) handleListElasticsearchInstanceTypes(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(r, w, map[string]any{"ElasticsearchInstanceTypes": []string{
		defaultInstanceType,
		largeInstanceType,
	}})
}
