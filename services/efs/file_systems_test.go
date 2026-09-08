package efs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestDescribeFileSystems_NotFound verifies the backend returns ErrNotFound
// when a specific FileSystemId is queried but does not exist.
func TestDescribeFileSystems_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{name: "nonexistent_id", id: "fs-notfound"},
		{name: "garbage_id", id: "not-a-real-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			_, _, err := b.DescribeFileSystems(context.Background(), tt.id, "", "", 0)
			require.ErrorIs(t, err, efs.ErrNotFound)
		})
	}
}

// TestDescribeFileSystems_CreationTokenFilter_Backend verifies the backend
// CreationToken filter directly.
func TestDescribeFileSystems_CreationTokenFilter_Backend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		token       string
		setupTokens []string
		wantCount   int
	}{
		{
			name:        "matches_existing_token",
			setupTokens: []string{"tok-x", "tok-y"},
			token:       "tok-x",
			wantCount:   1,
		},
		{
			name:        "no_match_returns_empty",
			setupTokens: []string{"tok-a"},
			token:       "tok-z",
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			for _, tok := range tt.setupTokens {
				_, err := b.CreateFileSystem(
					context.Background(),
					efs.CreateFileSystemRequest{CreationToken: tok},
				)
				require.NoError(t, err)
			}

			list, _, err := b.DescribeFileSystems(context.Background(), "", tt.token, "", 0)
			require.NoError(t, err)
			assert.Len(t, list, tt.wantCount)

			if tt.wantCount == 1 {
				assert.Equal(t, tt.token, list[0].CreationToken)
			}
		})
	}
}

// TestDescribeFileSystems_Pagination verifies Marker/NextMarker pagination.
func TestDescribeFileSystems_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		total     int
		maxItems  int
		wantFirst int
		wantNext  bool
	}{
		{
			name:      "all_items_fit_no_next",
			total:     3,
			maxItems:  5,
			wantFirst: 3,
			wantNext:  false,
		},
		{
			name:      "page_smaller_than_total",
			total:     5,
			maxItems:  2,
			wantFirst: 2,
			wantNext:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			for i := range tt.total {
				_, err := b.CreateFileSystem(
					context.Background(),
					fsReq("tok-page-"+tt.name+"-"+string(rune('a'+i))),
				)
				require.NoError(t, err)
			}

			list, nextMarker, err := b.DescribeFileSystems(
				context.Background(),
				"",
				"",
				"",
				tt.maxItems,
			)
			require.NoError(t, err)
			assert.Len(t, list, tt.wantFirst)

			if tt.wantNext {
				assert.NotEmpty(t, nextMarker)

				// Walk every remaining page and confirm the union of all pages
				// (this one plus every later one) equals the full created set,
				// with no item lost or duplicated at any page boundary.
				seen := make(map[string]bool, tt.total)
				for _, fs := range list {
					seen[fs.FileSystemID] = true
				}

				marker := nextMarker
				for range tt.total { // hard cap: at most tt.total more fetches before failing.
					pageList, next, pageErr := b.DescribeFileSystems(context.Background(), "", "", marker, tt.maxItems)
					require.NoError(t, pageErr)
					require.NotEmpty(t, pageList, "page must not be empty while a marker is being followed")

					for _, fs := range pageList {
						assert.False(t, seen[fs.FileSystemID],
							"file system %s duplicated across pages", fs.FileSystemID)
						seen[fs.FileSystemID] = true
					}

					if next == "" {
						break
					}
					marker = next
				}
				assert.Len(t, seen, tt.total, "union of every page must equal every created file system")
			} else {
				assert.Empty(t, nextMarker)
			}
		})
	}
}

// TestErrValidation verifies ErrValidation is a distinct sentinel.
func TestErrValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		perform func(b *efs.InMemoryBackend) error
		name    string
	}{
		{
			name: "bad_performance_mode",
			perform: func(b *efs.InMemoryBackend) error {
				_, err := b.CreateFileSystem(context.Background(), efs.CreateFileSystemRequest{
					CreationToken:   "tok",
					PerformanceMode: "badMode",
				})

				return err
			},
		},
		{
			name: "bad_throughput_mode",
			perform: func(b *efs.InMemoryBackend) error {
				_, err := b.CreateFileSystem(context.Background(), efs.CreateFileSystemRequest{
					CreationToken:  "tok",
					ThroughputMode: "badMode",
				})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			err := tt.perform(b)

			require.ErrorIs(t, err, efs.ErrValidation)
		})
	}
}

