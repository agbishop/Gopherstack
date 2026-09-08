// White-box: needs the endpoint/inference-component transition delay and
// status constants directly, mirroring pipeline_execution_start_test.go.
package sagemaker //nolint:testpackage // see comment above

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

const transitionTestVariant = "v1"

func seedEndpointConfig(t *testing.T, b *InMemoryBackend, name string) {
	t.Helper()

	_, err := b.CreateEndpointConfig(context.Background(), name, []ProductionVariant{
		{VariantName: transitionTestVariant, ModelName: "m", InitialInstanceCount: 1, InitialVariantWeight: 1},
	}, nil)
	if err != nil {
		t.Fatalf("CreateEndpointConfig: %v", err)
	}
}

// TestEndpointTransitions_ReachInService pins the normal path for all three
// callers of scheduleEndpointTransition: each must still land on InService
// after its delay. A fromStatus guard narrower than the status the caller
// actually set (e.g. requiring InService instead of Creating/Updating) would
// break this and only this test would catch it (gopherstack-rh77).
func TestEndpointTransitions_ReachInService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		act         func(t *testing.T, b *InMemoryBackend, name string)
		name        string
		wantInterim string
		delay       time.Duration
	}{
		{
			name:        "create",
			wantInterim: statusCreating,
			delay:       endpointCreatingToInService,
			act: func(t *testing.T, b *InMemoryBackend, name string) {
				t.Helper()

				seedEndpointConfig(t, b, "ec-"+name)

				if _, err := b.CreateEndpointFSM(context.Background(), CreateEndpointOptions{
					Name: name, EndpointConfigName: "ec-" + name,
				}); err != nil {
					t.Fatalf("CreateEndpointFSM: %v", err)
				}
			},
		},
		{
			name:        "update",
			wantInterim: statusUpdating,
			delay:       endpointUpdatingToInService,
			act: func(t *testing.T, b *InMemoryBackend, name string) {
				t.Helper()

				seedEndpointConfig(t, b, "ec-"+name)

				if _, err := b.CreateEndpointFSM(context.Background(), CreateEndpointOptions{
					Name: name, EndpointConfigName: "ec-" + name,
				}); err != nil {
					t.Fatalf("CreateEndpointFSM: %v", err)
				}

				time.Sleep(endpointCreatingToInService + time.Millisecond)
				synctest.Wait()

				if _, err := b.UpdateEndpointFSM(context.Background(), name, UpdateEndpointOptions{
					EndpointConfigName: "ec-" + name,
				}); err != nil {
					t.Fatalf("UpdateEndpointFSM: %v", err)
				}
			},
		},
		{
			name:        "weights and capacities",
			wantInterim: statusUpdating,
			delay:       endpointUpdatingToInService,
			act: func(t *testing.T, b *InMemoryBackend, name string) {
				t.Helper()

				seedEndpointConfig(t, b, "ec-"+name)

				if _, err := b.CreateEndpointFSM(context.Background(), CreateEndpointOptions{
					Name: name, EndpointConfigName: "ec-" + name,
				}); err != nil {
					t.Fatalf("CreateEndpointFSM: %v", err)
				}

				time.Sleep(endpointCreatingToInService + time.Millisecond)
				synctest.Wait()

				w := 0.5
				changes := []DesiredWeightAndCapacity{{VariantName: transitionTestVariant, DesiredWeight: &w}}
				if _, err := b.UpdateEndpointWeightsAndCapacitiesFull(context.Background(), name, changes); err != nil {
					t.Fatalf("UpdateEndpointWeightsAndCapacitiesFull: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := NewInMemoryBackend("000000000000", "us-east-1")
				defer b.Shutdown(context.Background())

				name := "ep-" + tc.name

				tc.act(t, b, name)

				ep, err := b.DescribeEndpoint(context.Background(), name)
				if err != nil {
					t.Fatalf("DescribeEndpoint: %v", err)
				}
				if ep.EndpointStatus != tc.wantInterim {
					t.Fatalf("status before delay = %q, want %q (must not report a terminal status early)",
						ep.EndpointStatus, tc.wantInterim)
				}

				time.Sleep(tc.delay + time.Millisecond)
				synctest.Wait()

				ep, err = b.DescribeEndpoint(context.Background(), name)
				if err != nil {
					t.Fatalf("DescribeEndpoint after delay: %v", err)
				}
				if ep.EndpointStatus != statusInService {
					t.Fatalf("status after delay = %q, want %q", ep.EndpointStatus, statusInService)
				}
			})
		})
	}
}

// TestEndpointTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate is an
// ordering test, not a clobber test: every caller of scheduleEndpointTransition
// targets the same terminal status (InService), so a stale callback can never
// leave an endpoint on a permanently wrong status here (gopherstack-rh77
// report). What a missing fromStatus guard does allow is a stale callback
// re-touching the record after a later, overlapping transition already
// finished it -- observable as a spurious LastModifiedTime bump. This
// constructs that interleaving: Create (300ms) then an immediate Update
// (250ms) on the same endpoint. Update's callback fires first and reaches
// InService; Create's callback, firing 50ms later, must see EndpointStatus
// no longer equal to Creating and no-op rather than re-stamping
// LastModifiedTime.
func TestEndpointTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := NewInMemoryBackend("000000000000", "us-east-1")
		defer b.Shutdown(context.Background())

		seedEndpointConfig(t, b, "ec-race")

		if _, err := b.CreateEndpointFSM(context.Background(), CreateEndpointOptions{
			Name: "ep-race", EndpointConfigName: "ec-race",
		}); err != nil {
			t.Fatalf("CreateEndpointFSM: %v", err)
		}

		if _, err := b.UpdateEndpointFSM(context.Background(), "ep-race", UpdateEndpointOptions{
			EndpointConfigName: "ec-race",
		}); err != nil {
			t.Fatalf("UpdateEndpointFSM: %v", err)
		}

		time.Sleep(endpointUpdatingToInService + time.Millisecond)
		synctest.Wait()

		ep, err := b.DescribeEndpoint(context.Background(), "ep-race")
		if err != nil {
			t.Fatalf("DescribeEndpoint: %v", err)
		}
		if ep.EndpointStatus != statusInService {
			t.Fatalf("status after update delay = %q, want %q", ep.EndpointStatus, statusInService)
		}

		settledAt := ep.LastModifiedTime

		time.Sleep(endpointCreatingToInService)
		synctest.Wait()

		ep, err = b.DescribeEndpoint(context.Background(), "ep-race")
		if err != nil {
			t.Fatalf("DescribeEndpoint after create's delay: %v", err)
		}
		if ep.EndpointStatus != statusInService {
			t.Fatalf("status after create's stale delay = %q, want %q", ep.EndpointStatus, statusInService)
		}
		if !ep.LastModifiedTime.Equal(settledAt) {
			t.Fatalf(
				"LastModifiedTime changed from %v to %v: Create's stale callback re-touched an endpoint "+
					"already advanced past Creating by Update",
				settledAt, ep.LastModifiedTime,
			)
		}
	})
}

