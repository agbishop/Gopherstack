package medialive

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// --- Input handlers ---

// Tags first, SdiSources last: reduces GC pointer scan from 120 to 112 bytes.
type inputOutput struct {
	Tags       map[string]string `json:"tags"`
	Arn        string            `json:"arn"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	RoleArn    string            `json:"roleArn"`
	State      string            `json:"state"`
	SdiSources []string          `json:"sdiSources"`
}

func toInputOutput(inp *Input) inputOutput {
	tags := inp.Tags
	if tags == nil {
		tags = map[string]string{}
	}

	sdiSources := inp.SdiSources
	if sdiSources == nil {
		sdiSources = []string{}
	}

	return inputOutput{
		Tags:       tags,
		SdiSources: sdiSources,
		Arn:        inp.ARN,
		ID:         inp.ID,
		Name:       inp.Name,
		Type:       inp.InputType,
		RoleArn:    inp.RoleARN,
		State:      inp.State,
	}
}

func (h *Handler) handleCreateInput(c *echo.Context, body map[string]any) error {
	name, _ := body["name"].(string)
	inputType, _ := body["type"].(string)
	roleArn, _ := body["roleArn"].(string)
	sdiSources := extractStringSlice(body, "sdiSources")
	tags := extractTags(body)

	inp, err := h.Backend.CreateInput(name, inputType, roleArn, sdiSources, tags)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusCreated, map[string]any{keyInput: toInputOutput(inp)})
}

func (h *Handler) handleDescribeInput(c *echo.Context, inputID string) error {
	inp, err := h.Backend.DescribeInput(inputID)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, toInputOutput(inp))
}

func (h *Handler) handleUpdateInput(c *echo.Context, inputID string, body map[string]any) error {
	name, _ := body["name"].(string)
	roleArn, _ := body["roleArn"].(string)
	sdiSources := extractStringSlice(body, "sdiSources")
	_, sdiSourcesSet := body["sdiSources"]

	inp, err := h.Backend.UpdateInput(inputID, name, roleArn, sdiSources, sdiSourcesSet)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyInput: toInputOutput(inp)})
}

func (h *Handler) handleDeleteInput(c *echo.Context, inputID string) error {
	if err := h.Backend.DeleteInput(inputID); err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleListInputs(c *echo.Context) error {
	maxResults, nextTokenParam := paginationParams(c)
	summaries, nextToken, err := h.Backend.ListInputs(maxResults, nextTokenParam)
	if err != nil {
		return respondErr(c, err)
	}

	out := make([]map[string]any, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, map[string]any{
			keyArn:   s.ARN,
			keyID:    s.ID,
			keyName:  s.Name,
			"type":   s.InputType,
			keyState: s.State,
		})
	}

	resp := map[string]any{"inputs": out}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// --- Partner input handler ---

func (h *Handler) handleCreatePartnerInput(
	c *echo.Context,
	inputID string,
	body map[string]any,
) error {
	tags := extractTags(body)

	inp, err := h.Backend.CreatePartnerInput(inputID, tags)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusCreated, map[string]any{keyInput: toInputOutput(inp)})
}
