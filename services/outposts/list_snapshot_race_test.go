package outposts_test

import (
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	outpostssdk "github.com/aws/aws-sdk-go-v2/service/outposts"
	"github.com/stretchr/testify/require"
)

// TestListOutposts_ConcurrentUpdate_NoRace races UpdateOutpost (which
// mutates the stored Outpost in place under the backend lock) against
// ListOutposts (whose handler reads Description/Name/LifeCycleStatus/
// SupportedHardwareType off whatever pointer ListOutposts returned, with no
// lock held) on the same Outpost. ListAssets/ListCapacityTasks/ListOrders
// all clone before returning specifically to prevent this; ListOutposts did
// not. Run with -race.
func TestListOutposts_ConcurrentUpdate_NoRace(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	siteID := createTestSite(t, client)
	created := createTestOutpost(t, client, siteID)

	var wg sync.WaitGroup

	var updateErr, listErr error

	wg.Add(2)

	go func() {
		defer wg.Done()

		for range 50 {
			if _, err := client.UpdateOutpost(t.Context(), &outpostssdk.UpdateOutpostInput{
				OutpostId:   created.OutpostId,
				Description: aws.String("updated"),
			}); err != nil {
				updateErr = err
			}
		}
	}()

	go func() {
		defer wg.Done()

		for range 50 {
			if _, err := client.ListOutposts(t.Context(), &outpostssdk.ListOutpostsInput{}); err != nil {
				listErr = err
			}
		}
	}()

	wg.Wait()
	require.NoError(t, updateErr)
	require.NoError(t, listErr)
}

// TestListSites_ConcurrentUpdate_NoRace is TestListOutposts_ConcurrentUpdate_NoRace's
// Site analog: UpdateSite mutates Description/Name/Notes in place under lock
// while ListSites's handler reads them off an unlocked, uncloned pointer.
func TestListSites_ConcurrentUpdate_NoRace(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	siteID := createTestSite(t, client)

	var wg sync.WaitGroup

	var updateErr, listErr error

	wg.Add(2)

	go func() {
		defer wg.Done()

		for range 50 {
			if _, err := client.UpdateSite(t.Context(), &outpostssdk.UpdateSiteInput{
				SiteId:      aws.String(siteID),
				Description: aws.String("updated"),
			}); err != nil {
				updateErr = err
			}
		}
	}()

	go func() {
		defer wg.Done()

		for range 50 {
			if _, err := client.ListSites(t.Context(), &outpostssdk.ListSitesInput{}); err != nil {
				listErr = err
			}
		}
	}()

	wg.Wait()
	require.NoError(t, updateErr)
	require.NoError(t, listErr)
}
