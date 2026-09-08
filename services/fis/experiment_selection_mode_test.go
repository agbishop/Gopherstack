package fis_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/fis"
)

func TestApplySelectionMode(t *testing.T) {
	t.Parallel()

	arns := []string{"arn:1", "arn:2", "arn:3", "arn:4"}

	tests := []struct {
		name string
		mode string
		want []string
	}{
		{name: "all", mode: "ALL", want: arns},
		{name: "empty mode", mode: "", want: arns},
		{name: "count", mode: "COUNT(2)", want: arns[:2]},
		{name: "count exceeds total", mode: "COUNT(10)", want: arns},
		{name: "count zero", mode: "COUNT(0)", want: []string{}},
		{name: "percent", mode: "PERCENT(50)", want: arns[:2]},
		{name: "percent rounds up", mode: "PERCENT(30)", want: arns[:2]},
		{name: "percent 100", mode: "PERCENT(100)", want: arns},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := fis.ApplySelectionModeForTest(arns, tc.mode)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestStartExperiment_SelectionMode_ScopesTargetARNs is a regression test for
// gopherstack-0u4: ExperimentTemplateTarget.SelectionMode ("Scopes the identified
// resources to a specific count or percentage" -- aws-sdk-go-v2/service/fis@v1.40.4
// types/types.go:888) was validated and echoed on the wire but never consulted when
// resolving which target ARNs a real fault-injection action provider receives. A
// COUNT(2) target with 4 resourceArns must scope the provider call to exactly 2 ARNs.
func TestStartExperiment_SelectionMode_ScopesTargetARNs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	mock := &fis.MockFISActionProvider{
		Definitions: []service.FISActionDefinition{
			{ActionID: "aws:test:mode-action", TargetType: "aws:ec2:instance"},
		},
	}
	h.SetActionProviders([]service.FISActionProvider{mock})

	body := map[string]any{
		"roleArn":        "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{{"source": "none"}},
		"targets": map[string]any{
			"MyInstances": map[string]any{
				"resourceType":  "aws:ec2:instance",
				"selectionMode": "COUNT(2)",
				"resourceArns": []string{
					"arn:aws:ec2:us-east-1:000:instance/i-1",
					"arn:aws:ec2:us-east-1:000:instance/i-2",
					"arn:aws:ec2:us-east-1:000:instance/i-3",
					"arn:aws:ec2:us-east-1:000:instance/i-4",
				},
			},
		},
		"actions": map[string]any{
			"modeAction": map[string]any{
				"actionId": "aws:test:mode-action",
				"targets":  map[string]string{"Instances": "MyInstances"},
			},
		},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	var tplResp struct {
		ExperimentTemplate struct {
			ID string `json:"id"`
		} `json:"experimentTemplate"`
	}

	mustJSON(t, rec, &tplResp)

	rec2 := doRequest(t, h, http.MethodPost, "/experiments", map[string]any{
		"experimentTemplateId": tplResp.ExperimentTemplate.ID,
	})
	require.Equal(t, http.StatusCreated, rec2.Code)

	var startResp struct {
		Experiment struct {
			ID string `json:"id"`
		} `json:"experiment"`
	}

	mustJSON(t, rec2, &startResp)

	pollExperimentUntilTerminal(t, h, startResp.Experiment.ID)

	require.Len(t, mock.Execs, 1)
	assert.Len(t, mock.Execs[0].Targets, 2, "COUNT(2) selectionMode must scope the action provider call to 2 ARNs")
}
