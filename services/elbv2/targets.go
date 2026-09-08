package elbv2

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// InMemoryBackend is an in-memory implementation of StorageBackend.
// targetHealthKey builds a map key for a target in a target group.
func targetHealthKey(id string, port int32) string {
	return id + ":" + strconv.Itoa(int(port))
}

const (
	targetHealthDelay              = 200 * time.Millisecond
	defaultDeregistrationDelaySecs = 300
	targetDrainingReason           = "Target.DeregistrationInProgress"
)

// runHealthReconciler transitions registered targets from initial to healthy.
func (b *InMemoryBackend) runHealthReconciler() {
	ticker := time.NewTicker(targetHealthDelay / 5) //nolint:mnd // 5 ticks per delay period
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.reconcileTargetHealth()
		}
	}
}

type pendingTarget struct {
	tg        *TargetGroup
	tgArn     string
	targetKey string
}

type healthResult struct {
	key   string
	tgArn string
	state string
}

// reconcileTargetHealth promotes initial targets to healthy and removes expired draining targets.
func (b *InMemoryBackend) reconcileTargetHealth() {
	now := time.Now()

	var pending []pendingTarget
	var drained []drainedTarget
	func() {
		b.mu.RLock("reconcileTargetHealth-read")
		defer b.mu.RUnlock()
		pending = b.collectPendingTargets(now)
		drained = b.collectDrainedTargets(now)
	}()

	results := resolveTargetHealth(pending)

	if len(results) == 0 && len(drained) == 0 {
		return
	}

	b.mu.Lock("reconcileTargetHealth-write")
	defer b.mu.Unlock()
	b.applyHealthResults(results)
	b.removeDrainedTargets(drained)
}

func (b *InMemoryBackend) collectPendingTargets(now time.Time) []pendingTarget {
	var pending []pendingTarget

	for tgArn, readyMap := range b.targetReadyAt {
		for key, readyAt := range readyMap {
			if now.After(readyAt) {
				if tg, ok := b.targetGroups.Get(tgArn); ok {
					pending = append(pending, pendingTarget{tgArn: tgArn, targetKey: key, tg: tg})
				}
			}
		}
	}

	return pending
}

func resolveTargetHealth(pending []pendingTarget) []healthResult {
	results := make([]healthResult, 0, len(pending))

	for _, p := range pending {
		state := healthStateHealthy

		if p.tg.HealthCheckProtocol == protoHTTP || p.tg.HealthCheckProtocol == protoHTTPS {
			state = probeTargetHTTP(p.tg, p.targetKey)
		}

		results = append(results, healthResult{key: p.targetKey, tgArn: p.tgArn, state: state})
	}

	return results
}

func (b *InMemoryBackend) applyHealthResults(results []healthResult) {
	for _, r := range results {
		tg, ok := b.targetGroups.Get(r.tgArn)
		if !ok {
			continue
		}

		for i := range tg.Targets {
			if targetHealthKey(tg.Targets[i].ID, tg.Targets[i].Port) == r.key {
				if tg.Targets[i].HealthState == "initial" {
					tg.Targets[i].HealthState = r.state
					tg.Targets[i].HealthReason = ""
				}
			}
		}

		if rm := b.targetReadyAt[r.tgArn]; rm != nil {
			delete(rm, r.key)
		}
	}
}

type drainedTarget struct {
	tgArn     string
	targetKey string
}

// collectDrainedTargets returns targets whose drain expiry has passed.
// Caller must hold b.mu (read).
func (b *InMemoryBackend) collectDrainedTargets(now time.Time) []drainedTarget {
	var drained []drainedTarget

	for tgArn, expiryMap := range b.targetDrainingUntil {
		for key, expiry := range expiryMap {
			if now.After(expiry) {
				drained = append(drained, drainedTarget{tgArn: tgArn, targetKey: key})
			}
		}
	}

	return drained
}

// removeDrainedTargets removes drained targets from their target groups.
// Caller must hold b.mu (write).
func (b *InMemoryBackend) removeDrainedTargets(drained []drainedTarget) {
	for _, d := range drained {
		tg, ok := b.targetGroups.Get(d.tgArn)
		if !ok {
			continue
		}

		remaining := make([]Target, 0, len(tg.Targets))

		for _, t := range tg.Targets {
			if targetHealthKey(t.ID, t.Port) != d.targetKey {
				remaining = append(remaining, t)
			}
		}

		tg.Targets = remaining

		if rm := b.targetDrainingUntil[d.tgArn]; rm != nil {
			delete(rm, d.targetKey)
		}
	}
}

