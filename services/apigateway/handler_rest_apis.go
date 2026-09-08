package apigateway

import (
	"encoding/json"
	"net/http"
)

type createRestAPIInput = CreateRestAPIInput

type deleteRestAPIInput struct {
	RestAPIID string `json:"restApiId"`
}

type getRestAPIInput struct {
	RestAPIID string `json:"restApiId"`
}

type getRestApisInput struct {
	Position string `json:"position"`
	Limit    int    `json:"limit"`
}

type updateRestAPIHandlerInput struct {
	RestAPIID string `json:"restApiId"`
	UpdateRestAPIInput
}

func (h *Handler) restAPIActions() map[string]actionFn {
	return map[string]actionFn{
		opCreateRestAPI: h.createRestAPIAction,
		opDeleteRestAPI: h.deleteRestAPIAction,
		opGetRestAPI:    h.getRestAPIAction,
		opGetRestApis:   h.getRestAPIsAction,
		opUpdateRestAPI: h.updateRestAPIAction,
	}
}

func (h *Handler) createRestAPIAction(b []byte) (int, any, error) {
	var input createRestAPIInput
	if err := json.Unmarshal(b, &input); err != nil {
		return 0, nil, err
	}
	api, err := h.Backend.CreateRestAPI(input)
	if err != nil {
		return 0, nil, err
	}

	return http.StatusCreated, api, nil
}

func (h *Handler) deleteRestAPIAction(b []byte) (int, any, error) {
	var input deleteRestAPIInput
	if err := json.Unmarshal(b, &input); err != nil {
		return 0, nil, err
	}
	if err := h.Backend.DeleteRestAPI(input.RestAPIID); err != nil {
		return 0, nil, err
	}

	// Evict the cached routing trie -- otherwise every RestApi ID ever routed to
	// stays in h.trieCache forever, since fresh random IDs never reuse a deleted
	// entry's key for the cache to overwrite.
	h.trieCache.Delete(input.RestAPIID)

	return http.StatusAccepted, map[string]any{}, nil
}

func (h *Handler) getRestAPIAction(b []byte) (int, any, error) {
	var input getRestAPIInput
	if err := json.Unmarshal(b, &input); err != nil {
		return 0, nil, err
	}
	api, err := h.Backend.GetRestAPI(input.RestAPIID)
	if err != nil {
		return 0, nil, err
	}

	return http.StatusOK, api, nil
}

func (h *Handler) getRestAPIsAction(b []byte) (int, any, error) {
	var input getRestApisInput
	if err := json.Unmarshal(b, &input); err != nil {
		return 0, nil, err
	}
	apis, position, err := h.Backend.GetRestAPIs(input.Limit, input.Position)
	if err != nil {
		return 0, nil, err
	}
	if position != "" {
		return http.StatusOK, map[string]any{keyItem: apis, keyPosition: position}, nil
	}

	return http.StatusOK, map[string]any{keyItem: apis}, nil
}

func (h *Handler) updateRestAPIAction(b []byte) (int, any, error) {
	var input updateRestAPIHandlerInput
	if err := json.Unmarshal(b, &input); err != nil {
		return 0, nil, err
	}
	api, err := h.Backend.UpdateRestAPI(input.RestAPIID, input.UpdateRestAPIInput)
	if err != nil {
		return 0, nil, err
	}

	return http.StatusOK, api, nil
}
