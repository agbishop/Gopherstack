package databrew

import (
	"context"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) scheduleARN(region, name string) string {
	return arn.Build("databrew", region, b.accountID, "schedule/"+name)
}

func (b *InMemoryBackend) CreateSchedule(
	ctx context.Context,
	name string,
	jobNames []string,
	cron string,
	tags map[string]string,
) (*Schedule, error) {
	b.mu.Lock("CreateSchedule")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if name == "" {
		return nil, ErrValidation
	}
	t := b.schedulesTable(region)
	if t.Has(name) {
		return nil, ErrAlreadyExists
	}
	sc := &Schedule{
		Name: name, Arn: b.scheduleARN(region, name), JobNames: append([]string(nil), jobNames...),
		CronExpression: cron, Tags: maps.Clone(tags), AccountID: b.accountID,
		CreateDate: float64(time.Now().Unix()), LastModifiedDate: float64(time.Now().Unix()),
	}
	t.Put(sc)

	return sc, nil
}

func (b *InMemoryBackend) DescribeSchedule(ctx context.Context, name string) (*Schedule, error) {
	b.mu.RLock("DescribeSchedule")
	defer b.mu.RUnlock()
	region := getRegion(ctx, b.defaultRegion)
	sc, ok := b.schedulesTable(region).Get(name)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *sc
	cp.Tags = maps.Clone(sc.Tags)
	cp.JobNames = append([]string(nil), sc.JobNames...)

	return &cp, nil
}

func (b *InMemoryBackend) ListSchedules(
	ctx context.Context,
	maxResults int,
	nextToken string,
) ([]*Schedule, string) {
	b.mu.RLock("ListSchedules")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.schedulesTable(region)
	keys := snapshotKeys(t, scheduleKeyFn)
	pageKeys, next := paginateKeys(keys, maxResults, nextToken)
	out := make([]*Schedule, 0, len(pageKeys))
	for _, k := range pageKeys {
		v, _ := t.Get(k)
		cp := *v
		cp.Tags = maps.Clone(v.Tags)
		cp.JobNames = append([]string(nil), v.JobNames...)
		out = append(out, &cp)
	}

	return out, next
}

// UpdateSchedule overwrites CronExpression unconditionally
// (UpdateScheduleInput marks it "This member is required") but only
// overwrites JobNames when non-empty: JobNames has no such marker, so a
// caller updating just CronExpression must not have their existing
// JobNames clobbered.
func (b *InMemoryBackend) UpdateSchedule(
	ctx context.Context,
	name string,
	jobNames []string,
	cron string,
) error {
	b.mu.Lock("UpdateSchedule")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	sc, ok := b.schedulesTable(region).Get(name)
	if !ok {
		return ErrNotFound
	}
	if len(jobNames) > 0 {
		sc.JobNames = jobNames
	}
	sc.CronExpression = cron
	sc.LastModifiedDate = float64(time.Now().Unix())

	return nil
}

func (b *InMemoryBackend) DeleteSchedule(ctx context.Context, name string) error {
	b.mu.Lock("DeleteSchedule")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.schedulesTable(region).Delete(name) {
		return ErrNotFound
	}

	return nil
}
