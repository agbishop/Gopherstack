package cloudwatch

// SubscribeAlarmStateChange registers cb to run whenever the alarm identified by
// alarmArn transitions to a new state. cb is invoked after setAlarmStateLocked's
// caller has released b.mu (see SetAlarmState), never while the lock is held, so
// it is safe for cb to call back into this backend or another one. It returns an
// unsubscribe func, safe to call at most once.
//
// This is the generic alarm-state-change hook other services attach to an alarm
// they do not own -- e.g. FIS stop conditions (gopherstack-x842, gopherstack-9939)
// -- distinct from AlarmActions/OKActions/InsufficientDataActions, which only
// fire ARNs the alarm's own owner configured.
func (b *InMemoryBackend) SubscribeAlarmStateChange(
	alarmArn string, cb func(newState string),
) func() {
	b.mu.Lock("SubscribeAlarmStateChange")
	defer b.mu.Unlock()

	if b.alarmStateSubscribers == nil {
		b.alarmStateSubscribers = make(map[string]map[uint64]func(string))
	}

	if b.alarmStateSubscribers[alarmArn] == nil {
		b.alarmStateSubscribers[alarmArn] = make(map[uint64]func(string))
	}

	id := b.alarmSubSeq
	b.alarmSubSeq++
	b.alarmStateSubscribers[alarmArn][id] = cb

	return func() {
		b.mu.Lock("UnsubscribeAlarmStateChange")
		defer b.mu.Unlock()
		delete(b.alarmStateSubscribers[alarmArn], id)
	}
}

// alarmStateChangeCallbacksLocked returns a snapshot of the callbacks subscribed
// to alarmArn. Caller must hold b.mu.
func (b *InMemoryBackend) alarmStateChangeCallbacksLocked(alarmArn string) []func(string) {
	subs := b.alarmStateSubscribers[alarmArn]
	if len(subs) == 0 {
		return nil
	}

	cbs := make([]func(string), 0, len(subs))
	for _, cb := range subs {
		cbs = append(cbs, cb)
	}

	return cbs
}