// probeTargetHTTP performs a real HTTP health check against the target.
// Returns healthStateHealthy on 2xx, "unhealthy" otherwise. Falls back to healthStateHealthy on unreachable targets.
func probeTargetHTTP(tg *TargetGroup, targetKey string) string {
	id, _, _ := strings.Cut(targetKey, ":")

	port := tg.HealthCheckPort
	if port == "" || port == "traffic-port" {
		port = strconv.Itoa(int(tg.Port))
	}

	path := tg.HealthCheckPath
	if path == "" {
		path = "/"
	}

	scheme := strings.ToLower(tg.HealthCheckProtocol)
	url := scheme + "://" + id + ":" + port + path

	client := &http.Client{Timeout: 2 * time.Second} //nolint:mnd // 2s probe timeout

	resp, err := client.Get(url) //nolint:noctx // probe is fire-and-forget
	if err != nil {
		return healthStateHealthy // unreachable → treat as healthy in mock
	}

	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return healthStateHealthy
	}

	return "unhealthy"
}

// RegisterTargets registers targets with a target group.
func (b *InMemoryBackend) RegisterTargets(tgArn string, targets []Target) error {
	b.mu.Lock("RegisterTargets")
	defer b.mu.Unlock()

	tg, ok := b.targetGroups.Get(tgArn)
	if !ok {
		return ErrTargetGroupNotFound
	}

	existing := make(map[string]bool)
	for _, t := range tg.Targets {
		existing[t.ID+":"+strconv.Itoa(int(t.Port))] = true
	}

	now := time.Now()

	for _, t := range targets {
		key := targetHealthKey(t.ID, t.Port)
		if !existing[key] {
			if t.HealthState == "" {
				t.HealthState = "initial"
				t.HealthReason = "Elb.InitialHealthChecking"

				if b.targetReadyAt[tgArn] == nil {
					b.targetReadyAt[tgArn] = make(map[string]time.Time)
				}

				b.targetReadyAt[tgArn][key] = now.Add(targetHealthDelay)
			}

			tg.Targets = append(tg.Targets, t)
			existing[key] = true
		}
	}

	return nil
}

// DeregisterTargets transitions targets to draining state. They are removed
// after the deregistration_delay.timeout_seconds attribute expires.
func (b *InMemoryBackend) DeregisterTargets(tgArn string, targets []Target) error {
	b.mu.Lock("DeregisterTargets")
	defer b.mu.Unlock()

	tg, ok := b.targetGroups.Get(tgArn)
	if !ok {
		return ErrTargetGroupNotFound
	}

	drainSecs := int64(defaultDeregistrationDelaySecs)
	if v, ok2 := tg.TargetGroupAttributes["deregistration_delay.timeout_seconds"]; ok2 {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			drainSecs = n
		}
	}

	drainDuration := time.Duration(drainSecs) * time.Second
	drainExpiry := time.Now().Add(drainDuration)

	remove := make(map[string]bool)
	for _, t := range targets {
		remove[targetHealthKey(t.ID, t.Port)] = true
	}

	for i := range tg.Targets {
		key := targetHealthKey(tg.Targets[i].ID, tg.Targets[i].Port)
		if remove[key] && tg.Targets[i].HealthState != "draining" {
			tg.Targets[i].HealthState = "draining"
			tg.Targets[i].HealthReason = targetDrainingReason

			if b.targetDrainingUntil[tgArn] == nil {
				b.targetDrainingUntil[tgArn] = make(map[string]time.Time)
			}

			b.targetDrainingUntil[tgArn][key] = drainExpiry
		}
	}

	return nil
}

// DescribeTargetHealth returns health descriptions for targets registered with the target group.
func (b *InMemoryBackend) DescribeTargetHealth(tgArn string) ([]TargetHealthDescription, error) {
	b.mu.RLock("DescribeTargetHealth")
	defer b.mu.RUnlock()

	tg, ok := b.targetGroups.Get(tgArn)
	if !ok {
		return nil, ErrTargetGroupNotFound
	}

	result := make([]TargetHealthDescription, len(tg.Targets))
	for i, t := range tg.Targets {
		state := t.HealthState
		if state == "" {
			state = healthStateHealthy
		}

		result[i] = TargetHealthDescription{
			Target:       t,
			HealthState:  state,
			HealthReason: t.HealthReason,
		}
	}

	return result, nil
}

// SetTargetHealthState overrides the health state for a specific target in a target group.
// Used in tests to simulate health state transitions.
func (b *InMemoryBackend) SetTargetHealthState(
	tgArn, targetID string,
	port int32,
	state, reason string,
) error {
	b.mu.Lock("SetTargetHealthState")
	defer b.mu.Unlock()

	tg, ok := b.targetGroups.Get(tgArn)
	if !ok {
		return ErrTargetGroupNotFound
	}

	for i, t := range tg.Targets {
		if t.ID == targetID && t.Port == port {
			tg.Targets[i].HealthState = state
			tg.Targets[i].HealthReason = reason

			return nil
		}
	}

	return ErrTargetGroupNotFound
}
