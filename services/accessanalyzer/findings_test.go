package accessanalyzer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/accessanalyzer"
)

func TestAddFinding_ThenGet(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateAnalyzer("find-analyzer", accessanalyzer.AnalyzerTypeAccount, nil)

	isPublic := true
	f, err := b.AddFinding("find-analyzer", "AWS::S3::Bucket", "arn:aws:s3:::public-bucket",
		[]string{"s3:GetObject"}, nil, &isPublic)
	require.NoError(t, err)
	require.NotEmpty(t, f.ID)

	got, err := b.GetFinding("find-analyzer", f.ID)
	require.NoError(t, err)
	assert.Equal(t, accessanalyzer.FindingStatusActive, got.Status)
	assert.True(t, *got.IsPublic)
}

func TestListFindings_FilterByStatus(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateAnalyzer("list-find-analyzer", accessanalyzer.AnalyzerTypeAccount, nil)

	_, _ = b.AddFinding("list-find-analyzer", "AWS::S3::Bucket", "arn:aws:s3:::a", nil, nil, nil)
	f2, _ := b.AddFinding("list-find-analyzer", "AWS::IAM::Role", "arn:aws:iam:::role/r", nil, nil, nil)

	// Archive one finding.
	require.NoError(t,
		b.UpdateFindings("list-find-analyzer", []string{f2.ID}, "", accessanalyzer.FindingStatusArchived))

	active, _, err := b.ListFindings("list-find-analyzer", nil, "ACTIVE", nil, 0, "")
	require.NoError(t, err)
	assert.Len(t, active, 1)

	archived, _, err := b.ListFindings("list-find-analyzer", nil, "ARCHIVED", nil, 0, "")
	require.NoError(t, err)
	assert.Len(t, archived, 1)
}

func TestUpdateFindings_ChangesStatus(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateAnalyzer("upd-find-analyzer", accessanalyzer.AnalyzerTypeAccount, nil)

	f, _ := b.AddFinding("upd-find-analyzer", "AWS::S3::Bucket", "arn:aws:s3:::b", nil, nil, nil)

	require.NoError(t, b.UpdateFindings("upd-find-analyzer", []string{f.ID}, "", accessanalyzer.FindingStatusArchived))

	got, err := b.GetFinding("upd-find-analyzer", f.ID)
	require.NoError(t, err)
	assert.Equal(t, accessanalyzer.FindingStatusArchived, got.Status)
}

func TestUpdateFindings_SelectionMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceArn  string
		findingIDs   []string
		wantArchived bool
	}{
		{name: "by_ids_only", findingIDs: []string{"__target__"}, wantArchived: true},
		{name: "by_resource_arn_only", resourceArn: "arn:aws:s3:::sel-bucket", wantArchived: true},
		{
			name: "ids_and_resource_arn_match", findingIDs: []string{"__target__"},
			resourceArn: "arn:aws:s3:::sel-bucket", wantArchived: true,
		},
		{
			name: "ids_and_resource_arn_mismatch", findingIDs: []string{"__target__"},
			resourceArn: "arn:aws:s3:::other-bucket", wantArchived: false,
		},
		{name: "neither_selector_is_noop", wantArchived: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			_, err := b.CreateAnalyzer("sel-analyzer-"+tt.name, accessanalyzer.AnalyzerTypeAccount, nil)
			require.NoError(t, err)

			f, err := b.AddFinding("sel-analyzer-"+tt.name, "AWS::S3::Bucket", "arn:aws:s3:::sel-bucket", nil, nil, nil)
			require.NoError(t, err)

			ids := make([]string, len(tt.findingIDs))
			for i, id := range tt.findingIDs {
				if id == "__target__" {
					id = f.ID
				}

				ids[i] = id
			}

			require.NoError(t,
				b.UpdateFindings("sel-analyzer-"+tt.name, ids, tt.resourceArn, accessanalyzer.FindingStatusArchived))

			got, err := b.GetFinding("sel-analyzer-"+tt.name, f.ID)
			require.NoError(t, err)

			wantStatus := accessanalyzer.FindingStatusActive
			if tt.wantArchived {
				wantStatus = accessanalyzer.FindingStatusArchived
			}

			assert.Equal(t, wantStatus, got.Status)
		})
	}
}