// TestPerformanceModeValidation verifies valid and invalid performance modes.
func TestPerformanceModeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "generalPurpose_valid", mode: "generalPurpose"},
		{name: "maxIO_valid", mode: "maxIO"},
		{name: "empty_defaults_to_valid", mode: ""},
		{name: "invalid_mode", mode: "invalid", wantErr: true},
		{name: "superPerf_invalid", mode: "superPerf", wantErr: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			token := "token-perf-" + tt.name + "-" + string(rune('a'+i))
			_, err := b.CreateFileSystem(context.Background(), efs.CreateFileSystemRequest{
				CreationToken:   token,
				PerformanceMode: tt.mode,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, efs.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestThroughputModeValidation verifies valid and invalid throughput modes.
func TestThroughputModeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		mode                     string
		provisionedThroughputMib float64
		wantErr                  bool
	}{
		{name: "bursting_valid", mode: "bursting"},
		{name: "provisioned_valid", mode: "provisioned", provisionedThroughputMib: 100},
		{name: "elastic_valid", mode: "elastic"},
		{name: "empty_defaults_to_valid", mode: ""},
		{name: "invalid_mode", mode: "invalid", wantErr: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			token := "token-thru-" + tt.name + "-" + string(rune('a'+i))
			_, err := b.CreateFileSystem(context.Background(), efs.CreateFileSystemRequest{
				CreationToken:            token,
				ThroughputMode:           tt.mode,
				ProvisionedThroughputMib: tt.provisionedThroughputMib,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, efs.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDeleteFileSystem_RequiresEmptyState verifies AWS-accurate behavior:
// DeleteFileSystem returns ErrFileSystemInUse when mount targets or access points exist.
// Callers must delete dependents before deleting the file system.
func TestDeleteFileSystem_RequiresEmptyState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		numMTs        int
		numAPs        int
		wantErrOnFull bool
	}{
		{name: "no_dependents_succeeds", numMTs: 0, numAPs: 0, wantErrOnFull: false},
		{name: "with_mount_target_rejected", numMTs: 1, numAPs: 0, wantErrOnFull: true},
		{name: "with_access_point_rejected", numMTs: 0, numAPs: 1, wantErrOnFull: true},
		{name: "with_both_rejected", numMTs: 2, numAPs: 1, wantErrOnFull: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-del-"+tt.name))
			require.NoError(t, err)

			var mtIDs []string
			for i := range tt.numMTs {
				mt, mtErr := b.CreateMountTarget(
					context.Background(),
					mtReq(fs.FileSystemID, "sn-"+string(rune('a'+i))),
				)
				require.NoError(t, mtErr)
				mtIDs = append(mtIDs, mt.MountTargetID)
			}

			var apIDs []string
			for range tt.numAPs {
				ap, apErr := b.CreateAccessPoint(context.Background(), apReq(fs.FileSystemID))
				require.NoError(t, apErr)
				apIDs = append(apIDs, ap.AccessPointID)
			}

			// First delete attempt.
			err = b.DeleteFileSystem(context.Background(), fs.FileSystemID)
			if tt.wantErrOnFull {
				require.ErrorIs(t, err, efs.ErrFileSystemInUse)

				// Clean up dependents and retry.
				for _, mtID := range mtIDs {
					require.NoError(t, b.DeleteMountTarget(context.Background(), mtID))
				}
				for _, apID := range apIDs {
					require.NoError(t, b.DeleteAccessPoint(context.Background(), apID))
				}

				err = b.DeleteFileSystem(context.Background(), fs.FileSystemID)
				require.NoError(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, 0, efs.FileSystemCount(b))
		})
	}
}

func TestDeleteFileSystem_RejectedWhileReplicating(t *testing.T) {
	t.Parallel()

	b := newTestEFSBackend()
	fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-del-repl"))
	require.NoError(t, err)

	_, err = b.CreateReplicationConfiguration(
		context.Background(),
		fs.FileSystemID,
		[]efs.ReplicationDestination{{Region: "us-west-2", Status: "ENABLED"}},
	)
	require.NoError(t, err)

	err = b.DeleteFileSystem(context.Background(), fs.FileSystemID)
	require.ErrorIs(t, err, efs.ErrFileSystemInUse)

	require.NoError(t, b.DeleteReplicationConfiguration(context.Background(), fs.FileSystemID))

	require.NoError(t, b.DeleteFileSystem(context.Background(), fs.FileSystemID))
}

// TestCreationTokenIdempotency verifies identical args return 200, different args return 409.
func TestCreationTokenIdempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs  error
		name       string
		first      efs.CreateFileSystemRequest
		second     efs.CreateFileSystemRequest
		wantSameFS bool
	}{
		{
			name: "identical_token_and_mode_returns_existing",
			first: efs.CreateFileSystemRequest{
				CreationToken:  "tok",
				ThroughputMode: "bursting",
			},
			second: efs.CreateFileSystemRequest{
				CreationToken:  "tok",
				ThroughputMode: "bursting",
			},
			wantErrIs:  efs.ErrCreationTokenExists,
			wantSameFS: true,
		},
		{
			name: "same_token_different_perf_mode_returns_conflict",
			first: efs.CreateFileSystemRequest{
				CreationToken:   "tok2",
				PerformanceMode: "generalPurpose",
			},
			second:    efs.CreateFileSystemRequest{CreationToken: "tok2", PerformanceMode: "maxIO"},
			wantErrIs: efs.ErrAlreadyExists,
		},
		{
			name: "same_token_different_encrypted_returns_conflict",
			first: efs.CreateFileSystemRequest{
				CreationToken: "tok3",
				Encrypted:     true,
			},
			second: efs.CreateFileSystemRequest{
				CreationToken: "tok3",
				Encrypted:     false,
			},
			wantErrIs: efs.ErrAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs1, err := b.CreateFileSystem(context.Background(), tt.first)
			require.NoError(t, err)

			fs2, err2 := b.CreateFileSystem(context.Background(), tt.second)
			require.ErrorIs(t, err2, tt.wantErrIs)

			if tt.wantSameFS {
				require.NotNil(t, fs2)
				assert.Equal(t, fs1.FileSystemID, fs2.FileSystemID)
			}
		})
	}
}

// TestProvisionedThroughput verifies provisioned throughput validation and persistence.
func TestProvisionedThroughput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		req       efs.CreateFileSystemRequest
		wantMibps float64
		wantErr   bool
	}{
		{
			name: "provisioned_with_valid_throughput",
			req: efs.CreateFileSystemRequest{
				CreationToken:            "prov-valid",
				ThroughputMode:           "provisioned",
				ProvisionedThroughputMib: 256,
			},
			wantMibps: 256,
		},
		{
			name: "provisioned_without_throughput_is_invalid",
			req: efs.CreateFileSystemRequest{
				CreationToken:  "prov-no-tp",
				ThroughputMode: "provisioned",
			},
			wantErr:   true,
			wantErrIs: efs.ErrValidation,
		},
		{
			name: "provisioned_throughput_below_minimum",
			req: efs.CreateFileSystemRequest{
				CreationToken:            "prov-low",
				ThroughputMode:           "provisioned",
				ProvisionedThroughputMib: 0.5,
			},
			wantErr:   true,
			wantErrIs: efs.ErrValidation,
		},
		{
			name: "provisioned_throughput_above_maximum",
			req: efs.CreateFileSystemRequest{
				CreationToken:            "prov-high",
				ThroughputMode:           "provisioned",
				ProvisionedThroughputMib: 2048,
			},
			wantErr:   true,
			wantErrIs: efs.ErrValidation,
		},
		{
			name: "non_provisioned_with_throughput_is_invalid",
			req: efs.CreateFileSystemRequest{
				CreationToken:            "prov-wrong-mode",
				ThroughputMode:           "bursting",
				ProvisionedThroughputMib: 100,
			},
			wantErr:   true,
			wantErrIs: efs.ErrValidation,
		},
		{
			name: "bursting_without_throughput_is_valid",
			req: efs.CreateFileSystemRequest{
				CreationToken:  "prov-bursting",
				ThroughputMode: "bursting",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), tt.req)

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.wantMibps, fs.ProvisionedThroughputMib, 0.001)
			}
		})
	}
}

