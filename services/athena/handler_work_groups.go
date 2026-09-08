package athena

import "encoding/json"

type listWorkGroupsInput struct {
	NextToken  string `json:"NextToken"`
	MaxResults int    `json:"MaxResults"`
}

type createWorkGroupInput struct {
	Name          string                 `json:"Name"`
	Description   string                 `json:"Description"`
	State         string                 `json:"State"`
	Tags          []Tag                  `json:"Tags"`
	Configuration WorkGroupConfiguration `json:"Configuration"`
}

type updateWorkGroupInput struct {
	ConfigurationUpdates *WorkGroupConfigurationUpdates `json:"ConfigurationUpdates"`
	WorkGroup            string                         `json:"WorkGroup"`
	Description          string                         `json:"Description"`
	State                string                         `json:"State"`
}

type deleteWorkGroupInput struct {
	WorkGroup             string `json:"WorkGroup"`
	RecursiveDeleteOption bool   `json:"RecursiveDeleteOption"`
}

type getWorkGroupInput struct {
	WorkGroup string `json:"WorkGroup"`
}

func (h *Handler) workGroupOps() map[string]athenaActionFn {
	return map[string]athenaActionFn{
		"CreateWorkGroup": func(b []byte) (any, error) {
			var input createWorkGroupInput
			if err := json.Unmarshal(b, &input); err != nil {
				return nil, err
			}

			return struct{}{}, h.Backend.CreateWorkGroup(
				input.Name,
				input.Description,
				input.State,
				input.Configuration,
				tagsFromSlice(input.Tags),
			)
		},
		"GetWorkGroup": func(b []byte) (any, error) {
			var input getWorkGroupInput
			if err := json.Unmarshal(b, &input); err != nil {
				return nil, err
			}

			wg, err := h.Backend.GetWorkGroup(input.WorkGroup)
			if err != nil {
				return nil, err
			}

			return map[string]any{"WorkGroup": wg}, nil
		},
		"ListWorkGroups": func(b []byte) (any, error) {
			var input listWorkGroupsInput
			if err := json.Unmarshal(b, &input); err != nil {
				return nil, err
			}
			list, nextToken, err := h.Backend.ListWorkGroups(input.NextToken, input.MaxResults)
			if err != nil {
				return nil, err
			}
			resp := map[string]any{"WorkGroups": list}
			if nextToken != "" {
				resp["NextToken"] = nextToken
			}

			return resp, nil
		},
		"UpdateWorkGroup": func(b []byte) (any, error) {
			var input updateWorkGroupInput
			if err := json.Unmarshal(b, &input); err != nil {
				return nil, err
			}

			return struct{}{}, h.Backend.UpdateWorkGroup(
				input.WorkGroup, input.Description, input.State, input.ConfigurationUpdates,
			)
		},
		"DeleteWorkGroup": func(b []byte) (any, error) {
			var input deleteWorkGroupInput
			if err := json.Unmarshal(b, &input); err != nil {
				return nil, err
			}

			return struct{}{}, h.Backend.DeleteWorkGroup(input.WorkGroup, input.RecursiveDeleteOption)
		},
	}
}
