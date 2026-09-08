package cloudwatchlogs_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
)

func TestCloudWatchLogsBackend_DescribeExportTasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(b *cloudwatchlogs.InMemoryBackend)
		taskID     string
		statusCode string
		wantLen    int
		wantErr    bool
	}{
		{
			name: "no_filter_returns_all",
			setup: func(b *cloudwatchlogs.InMemoryBackend) {
				cloudwatchlogs.AddExportTaskInternal(
					b,
					cloudwatchlogs.ExportTask{TaskID: "t1", Status: "COMPLETED", CreationTime: 1},
				)
				cloudwatchlogs.AddExportTaskInternal(
					b,
					cloudwatchlogs.ExportTask{TaskID: "t2", Status: "RUNNING", CreationTime: 2},
				)
			},
			wantLen: 2,
		},
		{
			name: "filter_by_task_id",
			setup: func(b *cloudwatchlogs.InMemoryBackend) {
				cloudwatchlogs.AddExportTaskInternal(
					b,
					cloudwatchlogs.ExportTask{TaskID: "t1", Status: "COMPLETED", CreationTime: 1},
				)
				cloudwatchlogs.AddExportTaskInternal(
					b,
					cloudwatchlogs.ExportTask{TaskID: "t2", Status: "RUNNING", CreationTime: 2},
				)
			},
			taskID:  "t1",
			wantLen: 1,
		},
		{
			name: "filter_by_status",
			setup: func(b *cloudwatchlogs.InMemoryBackend) {
				cloudwatchlogs.AddExportTaskInternal(
					b,
					cloudwatchlogs.ExportTask{TaskID: "t1", Status: "COMPLETED", CreationTime: 1},
				)
				cloudwatchlogs.AddExportTaskInternal(
					b,
					cloudwatchlogs.ExportTask{TaskID: "t2", Status: "RUNNING", CreationTime: 2},
				)
			},
			statusCode: "COMPLETED",
			wantLen:    1,
		},
		{
			name:    "empty_returns_empty",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(b)
			}

			tasks, _, err := b.DescribeExportTasks(tt.taskID, tt.statusCode, 50, "")

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, tasks, tt.wantLen)
		})
	}
}

func TestCloudWatchLogsBackend_DescribeImportTasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(b *cloudwatchlogs.InMemoryBackend)
		taskID  string
		wantLen int
		wantErr bool
	}{
		{
			name: "no_filter_returns_all",
			setup: func(b *cloudwatchlogs.InMemoryBackend) {
				cloudwatchlogs.AddImportTaskInternal(
					b,
					cloudwatchlogs.ImportTask{ImportID: "i1", CreationTime: 1},
				)
				cloudwatchlogs.AddImportTaskInternal(
					b,
					cloudwatchlogs.ImportTask{ImportID: "i2", CreationTime: 2},
				)
			},
			wantLen: 2,
		},
		{
			name: "filter_by_task_id",
			setup: func(b *cloudwatchlogs.InMemoryBackend) {
				cloudwatchlogs.AddImportTaskInternal(
					b,
					cloudwatchlogs.ImportTask{ImportID: "i1", CreationTime: 1},
				)
				cloudwatchlogs.AddImportTaskInternal(
					b,
					cloudwatchlogs.ImportTask{ImportID: "i2", CreationTime: 2},
				)
			},
			taskID:  "i1",
			wantLen: 1,
		},
		{
			name:    "empty_returns_empty",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(b)
			}

			tasks, _, err := b.DescribeImportTasks(tt.taskID, 50, "")

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, tasks, tt.wantLen)
		})
	}
}

func TestCloudWatchLogsBackend_BoundedMaps(t *testing.T) {
	t.Parallel()

	t.Run("export_tasks_limit", func(t *testing.T) {
		t.Parallel()

		b := cloudwatchlogs.NewInMemoryBackend()
		// Seed tasks directly using internal helper to avoid the limit.
		for i := range 1000 {
			cloudwatchlogs.AddExportTaskInternal(b, cloudwatchlogs.ExportTask{
				TaskID:       fmt.Sprintf("task-%d", i),
				LogGroupName: "g",
				Destination:  "bucket",
				Status:       "COMPLETED",
				CreationTime: int64(i + 1),
				From:         1,
				To:           2,
			})
		}

		_, err := b.CreateLogGroup(context.Background(), "g", "", "")
		require.NoError(t, err)
		_, err = b.CreateExportTask(context.Background(), "", "g", "", "bucket2", "", 1, 2)
		require.ErrorIs(t, err, cloudwatchlogs.ErrValidation)
	})
}
