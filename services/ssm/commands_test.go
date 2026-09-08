package ssm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

func TestSendCommand_StatusMachine(t *testing.T) {
	t.Parallel()

	t.Run("command ends in Success after SendCommand", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		// Need a document.
		_, err := b.CreateDocument(ctx, &ssm.CreateDocumentInput{
			Name:    "My-Doc",
			Content: `{"schemaVersion":"2.2"}`,
		})
		require.NoError(t, err)

		out, err := b.SendCommand(ctx, &ssm.SendCommandInput{
			DocumentName: "My-Doc",
			InstanceIDs:  []string{"i-111", "i-222"},
		})
		require.NoError(t, err)
		assert.Equal(t, "Success", out.Command.Status,
			"no-op runner must complete synchronously")
		assert.Equal(t, "Success", out.Command.StatusDetails)
	})

	t.Run("invocations also end in Success", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		_, err := b.CreateDocument(ctx, &ssm.CreateDocumentInput{
			Name:    "My-Doc2",
			Content: `{"schemaVersion":"2.2"}`,
		})
		require.NoError(t, err)

		out, err := b.SendCommand(ctx, &ssm.SendCommandInput{
			DocumentName: "My-Doc2",
			InstanceIDs:  []string{"i-abc"},
		})
		require.NoError(t, err)

		cmdID := out.Command.CommandID

		invOut, err := b.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandID:  cmdID,
			InstanceID: "i-abc",
		})
		require.NoError(t, err)
		assert.Equal(t, "Success", invOut.Status)
	})

	t.Run("CancelCommand transitions from Pending to Cancelled", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		_, err := b.CreateDocument(ctx, &ssm.CreateDocumentInput{
			Name:    "Cancel-Doc",
			Content: `{"schemaVersion":"2.2"}`,
		})
		require.NoError(t, err)

		// First send a command (will end in Success).
		sendOut, err := b.SendCommand(ctx, &ssm.SendCommandInput{
			DocumentName: "Cancel-Doc",
			InstanceIDs:  []string{"i-x"},
		})
		require.NoError(t, err)

		cmdID := sendOut.Command.CommandID

		// CancelCommand on a Success command should succeed gracefully (AWS allows this).
		_, err = b.CancelCommand(ctx, &ssm.CancelCommandInput{CommandID: cmdID})
		require.NoError(t, err)
	})

	t.Run("handler returns CommandId in SendCommand response", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t)

		// AWS-RunShellScript is pre-registered.
		rec := doRequest(t, h, "SendCommand",
			`{"DocumentName":"AWS-RunShellScript","InstanceIds":["i-abc"]}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "CommandId")
	})
}

// TestSendCommand_NoWritePathExpiry verifies that SendCommand no longer prunes
// expired commands inline. Expiry is deferred to the janitor sweep.
func TestSendCommand_NoWritePathExpiry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		expireN int
	}{
		{name: "expired commands survive SendCommand", expireN: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			b := ssm.NewInMemoryBackend()

			ids := make([]string, tc.expireN)
			for i := range tc.expireN {
				out, err := b.SendCommand(ctx, &ssm.SendCommandInput{
					DocumentName: "AWS-RunShellScript",
					InstanceIDs:  []string{"i-abc"},
				})
				require.NoError(t, err)
				ids[i] = out.Command.CommandID
			}

			// Force commands into the past.
			for _, id := range ids {
				b.SetCommandExpiresAfter(id, 1)
			}

			// New SendCommand must not prune the expired ones.
			_, err := b.SendCommand(ctx, &ssm.SendCommandInput{
				DocumentName: "AWS-RunShellScript",
				InstanceIDs:  []string{"i-def"},
			})
			require.NoError(t, err)

			require.Equal(t, tc.expireN+1, b.CommandCount(),
				"SendCommand must not prune on the write path")

			// Janitor sweeps them.
			j := ssm.NewJanitor(b, 0)
			j.SweepOnce(ctx)

			require.Equal(t, 1, b.CommandCount(), "janitor must evict expired commands")
		})
	}
}
func TestSendCommand_OutputRendering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		doc        string
		wantStdout string
		wantStderr string
		wantStatus string
		commands   []string
	}{
		{
			name:       "shell_echo_single",
			doc:        "AWS-RunShellScript",
			commands:   []string{"echo hello"},
			wantStdout: "hello\n",
			wantStatus: "Success",
		},
		{
			name:       "shell_echo_quoted_and_plain",
			doc:        "AWS-RunShellScript",
			commands:   []string{"echo 'a b'", "echo c"},
			wantStdout: "a b\nc\n",
			wantStatus: "Success",
		},
		{
			name:       "shell_multiline_command",
			doc:        "AWS-RunShellScript",
			commands:   []string{"echo one\necho two"},
			wantStdout: "one\ntwo\n",
			wantStatus: "Success",
		},
		{
			name:       "shell_nonzero_exit_fails",
			doc:        "AWS-RunShellScript",
			commands:   []string{"echo before", "exit 3"},
			wantStdout: "before\n",
			wantStderr: "exit status 3",
			wantStatus: "Failed",
		},
		{
			name:       "powershell_write_host",
			doc:        "AWS-RunPowerShellScript",
			commands:   []string{"Write-Host hi"},
			wantStdout: "hi\n",
			wantStatus: "Success",
		},
		{
			name:       "powershell_echo_alias",
			doc:        "AWS-RunPowerShellScript",
			commands:   []string{"echo pow"},
			wantStdout: "pow\n",
			wantStatus: "Success",
		},
		{
			name:       "unknown_document_empty_output",
			doc:        "My-Custom-Doc",
			commands:   []string{"whatever"},
			wantStdout: "",
			wantStatus: "Success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			ctx := context.TODO()

			// SendCommand validates the document exists (AWS-accurate). Register
			// any non-default document used by the case first.
			if tt.doc != "AWS-RunShellScript" && tt.doc != "AWS-RunPowerShellScript" {
				_, err := b.CreateDocument(ctx, &ssm.CreateDocumentInput{
					Name:         tt.doc,
					Content:      `{"schemaVersion":"2.2","mainSteps":[]}`,
					DocumentType: "Command",
				})
				require.NoError(t, err)
			}

			out, err := b.SendCommand(ctx, &ssm.SendCommandInput{
				DocumentName: tt.doc,
				InstanceIDs:  []string{"i-0123456789abcdef0"},
				Parameters:   map[string][]string{"commands": tt.commands},
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, out.Command.Status,
				"command status must reflect script outcome")

			inv, err := b.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
				CommandID:  out.Command.CommandID,
				InstanceID: "i-0123456789abcdef0",
			})
			require.NoError(t, err)

			assert.Equal(t, tt.wantStdout, inv.StandardOutputContent,
				"stdout must be the rendered command output")
			assert.Equal(t, tt.wantStatus, inv.Status)

			if tt.wantStderr == "" {
				assert.Empty(t, inv.StandardErrorContent)
			} else {
				assert.Contains(t, inv.StandardErrorContent, tt.wantStderr)
			}
		})
	}
}
func TestSendCommand_InProgressObservable(t *testing.T) {
	t.Parallel()

	// A long exec delay keeps the command InProgress until forced complete,
	// making the InProgress window (and hidden output) observable.
	b := ssm.NewInMemoryBackend().WithCommandExecDelay(time.Hour)
	ctx := context.TODO()

	out, err := b.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: "AWS-RunShellScript",
		InstanceIDs:  []string{"i-1"},
		Parameters:   map[string][]string{"commands": {"echo hi"}},
	})
	require.NoError(t, err)
	require.Equal(t, "InProgress", out.Command.Status,
		"with an exec delay the command must be observable as InProgress")

	inv, err := b.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
		CommandID:  out.Command.CommandID,
		InstanceID: "i-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "InProgress", inv.Status)
	assert.Empty(t, inv.StandardOutputContent, "output must be hidden while InProgress")

	// Force completion (as elapsed time would) and confirm output is revealed.
	b.ForceCompleteCommands()

	inv, err = b.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
		CommandID:  out.Command.CommandID,
		InstanceID: "i-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "Success", inv.Status)
	assert.Equal(t, "hi\n", inv.StandardOutputContent)
}
func TestSendCommand_InProgressCompletesAfterDelay(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend().WithCommandExecDelay(60 * time.Millisecond)
	ctx := context.TODO()

	out, err := b.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: "AWS-RunShellScript",
		InstanceIDs:  []string{"i-1"},
		Parameters:   map[string][]string{"commands": {"echo done"}},
	})
	require.NoError(t, err)
	require.Equal(t, "InProgress", out.Command.Status)

	require.Eventually(t, func() bool {
		inv, gErr := b.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandID:  out.Command.CommandID,
			InstanceID: "i-1",
		})

		return gErr == nil && inv.Status == "Success" && inv.StandardOutputContent == "done\n"
	}, 2*time.Second, 10*time.Millisecond,
		"command must complete and reveal output once the delay elapses")
}
func TestGetCommandInvocation_OutputFields(t *testing.T) {
	t.Parallel()
	h := newHandler()

	postJSON(t, h, "CreateDocument", map[string]any{
		"Name":    "MyDoc",
		"Content": `{"schemaVersion":"2.2"}`,
	})

	_, sendOut := postJSON(t, h, "SendCommand", map[string]any{
		"DocumentName":       "MyDoc",
		"DocumentVersion":    "2",
		"InstanceIds":        []string{"i-abc"},
		"Comment":            "run now",
		"OutputS3BucketName": "my-bucket",
		"OutputS3KeyPrefix":  "logs/",
		"TimeoutSeconds":     600,
	})

	cmd := sendOut["Command"].(map[string]any)
	cmdID := cmd["CommandId"].(string)
	assert.Equal(t, "my-bucket", cmd["OutputS3BucketName"])
	assert.InEpsilon(t, float64(600), cmd["TimeoutSeconds"], 0.01)

	code, invOut := postJSON(t, h, "GetCommandInvocation", map[string]any{
		"CommandId":  cmdID,
		"InstanceId": "i-abc",
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "run now", invOut["Comment"])
	assert.Equal(t, "2", invOut["DocumentVersion"])
	assert.NotEmpty(t, invOut["ExecutionStartDateTime"])
}
func TestCancelCommand_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{name: "cancels_existing_command", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)

			sendRec := doRequest(
				t,
				h,
				"SendCommand",
				`{"DocumentName":"AWS-RunShellScript","Parameters":{"commands":["echo hi"]}}`,
			)
			require.Equal(t, http.StatusOK, sendRec.Code)

			var sendResp map[string]any
			require.NoError(t, json.Unmarshal(sendRec.Body.Bytes(), &sendResp))

			cmd, ok := sendResp["Command"].(map[string]any)
			require.True(t, ok)
			cmdID := cmd["CommandId"].(string)

			body := `{"CommandId":"` + cmdID + `"}`
			rec := doRequest(t, h, "CancelCommand", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
func TestCancelCommand_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantErrMsg string
		wantStatus int
	}{
		{
			name:       "unknown_command_id",
			body:       `{"CommandId":"nonexistent-id"}`,
			wantStatus: http.StatusBadRequest,
			wantErrMsg: "InvalidCommandId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			rec := doRequest(t, h, "CancelCommand", tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantErrMsg)
		})
	}
}

// TestCancelCommand_CancelsInvocations verifies invocations are also cancelled.
func TestCancelCommand_CancelsInvocations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		wantInvocStatus string
		instanceIDs     []string
	}{
		{
			name:            "cancels_single_invocation",
			instanceIDs:     []string{"i-001"},
			wantInvocStatus: "Cancelled",
		},
		{
			name:            "cancels_multiple_invocations",
			instanceIDs:     []string{"i-001", "i-002", "i-003"},
			wantInvocStatus: "Cancelled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)

			instanceIDsJSON, _ := json.Marshal(tt.instanceIDs)
			sendBody := `{"DocumentName":"AWS-RunShellScript","InstanceIds":` + string(instanceIDsJSON) + `}`
			sendRec := doRequest(t, h, "SendCommand", sendBody)
			require.Equal(t, http.StatusOK, sendRec.Code)

			var sendResp map[string]any
			require.NoError(t, json.Unmarshal(sendRec.Body.Bytes(), &sendResp))
			cmd := sendResp["Command"].(map[string]any)
			cmdID := cmd["CommandId"].(string)

			cancelRec := doRequest(t, h, "CancelCommand", `{"CommandId":"`+cmdID+`"}`)
			require.Equal(t, http.StatusOK, cancelRec.Code)

			for _, instanceID := range tt.instanceIDs {
				invRec := doRequest(t, h, "GetCommandInvocation",
					`{"CommandId":"`+cmdID+`","InstanceId":"`+instanceID+`"}`)
				require.Equal(t, http.StatusOK, invRec.Code)

				var invResp map[string]any
				require.NoError(t, json.Unmarshal(invRec.Body.Bytes(), &invResp))
				assert.Equal(t, tt.wantInvocStatus, invResp["Status"])
				assert.Equal(t, tt.wantInvocStatus, invResp["StatusDetails"])
			}
		})
	}
}

// TestCancelCommand_ScopedToInstanceIDs proves CancelCommandInput.InstanceIds
// (api_op_CancelCommand.go: "If not provided, the command is canceled on
// every node on which it was requested") actually scopes cancellation --
// previously the field was read into the struct but never applied, so
// CancelCommand always cancelled every invocation regardless of InstanceIds.
func TestCancelCommand_ScopedToInstanceIDs(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)

	sendRec := doRequest(
		t, h, "SendCommand",
		`{"DocumentName":"AWS-RunShellScript","InstanceIds":["i-001","i-002"]}`,
	)
	require.Equal(t, http.StatusOK, sendRec.Code)

	var sendResp map[string]any
	require.NoError(t, json.Unmarshal(sendRec.Body.Bytes(), &sendResp))
	cmdID := sendResp["Command"].(map[string]any)["CommandId"].(string)

	cancelRec := doRequest(t, h, "CancelCommand", `{"CommandId":"`+cmdID+`","InstanceIds":["i-001"]}`)
	require.Equal(t, http.StatusOK, cancelRec.Code)

	invRec := doRequest(t, h, "GetCommandInvocation", `{"CommandId":"`+cmdID+`","InstanceId":"i-001"}`)
	require.Equal(t, http.StatusOK, invRec.Code)

	var cancelledInv map[string]any
	require.NoError(t, json.Unmarshal(invRec.Body.Bytes(), &cancelledInv))
	assert.Equal(t, "Cancelled", cancelledInv["Status"])

	otherRec := doRequest(t, h, "GetCommandInvocation", `{"CommandId":"`+cmdID+`","InstanceId":"i-002"}`)
	require.Equal(t, http.StatusOK, otherRec.Code)

	var otherInv map[string]any
	require.NoError(t, json.Unmarshal(otherRec.Body.Bytes(), &otherInv))
	assert.NotEqual(t, "Cancelled", otherInv["Status"], "un-named instance's invocation must not be cancelled")
}

// TestListCommands_FilterByInstanceID proves ListCommandsInput.InstanceId
// (api_op_ListCommands.go) actually filters -- previously the field existed
// on the struct but was never read in the handler body, so a real client's
// InstanceId filter silently returned every command regardless.
func TestListCommands_FilterByInstanceID(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	_, err := b.CreateDocument(context.TODO(), &ssm.CreateDocumentInput{
		Name: "AWS-RunShellScript", Content: `{"schemaVersion":"2.2"}`,
	})
	_ = err

	_, err = b.SendCommand(context.TODO(), &ssm.SendCommandInput{
		DocumentName: "AWS-RunShellScript", InstanceIDs: []string{"i-1111"},
	})
	require.NoError(t, err)

	_, err = b.SendCommand(context.TODO(), &ssm.SendCommandInput{
		DocumentName: "AWS-RunShellScript", InstanceIDs: []string{"i-2222"},
	})
	require.NoError(t, err)

	out, err := b.ListCommands(context.TODO(), &ssm.ListCommandsInput{InstanceID: "i-1111"})
	require.NoError(t, err)
	require.Len(t, out.Commands, 1)
	assert.Equal(t, []string{"i-1111"}, out.Commands[0].InstanceIDs)
}

// TestSendCommand_RoundTripsRealSDKFields proves DocumentVersion,
// ServiceRoleArn (echoed as Command.ServiceRole -- the input/output member
// names genuinely differ), MaxConcurrency and MaxErrors -- all real
// SendCommandInput/types.Command members with no prior Go struct member at
// all -- now round-trip, and that TargetCount/CompletedCount/ErrorCount are
// computed from the command's own invocations rather than left at zero.
func TestSendCommand_RoundTripsRealSDKFields(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	_, err := b.CreateDocument(context.TODO(), &ssm.CreateDocumentInput{
		Name: "AWS-RunShellScript", Content: `{"schemaVersion":"2.2"}`,
	})
	_ = err

	out, err := b.SendCommand(context.TODO(), &ssm.SendCommandInput{
		DocumentName:    "AWS-RunShellScript",
		DocumentVersion: "3",
		ServiceRoleArn:  "arn:aws:iam::000000000000:role/SSMRole",
		MaxConcurrency:  "50%",
		MaxErrors:       "0",
		InstanceIDs:     []string{"i-1111", "i-2222"},
	})
	require.NoError(t, err)

	assert.Equal(t, "3", out.Command.DocumentVersion)
	assert.Equal(t, "arn:aws:iam::000000000000:role/SSMRole", out.Command.ServiceRole)
	assert.Equal(t, "50%", out.Command.MaxConcurrency)
	assert.Equal(t, "0", out.Command.MaxErrors)
	assert.EqualValues(t, 2, out.Command.TargetCount)
	assert.EqualValues(t, 2, out.Command.CompletedCount)
	assert.EqualValues(t, 0, out.Command.ErrorCount)
}

func TestFull_Command_SendListGetCancel(t *testing.T) {
	t.Parallel()
	h := newHandler()

	// Need a document
	postJSON(t, h, "CreateDocument", map[string]any{
		"Name":    "RunMe",
		"Content": `{"schemaVersion":"2.2"}`,
	})

	// SendCommand
	code, out := postJSON(t, h, "SendCommand", map[string]any{
		"DocumentName": "RunMe",
		"InstanceIds":  []string{"i-001", "i-002"},
		"Comment":      "integration test",
		"Parameters":   map[string]any{"commands": []string{"echo hello"}},
	})
	assert.Equal(t, http.StatusOK, code)
	cmd := out["Command"].(map[string]any)
	cmdID := cmd["CommandId"].(string)
	assert.NotEmpty(t, cmdID)
	assert.Equal(t, "RunMe", cmd["DocumentName"])
	assert.Equal(t, "integration test", cmd["Comment"])

	// ListCommands
	code, out = postJSON(t, h, "ListCommands", map[string]any{})
	assert.Equal(t, http.StatusOK, code)
	commands := out["Commands"].([]any)
	assert.NotEmpty(t, commands)

	// ListCommands filter by CommandId
	code, out = postJSON(t, h, "ListCommands", map[string]any{"CommandId": cmdID})
	assert.Equal(t, http.StatusOK, code)
	commands = out["Commands"].([]any)
	assert.Len(t, commands, 1)

	// GetCommandInvocation
	code, out = postJSON(t, h, "GetCommandInvocation", map[string]any{
		"CommandId":  cmdID,
		"InstanceId": "i-001",
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, cmdID, out["CommandId"])
	assert.Equal(t, "integration test", out["Comment"])

	// ListCommandInvocations
	code, out = postJSON(t, h, "ListCommandInvocations", map[string]any{"CommandId": cmdID})
	assert.Equal(t, http.StatusOK, code)
	invocations := out["CommandInvocations"].([]any)
	assert.Len(t, invocations, 2)

	// CancelCommand
	code, _ = postJSON(t, h, "CancelCommand", map[string]any{"CommandId": cmdID})
	assert.Equal(t, http.StatusOK, code)

	// After cancel - command status should be Cancelled
	code, out = postJSON(t, h, "ListCommands", map[string]any{"CommandId": cmdID})
	assert.Equal(t, http.StatusOK, code)
	cancelledCmd := out["Commands"].([]any)[0].(map[string]any)
	assert.Equal(t, "Cancelled", cancelledCmd["Status"])
}
func TestFull_Command_MissingDocument(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, _ := postJSON(t, h, "SendCommand", map[string]any{
		"DocumentName": "NonExistentDoc",
		"InstanceIds":  []string{"i-001"},
	})
	assert.Equal(t, http.StatusBadRequest, code)
}
func TestFull_Command_ListByInstanceId(t *testing.T) {
	t.Parallel()
	h := newHandler()

	postJSON(t, h, "CreateDocument", map[string]any{
		"Name":    "DocForList",
		"Content": `{"schemaVersion":"2.2"}`,
	})

	_, out1 := postJSON(t, h, "SendCommand", map[string]any{
		"DocumentName": "DocForList",
		"InstanceIds":  []string{"i-target", "i-other"},
	})
	cmdID := out1["Command"].(map[string]any)["CommandId"].(string)

	code, out := postJSON(t, h, "ListCommandInvocations", map[string]any{
		"CommandId":  cmdID,
		"InstanceId": "i-target",
	})
	assert.Equal(t, http.StatusOK, code)
	invocations := out["CommandInvocations"].([]any)
	assert.Len(t, invocations, 1)
	assert.Equal(t, "i-target", invocations[0].(map[string]any)["InstanceId"])
}
func TestWithCommandTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ttl     time.Duration
		wantTTL float64
	}{
		{
			name:    "positive_ttl_applied",
			ttl:     30 * time.Minute,
			wantTTL: (30 * time.Minute).Seconds(),
		},
		{
			name:    "zero_ttl_ignored",
			ttl:     0,
			wantTTL: ssm.DefaultCommandExpirySecs,
		},
		{
			name:    "negative_ttl_ignored",
			ttl:     -5 * time.Second,
			wantTTL: ssm.DefaultCommandExpirySecs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend().WithCommandTTL(tt.ttl)
			assert.InDelta(t, tt.wantTTL, b.GetCommandExpirySecs(), 0.0001)
		})
	}
}

// TestListCommands_Filtered exercises ListCommands filtering and pagination.
func TestListCommands_Filtered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		maxResults *int64
		name       string
		wantCount  int
		filterByID bool
	}{
		{
			name:       "no_filter_all_returned",
			filterByID: false,
			wantCount:  2,
		},
		{
			name:       "filter_by_command_id",
			filterByID: true,
			wantCount:  1,
		},
		{
			name:       "pagination_max_results_1",
			filterByID: false,
			maxResults: func() *int64 {
				v := int64(1)

				return &v
			}(),
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			// Create a document for commands
			_, err := b.CreateDocument(context.TODO(), &ssm.CreateDocumentInput{
				Name:    "AWS-RunShellScript",
				Content: `{"schemaVersion":"2.2"}`,
			})
			// AWS-RunShellScript is a default doc, error is expected
			_ = err

			out1, err := b.SendCommand(context.TODO(), &ssm.SendCommandInput{
				DocumentName: "AWS-RunShellScript",
				InstanceIDs:  []string{"i-1111"},
			})
			require.NoError(t, err)

			_, err = b.SendCommand(context.TODO(), &ssm.SendCommandInput{
				DocumentName: "AWS-RunShellScript",
				InstanceIDs:  []string{"i-2222"},
			})
			require.NoError(t, err)

			var cmdID string
			if tt.filterByID {
				cmdID = out1.Command.CommandID
			}

			listInput := &ssm.ListCommandsInput{
				CommandID:  cmdID,
				MaxResults: tt.maxResults,
			}
			listOut, err := b.ListCommands(context.TODO(), listInput)
			require.NoError(t, err)
			assert.Len(t, listOut.Commands, tt.wantCount)
		})
	}
}

// TestListCommandInvocations_Filtered exercises ListCommandInvocations branches.
func TestListCommandInvocations_Filtered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		commandID  string
		instanceID string
		wantCount  int
	}{
		{
			name:      "all_invocations",
			wantCount: 2,
		},
		{
			name:       "filter_by_instance",
			instanceID: "i-1111",
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			out, err := b.SendCommand(context.TODO(), &ssm.SendCommandInput{
				DocumentName: "AWS-RunShellScript",
				InstanceIDs:  []string{"i-1111", "i-2222"},
			})
			require.NoError(t, err)

			listOut, err := b.ListCommandInvocations(context.TODO(), &ssm.ListCommandInvocationsInput{
				CommandID:  out.Command.CommandID,
				InstanceID: tt.instanceID,
			})
			require.NoError(t, err)
			assert.Len(t, listOut.CommandInvocations, tt.wantCount)
		})
	}
}

// TestListCommands_EmptyWithPagination covers the empty-result pagination branch.
func TestListCommands_EmptyWithPagination(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	out, err := b.ListCommands(context.TODO(), &ssm.ListCommandsInput{
		NextToken: "999",
	})
	require.NoError(t, err)
	assert.Empty(t, out.Commands)
}

// TestListCommandInvocations_EmptyPagination covers the empty-result pagination path.
func TestListCommandInvocations_EmptyPagination(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	out, err := b.ListCommandInvocations(context.TODO(), &ssm.ListCommandInvocationsInput{
		NextToken: "999",
	})
	require.NoError(t, err)
	assert.Empty(t, out.CommandInvocations)
}
func TestHandler_SendCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name: "success",
			body: `{"DocumentName":"AWS-RunShellScript",` +
				`"InstanceIDs":["i-1234567890abcdef0"],` +
				`"Parameters":{"commands":["echo hello"]}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "document_not_found",
			body:       `{"DocumentName":"NoSuchDocument","InstanceIDs":["i-abc"]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_json",
			body:       `not-json`,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			rec := doRequest(t, h, "SendCommand", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out ssm.SendCommandOutput
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
				assert.NotEmpty(t, out.Command.CommandID)
				assert.Equal(t, "AWS-RunShellScript", out.Command.DocumentName)
			}
		})
	}
}
func TestHandler_ListCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(*testing.T, *ssm.Handler)
		body       string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "empty",
			body:       `{}`,
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "with_commands",
			setup: func(t *testing.T, h *ssm.Handler) {
				t.Helper()
				doRequest(t, h, "SendCommand", `{"DocumentName":"AWS-RunShellScript","InstanceIDs":["i-abc"]}`)
				doRequest(t, h, "SendCommand", `{"DocumentName":"AWS-RunShellScript","InstanceIDs":["i-def"]}`)
			},
			body:       `{}`,
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doRequest(t, h, "ListCommands", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var out ssm.ListCommandsOutput
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
			assert.Len(t, out.Commands, tt.wantCount)
		})
	}
}
func TestHandler_GetCommandInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup             func(*testing.T, *ssm.Handler) string
		body              func(cmdID string) string
		name              string
		wantCommandStatus string
		wantStatus        int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *ssm.Handler) string {
				t.Helper()
				rec := doRequest(t, h, "SendCommand", `{"DocumentName":"AWS-RunShellScript","InstanceIDs":["i-abc"]}`)
				var out ssm.SendCommandOutput
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))

				return out.Command.CommandID
			},
			body: func(cmdID string) string {
				return `{"CommandId":"` + cmdID + `","InstanceId":"i-abc"}`
			},
			wantStatus:        http.StatusOK,
			wantCommandStatus: "Success",
		},
		{
			name: "wrong_instance_id",
			setup: func(t *testing.T, h *ssm.Handler) string {
				t.Helper()
				rec := doRequest(t, h, "SendCommand", `{"DocumentName":"AWS-RunShellScript","InstanceIDs":["i-abc"]}`)
				var out ssm.SendCommandOutput
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))

				return out.Command.CommandID
			},
			body: func(cmdID string) string {
				return `{"CommandId":"` + cmdID + `","InstanceId":"i-wrong"}`
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "not_found",
			setup: nil,
			body: func(_ string) string {
				return `{"CommandId":"no-such-id","InstanceId":"i-abc"}`
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			var cmdID string
			if tt.setup != nil {
				cmdID = tt.setup(t, h)
			}

			rec := doRequest(t, h, "GetCommandInvocation", tt.body(cmdID))
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantCommandStatus != "" {
				var out ssm.GetCommandInvocationOutput
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
				assert.Equal(t, tt.wantCommandStatus, out.Status)
				assert.Equal(t, tt.wantCommandStatus, out.StatusDetails)
			}
		})
	}
}
func TestHandler_ListCommandInvocations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(*testing.T, *ssm.Handler)
		body       string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "empty",
			body:       `{}`,
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "with_invocations",
			setup: func(t *testing.T, h *ssm.Handler) {
				t.Helper()
				doRequest(t, h, "SendCommand", `{"DocumentName":"AWS-RunShellScript","InstanceIDs":["i-abc","i-def"]}`)
			},
			body:       `{}`,
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doRequest(t, h, "ListCommandInvocations", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var out ssm.ListCommandInvocationsOutput
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
			assert.Len(t, out.CommandInvocations, tt.wantCount)
		})
	}
}

// TestSendCommand_MaxConcurrencyMaxErrorsValidation locks in that
// SendCommandInput.MaxConcurrency/MaxErrors are checked against the wire
// model's pattern (ssm/2014-11-06/service-2.json: MaxConcurrency
// "^([1-9][0-9]*|[1-9][0-9]%|[1-9]%|100%)$", MaxErrors
// "^([1-9][0-9]*|[0]|[1-9][0-9]%|[0-9]%|100%)$") rather than accepted verbatim.
func TestSendCommand_MaxConcurrencyMaxErrorsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxConcurrency string
		maxErrors      string
		wantErr        bool
	}{
		{name: "absolute counts", maxConcurrency: "10", maxErrors: "0"},
		{name: "percentages", maxConcurrency: "50%", maxErrors: "10%"},
		{name: "unset is allowed"},
		{name: "maxConcurrency zero", maxConcurrency: "0", wantErr: true},
		{name: "maxConcurrency leading zero", maxConcurrency: "05", wantErr: true},
		{name: "maxConcurrency over 100 percent", maxConcurrency: "150%", wantErr: true},
		{name: "maxConcurrency non-numeric", maxConcurrency: "abc", wantErr: true},
		{name: "maxErrors negative", maxErrors: "-1", wantErr: true},
		{name: "maxErrors non-numeric", maxErrors: "abc", wantErr: true},
		{name: "maxConcurrency over length bound", maxConcurrency: "12345678", wantErr: true},
		{name: "maxErrors over length bound", maxErrors: "12345678", wantErr: true},
		{name: "maxConcurrency at length bound", maxConcurrency: "1234567"},
		{name: "maxErrors at length bound", maxErrors: "1234567"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()

			_, err := b.SendCommand(context.Background(), &ssm.SendCommandInput{
				DocumentName:   "AWS-RunShellScript",
				InstanceIDs:    []string{"i-111"},
				MaxConcurrency: tc.maxConcurrency,
				MaxErrors:      tc.maxErrors,
			})

			if tc.wantErr {
				require.ErrorIs(t, err, ssm.ErrValidationException)

				return
			}

			require.NoError(t, err)
		})
	}
}
