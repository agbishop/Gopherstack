package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
	ecrbackend "github.com/blackbirdworks/gopherstack/services/ecr"
)

// pushImageToRepo pushes a minimal image so the repository is non-empty.
func pushImageToRepo(t *testing.T, backends *cloudformation.ServiceBackends, repoName string) {
	t.Helper()

	_, err := backends.ECR.Backend.PutImage(t.Context(), repoName, ecrbackend.Image{
		ImageID:       ecrbackend.ImageIdentifier{ImageTag: "latest"},
		ImageManifest: `{"schemaVersion":2}`,
	})
	require.NoError(t, err)
}

func TestDeleteECRRepository_EmptyOnDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		props   map[string]any
		name    string
		wantErr bool
	}{
		{
			name:    "empty_on_delete_true_forces_through_images",
			props:   map[string]any{"EmptyOnDelete": true},
			wantErr: false,
		},
		{
			name:    "empty_on_delete_absent_blocks_nonempty_repo",
			props:   map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty_on_delete_false_blocks_nonempty_repo",
			props:   map[string]any{"EmptyOnDelete": false},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backends := newAdditionalServiceBackends()
			rc := cloudformation.NewResourceCreator(backends)
			ctx := t.Context()

			createProps := map[string]any{"RepositoryName": "repo-" + tt.name}

			arn, err := rc.Create(ctx, "MyRepo", "AWS::ECR::Repository", createProps, nil, nil)
			require.NoError(t, err)
			require.NotEmpty(t, arn)

			pushImageToRepo(t, backends, "repo-"+tt.name)

			err = rc.Delete(ctx, "AWS::ECR::Repository", arn, tt.props)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ecrbackend.ErrRepositoryNotEmpty)

				return
			}

			require.NoError(t, err)
		})
	}
}
