package lockmetrics_test

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seriesFor returns the metrics in family with the given "lock" label value.
func seriesFor(mfs []*dto.MetricFamily, family, lockName string) []*dto.Metric {
	var out []*dto.Metric

	for _, mf := range mfs {
		if mf.GetName() != family {
			continue
		}

		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "lock" && lp.GetValue() == lockName {
					out = append(out, m)
				}
			}
		}
	}

	return out
}

func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}

	return ""
}

// TestRWMutex_SharedName_NoDuplicateSeriesInGather is the regression test for
// gopherstack-koq4: two RWMutex instances sharing one resource name must
// aggregate into a single series per label tuple, not produce a MultiError
// from Gather.
//
// Before the fix this failed even with zero contention (see PR description);
// contention just adds a second colliding series (write-held).
func TestRWMutex_SharedName_NoDuplicateSeriesInGather(t *testing.T) {
	t.Parallel()

	name := "collision.shared." + t.Name()

	m1 := lockmetrics.New(name)
	m2 := lockmetrics.New(name)
	t.Cleanup(m1.Close)
	t.Cleanup(m2.Close)

	m1.Lock("holder")
	m2.Lock("holder")

	waiterReady := make(chan struct{}, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		waiterReady <- struct{}{}
		m1.Lock("waiter")
		m1.Unlock()
	}()
	go func() {
		defer wg.Done()
		waiterReady <- struct{}{}
		m2.Lock("waiter")
		m2.Unlock()
	}()
	<-waiterReady
	<-waiterReady

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (m1.WriteWaiters() == 0 || m2.WriteWaiters() == 0) {
		runtime.Gosched()
	}

	require.EqualValues(t, 1, m1.WriteWaiters(), "m1 should have a queued waiter")
	require.EqualValues(t, 1, m2.WriteWaiters(), "m2 should have a queued waiter")

	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err, "Gather must not return a MultiError from the shared name")

	waiters := seriesFor(mfs, "gopherstack_lock_write_waiters", name)
	require.Len(t, waiters, 1, "exactly one write_waiters series for the shared name")
	assert.InDelta(t, 2, waiters[0].GetGauge().GetValue(), 0,
		"waiters from both instances sharing the name must sum, not vanish or duplicate")

	held := seriesFor(mfs, "gopherstack_lock_write_held_seconds", name)
	require.Len(t, held, 1, "exactly one write_held series for (name, op) even though two instances hold it")
	assert.Equal(t, "holder", labelValue(held[0], "operation"))
	assert.Positive(t, held[0].GetGauge().GetValue())

	m1.Unlock()
	m2.Unlock()
	wg.Wait()
}

// TestRWMutex_DistinctNames_StayDistinct pins that aggregating by name does
// not over-share: two RWMutex instances with different names must keep
// independent series and values.
func TestRWMutex_DistinctNames_StayDistinct(t *testing.T) {
	t.Parallel()

	nameA := "collision.distinct.a." + t.Name()
	nameB := "collision.distinct.b." + t.Name()

	mA := lockmetrics.New(nameA)
	mB := lockmetrics.New(nameB)
	t.Cleanup(mA.Close)
	t.Cleanup(mB.Close)

	mA.Lock("opA")

	done := make(chan struct{})
	go func() {
		mA.Lock("waiterA")
		defer mA.Unlock()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && mA.WriteWaiters() == 0 {
		runtime.Gosched()
	}

	require.EqualValues(t, 1, mA.WriteWaiters())
	require.EqualValues(t, 0, mB.WriteWaiters())

	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	waitersA := seriesFor(mfs, "gopherstack_lock_write_waiters", nameA)
	require.Len(t, waitersA, 1)
	assert.InDelta(t, 1, waitersA[0].GetGauge().GetValue(), 0)

	waitersB := seriesFor(mfs, "gopherstack_lock_write_waiters", nameB)
	require.Len(t, waitersB, 1)
	assert.InDelta(t, 0, waitersB[0].GetGauge().GetValue(), 0)

	heldA := seriesFor(mfs, "gopherstack_lock_write_held_seconds", nameA)
	require.Len(t, heldA, 1)
	assert.Equal(t, "opA", labelValue(heldA[0], "operation"))

	heldB := seriesFor(mfs, "gopherstack_lock_write_held_seconds", nameB)
	assert.Empty(t, heldB, "nameB was never locked, so no held series should exist for it")

	mA.Unlock()
	<-done
}