// TestInferenceComponentTransitions_ReachInService is the inference
// component equivalent of TestEndpointTransitions_ReachInService.
func TestInferenceComponentTransitions_ReachInService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		act         func(t *testing.T, b *InMemoryBackend, name string)
		name        string
		wantInterim string
		delay       time.Duration
	}{
		{
			name:        "create",
			wantInterim: statusCreating,
			delay:       inferenceComponentCreatingToInService,
			act: func(t *testing.T, b *InMemoryBackend, name string) {
				t.Helper()

				if _, err := b.CreateInferenceComponent(context.Background(), CreateInferenceComponentOptions{
					InferenceComponentName: name, EndpointName: "ep", VariantName: transitionTestVariant, CopyCount: 1,
				}); err != nil {
					t.Fatalf("CreateInferenceComponent: %v", err)
				}
			},
		},
		{
			name:        "update",
			wantInterim: statusUpdating,
			delay:       inferenceComponentUpdatingToInService,
			act: func(t *testing.T, b *InMemoryBackend, name string) {
				t.Helper()

				if _, err := b.CreateInferenceComponent(context.Background(), CreateInferenceComponentOptions{
					InferenceComponentName: name, EndpointName: "ep", VariantName: transitionTestVariant, CopyCount: 1,
				}); err != nil {
					t.Fatalf("CreateInferenceComponent: %v", err)
				}

				time.Sleep(inferenceComponentCreatingToInService + time.Millisecond)
				synctest.Wait()

				opts := UpdateInferenceComponentOptions{}
				if err := b.UpdateInferenceComponent(context.Background(), name, opts); err != nil {
					t.Fatalf("UpdateInferenceComponent: %v", err)
				}
			},
		},
		{
			name:        "update runtime config",
			wantInterim: statusUpdating,
			delay:       inferenceComponentUpdatingToInService,
			act: func(t *testing.T, b *InMemoryBackend, name string) {
				t.Helper()

				if _, err := b.CreateInferenceComponent(context.Background(), CreateInferenceComponentOptions{
					InferenceComponentName: name, EndpointName: "ep", VariantName: transitionTestVariant, CopyCount: 1,
				}); err != nil {
					t.Fatalf("CreateInferenceComponent: %v", err)
				}

				time.Sleep(inferenceComponentCreatingToInService + time.Millisecond)
				synctest.Wait()

				if err := b.UpdateInferenceComponentRuntimeConfig(context.Background(), name, 3); err != nil {
					t.Fatalf("UpdateInferenceComponentRuntimeConfig: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := NewInMemoryBackend("000000000000", "us-east-1")
				defer b.Shutdown(context.Background())

				name := "ic-" + tc.name

				tc.act(t, b, name)

				c, err := b.DescribeInferenceComponent(context.Background(), name)
				if err != nil {
					t.Fatalf("DescribeInferenceComponent: %v", err)
				}
				if c.InferenceComponentStatus != tc.wantInterim {
					t.Fatalf("status before delay = %q, want %q (must not report a terminal status early)",
						c.InferenceComponentStatus, tc.wantInterim)
				}

				time.Sleep(tc.delay + time.Millisecond)
				synctest.Wait()

				c, err = b.DescribeInferenceComponent(context.Background(), name)
				if err != nil {
					t.Fatalf("DescribeInferenceComponent after delay: %v", err)
				}
				if c.InferenceComponentStatus != statusInService {
					t.Fatalf("status after delay = %q, want %q", c.InferenceComponentStatus, statusInService)
				}
			})
		})
	}
}

// TestInferenceComponentTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate
// is the inference component equivalent of
// TestEndpointTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate: an
// ordering test, not a clobber test, for the same reason (every caller
// targets InService as the sole terminal status).
func TestInferenceComponentTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := NewInMemoryBackend("000000000000", "us-east-1")
		defer b.Shutdown(context.Background())

		if _, err := b.CreateInferenceComponent(context.Background(), CreateInferenceComponentOptions{
			InferenceComponentName: "ic-race", EndpointName: "ep", VariantName: transitionTestVariant, CopyCount: 1,
		}); err != nil {
			t.Fatalf("CreateInferenceComponent: %v", err)
		}

		opts := UpdateInferenceComponentOptions{}
		if err := b.UpdateInferenceComponent(context.Background(), "ic-race", opts); err != nil {
			t.Fatalf("UpdateInferenceComponent: %v", err)
		}

		time.Sleep(inferenceComponentUpdatingToInService + time.Millisecond)
		synctest.Wait()

		c, err := b.DescribeInferenceComponent(context.Background(), "ic-race")
		if err != nil {
			t.Fatalf("DescribeInferenceComponent: %v", err)
		}
		if c.InferenceComponentStatus != statusInService {
			t.Fatalf("status after update delay = %q, want %q", c.InferenceComponentStatus, statusInService)
		}

		settledAt := c.LastModifiedTime

		time.Sleep(inferenceComponentCreatingToInService)
		synctest.Wait()

		c, err = b.DescribeInferenceComponent(context.Background(), "ic-race")
		if err != nil {
			t.Fatalf("DescribeInferenceComponent after create's delay: %v", err)
		}
		if c.InferenceComponentStatus != statusInService {
			t.Fatalf("status after create's stale delay = %q, want %q", c.InferenceComponentStatus, statusInService)
		}
		if !c.LastModifiedTime.Equal(settledAt) {
			t.Fatalf(
				"LastModifiedTime changed from %v to %v: Create's stale callback re-touched a component "+
					"already advanced past Creating by Update",
				settledAt, c.LastModifiedTime,
			)
		}
	})
}