// TestKmsKeyId verifies KmsKeyID auto-fill and validation.
func TestKmsKeyId(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs  error
		name       string
		req        efs.CreateFileSystemRequest
		wantErr    bool
		wantKmsSet bool
	}{
		{
			name: "encrypted_without_kms_autofills_managed_key",
			req: efs.CreateFileSystemRequest{
				CreationToken: "kms-auto",
				Encrypted:     true,
			},
			wantKmsSet: true,
		},
		{
			name: "encrypted_with_custom_kms_persisted",
			req: efs.CreateFileSystemRequest{
				CreationToken: "kms-custom",
				Encrypted:     true,
				KmsKeyID:      "arn:aws:kms:us-east-1:123456789012:key/custom-key",
			},
			wantKmsSet: true,
		},
		{
			name: "unencrypted_with_kms_is_invalid",
			req: efs.CreateFileSystemRequest{
				CreationToken: "kms-invalid",
				Encrypted:     false,
				KmsKeyID:      "arn:aws:kms:us-east-1:123456789012:key/some-key",
			},
			wantErr:   true,
			wantErrIs: efs.ErrValidation,
		},
		{
			name: "unencrypted_without_kms_is_valid",
			req: efs.CreateFileSystemRequest{
				CreationToken: "kms-none",
				Encrypted:     false,
			},
			wantKmsSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), tt.req)

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
				if tt.wantKmsSet {
					assert.NotEmpty(t, fs.KmsKeyID)
				} else {
					assert.Empty(t, fs.KmsKeyID)
				}
			}
		})
	}
}

