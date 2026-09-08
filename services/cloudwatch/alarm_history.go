package cloudwatch

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// matchesHistoryFilters returns true if the item passes all the given history filters.
// includeMetric/includeComposite/includeLog already have DescribeAlarmHistory's
// AlarmTypes default applied by the caller.
func matchesHistoryFilters(
	item AlarmHistoryItem,
	includeMetric, includeComposite, includeLog bool,
	historyItemType string,
	startDate, endDate time.Time,
) bool {
	switch item.AlarmType {
	case "CompositeAlarm":
		if !includeComposite {
			return false
		}
	case alarmTypeLogAlarm:
		if !includeLog {
			return false
		}
	default:
		// "MetricAlarm" and any legacy untagged entry.
		if !includeMetric {
			return false
		}
	}
	if historyItemType != "" && item.HistoryItemType != historyItemType {
		return false
	}
	if !startDate.IsZero() && item.Timestamp.Before(startDate) {
		return false
	}
	if !endDate.IsZero() && item.Timestamp.After(endDate) {
		return false
	}

	return true
}

// DescribeAlarmHistory returns history items for one or all alarms, filtered by type and date range.
// alarmTypes can contain "MetricAlarm", "CompositeAlarm", and/or "LogAlarm". Per the real
// DescribeAlarmHistoryInput.AlarmTypes doc comment ("If you omit this parameter, only metric
// alarms are returned"), omitting alarmTypes returns ONLY metric alarm history -- composite
// and log alarm history are included only when explicitly requested, mirroring DescribeAlarms'
// AlarmTypes default (bd gopherstack-yvb7).
//
// scanBy honors DescribeAlarmHistoryInput.ScanBy ("Specify TimestampDescending to
// have the newest event history returned first, and specify TimestampAscending
// to have the oldest history returned first" -- api_op_DescribeAlarmHistory.go).
// An empty/unrecognized scanBy keeps the existing oldest-first ordering.
func (b *InMemoryBackend) DescribeAlarmHistory(
	alarmName string, alarmTypes []string, historyItemType, nextToken, scanBy string,
	startDate, endDate time.Time,
	maxRecords int,
) (page.Page[AlarmHistoryItem], error) {
	b.mu.RLock("DescribeAlarmHistory")
	defer b.mu.RUnlock()

	typeSet := toSet(alarmTypes)
	includeMetric := len(typeSet) == 0 || typeSet["MetricAlarm"]
	includeComposite := typeSet["CompositeAlarm"]
	includeLog := typeSet[alarmTypeLogAlarm]

	var result []AlarmHistoryItem
	for name, items := range b.alarmHistory {
		if alarmName != "" && name != alarmName {
			continue
		}
		for _, item := range items {
			if matchesHistoryFilters(
				item, includeMetric, includeComposite, includeLog, historyItemType, startDate, endDate,
			) {
				result = append(result, item)
			}
		}
	}

	descending := scanBy == "TimestampDescending"
	sort.Slice(result, func(i, j int) bool {
		if !result[i].Timestamp.Equal(result[j].Timestamp) {
			if descending {
				return result[i].Timestamp.After(result[j].Timestamp)
			}

			return result[i].Timestamp.Before(result[j].Timestamp)
		}

		if descending {
			return result[i].seq > result[j].seq
		}

		return result[i].seq < result[j].seq
	})

	return page.New(result, nextToken, maxRecords, cwDefaultAlarmHistoryLimit), nil
}

// appendHistory adds a history item. Caller must hold b.mu (write lock).
// alarmTypeName should be "MetricAlarm" or "CompositeAlarm" to populate the AlarmType field.
func (b *InMemoryBackend) appendHistory(alarmName, alarmTypeName, itemType, summary, data string) {
	b.alarmHistorySeq++
	item := AlarmHistoryItem{
		Timestamp:       time.Now(),
		AlarmName:       alarmName,
		AlarmType:       alarmTypeName,
		HistoryItemType: itemType,
		HistorySummary:  summary,
		HistoryData:     data,
		seq:             b.alarmHistorySeq,
	}
	b.alarmHistory[alarmName] = append(b.alarmHistory[alarmName], item)
	// Cap history to avoid unbounded growth.
	if h := b.alarmHistory[alarmName]; len(h) > cwMaxAlarmHistory {
		b.alarmHistory[alarmName] = h[len(h)-cwMaxAlarmHistory:]
	}
}

// reindexAlarmHistorySeqLocked assigns fresh, deterministic seq values to
// every restored AlarmHistoryItem. seq is unexported and therefore not part
// of a persisted snapshot, so every item comes back from Restore with seq
// zero; without this, restored items sharing a Timestamp would tie again.
// Alarm names are visited in sorted order and each alarm's own item slice
// keeps its stored (insertion) order, so the result is reproducible from the
// same snapshot bytes regardless of Go's map iteration order.
// Caller must hold b.mu (write lock).
func (b *InMemoryBackend) reindexAlarmHistorySeqLocked() {
	names := make([]string, 0, len(b.alarmHistory))
	for name := range b.alarmHistory {
		names = append(names, name)
	}

	sort.Strings(names)

	var seq uint64

	for _, name := range names {
		items := b.alarmHistory[name]
		for i := range items {
			seq++
			items[i].seq = seq
		}
	}

	b.alarmHistorySeq = seq
}

// stateChangeHistoryData builds a JSON string for a state-change history item.
func (b *InMemoryBackend) stateChangeHistoryData(
	alarmName, oldState, newState, reason string,
) string {
	data := map[string]string{
		keyAlarmName:     alarmName,
		"OldStateValue":  oldState,
		"NewStateValue":  newState,
		"NewStateReason": reason,
	}
	// map[string]string marshaling cannot fail; error is intentionally ignored.
	bs, _ := json.Marshal(data)

	return string(bs)
}