// TestThroughputCooldown verifies UpdateFileSystem enforces the 24h cooldown.
func TestThroughputCooldown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs     error
		name          string
		firstMode     string
		secondMode    string
		allowCooldown bool
		wantSecondErr bool
	}{
		{
			name:          "first_change_always_succeeds",
			firstMode:     "elastic",
			secondMode:    "",
			allowCooldown: false,
		},
		{
			name:          "second_change_within_cooldown_fails",
			firstMode:     "elastic",
			secondMode:    "bursting",
			allowCooldown: false,
			wantSecondErr: true,
			wantErrIs:     efs.ErrTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-cooldown-"+tt.name))
			require.NoError(t, err)

			// First throughput change.
			_, err = b.UpdateFileSystem(
				context.Background(),
				fs.FileSystemID,
				efs.UpdateFileSystemRequest{
					ThroughputMode: tt.firstMode,
				},
			)
			require.NoError(t, err)

			if tt.secondMode != "" {
				_, err = b.UpdateFileSystem(
					context.Background(),
					fs.FileSystemID,
					efs.UpdateFileSystemRequest{
						ThroughputMode: tt.secondMode,
					},
				)

				if tt.wantSecondErr {
					require.ErrorIs(t, err, tt.wantErrIs)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

// TestUpdateFileSystem_ProvisionedThroughput verifies throughput updates are validated.
//
// wantErrIs was efs.ErrValidation until this pass; UpdateFileSystem declares
// BadRequest, never ValidationException (efs@v1.44.4 deserializers.go) -- the
// old assertion locked in the exact wire-code defect this pass fixed.
func TestUpdateFileSystem_ProvisionedThroughput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		updateReq efs.UpdateFileSystemRequest
		wantErr   bool
	}{
		{
			name: "update_provisioned_throughput_ok",
			updateReq: efs.UpdateFileSystemRequest{
				ProvisionedThroughputMib: 512,
			},
		},
		{
			name: "update_provisioned_throughput_out_of_range",
			updateReq: efs.UpdateFileSystemRequest{
				ProvisionedThroughputMib: 2048,
			},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name: "provisioned_throughput_on_bursting_invalid",
			updateReq: efs.UpdateFileSystemRequest{
				ThroughputMode:           "bursting",
				ProvisionedThroughputMib: 100,
			},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			// Create a provisioned FS so we can update its throughput.
			fs, err := b.CreateFileSystem(context.Background(), efs.CreateFileSystemRequest{
				CreationToken:            "tok-upd-tp-" + tt.name,
				ThroughputMode:           "provisioned",
				ProvisionedThroughputMib: 100,
			})
			require.NoError(t, err)

			// Manually reset LastThroughputChange to bypass cooldown.
			fs.LastThroughputChange = time.Time{}
			b.AddFileSystemInternal(fs)

			_, err = b.UpdateFileSystem(context.Background(), fs.FileSystemID, tt.updateReq)

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUpdateFileSystem_RequiresAvailableFileSystem verifies UpdateFileSystem
// rejects requests while the file system's lifecycle state is not "available"
// (efs@v1.44.4 types/errors.go: IncorrectFileSystemLifeCycleState, "Returned if
// the file system's lifecycle state is not \"available\"").
func TestUpdateFileSystem_RequiresAvailableFileSystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr       error
		name          string
		activateDelay time.Duration
	}{
		{name: "creating_state_rejected", activateDelay: time.Hour, wantErr: efs.ErrIncorrectFileSystemLifeCycleState},
		{name: "available_state_allowed", activateDelay: 0, wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			efs.SetFSActivationDelay(b, tt.activateDelay)

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ufs-lifecycle-"+tt.name))
			require.NoError(t, err)

			_, err = b.UpdateFileSystem(context.Background(), fs.FileSystemID, efs.UpdateFileSystemRequest{
				ThroughputMode: "elastic",
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
