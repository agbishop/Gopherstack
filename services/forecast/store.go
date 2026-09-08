package forecast

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// resourceNameRegex matches valid Amazon Forecast resource names:
// only alphanumeric characters, underscores, and hyphens are allowed.
var resourceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// InMemoryBackend stores Amazon Forecast state with concurrency-safe transitions.
type InMemoryBackend struct {
	resources   map[resourceKind]*store.Table[Resource]
	evaluations map[string][]MonitorEvaluation
	tags        map[string]map[string]string
	arnIndex    map[string]arnEntry // ARN → (kind, name) for O(1) cross-kind lookup
	registry    *store.Registry
	mu          *lockmetrics.RWMutex
	accountID   string
	region      string
}

// NewInMemoryBackend returns a stateful Amazon Forecast backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	if accountID == "" {
		accountID = defaultAccountID
	}
	if region == "" {
		region = defaultRegion
	}

	b := &InMemoryBackend{
		evaluations: make(map[string][]MonitorEvaluation),
		tags:        make(map[string]map[string]string),
		arnIndex:    make(map[string]arnEntry),
		registry:    store.NewRegistry(),
		accountID:   accountID,
		region:      region,
		mu:          lockmetrics.New("forecast"),
	}
	registerAllTables(b)

	return b
}

// Reset clears all in-memory Forecast state. It supports the
// /_gopherstack/reset test hook so suites start from a clean slate.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.evaluations = make(map[string][]MonitorEvaluation)
	b.tags = make(map[string]map[string]string)
	b.arnIndex = make(map[string]arnEntry)
}

// Region returns backend region.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns backend account.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

func (b *InMemoryBackend) create(
	kind resourceKind, action, name string, data map[string]any, failureMessage string,
) (*Resource, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: resource name is required", ErrValidation)
	}

	if len(name) > maxResourceNameLen {
		return nil, fmt.Errorf(
			"%w: resource name must not exceed %d characters; got %d",
			ErrValidation, maxResourceNameLen, len(name),
		)
	}

	if !resourceNameRegex.MatchString(name) {
		return nil, fmt.Errorf(
			"%w: resource name %q contains invalid characters "+
				"(only alphanumeric, underscore, and hyphen are allowed)",
			ErrValidation, name,
		)
	}

	b.mu.Lock("create")
	defer b.mu.Unlock()

	if err := b.validateCreateFieldsLocked(kind, action, data); err != nil {
		return nil, err
	}

	table := b.resources[kind]
	if table.Has(name) {
		return nil, fmt.Errorf("%w: %s %q", ErrAlreadyExists, kind, name)
	}

	now := time.Now().UTC()
	status := statusCreatePending
	if failureMessage != "" {
		status = statusCreateFailed
	}

	resource := &Resource{
		CreatedAt: now,
		UpdatedAt: now,
		Data:      cloneMap(data),
		ARN:       arn.Build("forecast", b.region, b.accountID, string(kind)+"/"+name),
		Name:      name,
		Status:    status,
		Message:   failureMessage,
		Kind:      kind,
	}
	table.Put(resource)
	b.arnIndex[resource.ARN] = arnEntry{kind: kind, name: name}
	if tags := tagsFromInput(data); len(tags) > 0 {
		b.tags[resource.ARN] = tags
	}
	if kind == kindMonitor {
		b.evaluations[resource.ARN] = []MonitorEvaluation{newEvaluation(resource)}
	}

	return cloneResource(resource), nil
}

func (b *InMemoryBackend) describe(kind resourceKind, nameOrARN string) (*Resource, error) {
	b.mu.Lock("describe")
	defer b.mu.Unlock()

	resource, ok := b.lookupLocked(kind, nameOrARN)
	if !ok {
		return nil, fmt.Errorf("%w: %s %q", ErrNotFound, kind, nameOrARN)
	}

	result := cloneResource(resource)
	if resource.Status == statusCreatePending {
		resource.Status = statusActive
		resource.UpdatedAt = time.Now().UTC()
	}

	return result, nil
}

func (b *InMemoryBackend) update(kind resourceKind, nameOrARN string, data map[string]any) (*Resource, error) {
	b.mu.Lock("update")
	defer b.mu.Unlock()

	resource, ok := b.lookupLocked(kind, nameOrARN)
	if !ok {
		return nil, fmt.Errorf("%w: %s %q", ErrNotFound, kind, nameOrARN)
	}

	if err := b.validateUpdateFieldsLocked(kind, data); err != nil {
		return nil, err
	}

	for key, value := range data {
		resource.Data[key] = cloneValue(value)
	}
	resource.Status = statusActive
	resource.UpdatedAt = time.Now().UTC()

	return cloneResource(resource), nil
}

func (b *InMemoryBackend) delete(kind resourceKind, nameOrARN string) error {
	b.mu.Lock("delete")
	defer b.mu.Unlock()

	resource, ok := b.lookupLocked(kind, nameOrARN)
	if !ok {
		return fmt.Errorf("%w: %s %q", ErrNotFound, kind, nameOrARN)
	}

	if err := validateDeletableLocked(resource); err != nil {
		return err
	}

	b.resources[kind].Delete(resource.Name)
	delete(b.arnIndex, resource.ARN)
	delete(b.evaluations, resource.ARN)
	delete(b.tags, resource.ARN)

	return nil
}

func (b *InMemoryBackend) list(kind resourceKind) []*Resource {
	b.mu.RLock("list")
	defer b.mu.RUnlock()

	items := b.resources[kind].All()
	result := make([]*Resource, 0, len(items))
	for _, resource := range items {
		result = append(result, cloneResource(resource))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	return result
}

// latestMonitorEvaluation returns the most recently created evaluation for
// monitorARN, for DescribeMonitor's LastEvaluationState/LastEvaluationTime
// (both real members of DescribeMonitorOutput, forecast@v1.44.4
// api_op_DescribeMonitor.go). Evaluations are appended in creation order, so
// the last element is the most recent.
func (b *InMemoryBackend) latestMonitorEvaluation(monitorARN string) (MonitorEvaluation, bool) {
	b.mu.RLock("latestMonitorEvaluation")
	defer b.mu.RUnlock()

	evaluations := b.evaluations[monitorARN]
	if len(evaluations) == 0 {
		return MonitorEvaluation{}, false
	}

	return evaluations[len(evaluations)-1], true
}

func (b *InMemoryBackend) listMonitorEvaluations(monitorARN string) ([]MonitorEvaluation, error) {
	b.mu.RLock("listMonitorEvaluations")
	defer b.mu.RUnlock()

	if _, ok := b.lookupLocked(kindMonitor, monitorARN); !ok {
		return nil, fmt.Errorf("%w: monitor %q", ErrNotFound, monitorARN)
	}

	evaluations := b.evaluations[monitorARN]
	result := make([]MonitorEvaluation, len(evaluations))
	copy(result, evaluations)

	return result, nil
}

func (b *InMemoryBackend) lookupLocked(kind resourceKind, nameOrARN string) (*Resource, bool) {
	table := b.resources[kind]

	// Fast path: the table is keyed by name, so a name lookup is O(1).
	if resource, ok := table.Get(nameOrARN); ok {
		return resource, true
	}

	// ARN lookup: every ARN is built deterministically as
	// arn:...:forecast:region:account:<kind>/<name>, so reverse it to the name
	// and look that up directly rather than scanning the whole kind's table.
	if name, ok := b.nameFromARN(kind, nameOrARN); ok {
		if resource, found := table.Get(name); found && resource.ARN == nameOrARN {
			return resource, true
		}
	}

	return nil, false
}

// nameFromARN extracts the resource name from a Forecast ARN of the form
// arn:...:forecast:region:account:<kind>/<name>. It returns false if the string
// is not an ARN with the expected "<kind>/" resource prefix.
func (b *InMemoryBackend) nameFromARN(kind resourceKind, candidate string) (string, bool) {
	prefix := arn.Build("forecast", b.region, b.accountID, string(kind)+"/")

	name, found := strings.CutPrefix(candidate, prefix)
	if !found || name == "" {
		return "", false
	}

	return name, true
}

func newEvaluation(monitor *Resource) MonitorEvaluation {
	return MonitorEvaluation{
		CreationTime:    monitor.CreatedAt,
		EvaluationTime:  monitor.CreatedAt,
		MonitorArn:      monitor.ARN,
		MonitorName:     monitor.Name,
		ResourceArn:     stringValue(monitor.Data["ResourceArn"]),
		Status:          statusActive,
		MetricResults:   []any{},
		EvaluationState: "SUCCESS",
	}
}

func cloneResource(resource *Resource) *Resource {
	copyResource := *resource
	copyResource.Data = cloneMap(resource.Data)

	return &copyResource
}

func cloneMap(data map[string]any) map[string]any {
	if data == nil {
		return make(map[string]any)
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return make(map[string]any)
	}

	var result map[string]any
	if unmarshalErr := json.Unmarshal(encoded, &result); unmarshalErr != nil {
		return make(map[string]any)
	}

	return result
}

func cloneValue(value any) any {
	return cloneMap(map[string]any{"value": value})["value"]
}

func stringValue(value any) string {
	result, _ := value.(string)

	return result
}

// UpdateResourceStatus handles StopResource and ResumeResource.
func (b *InMemoryBackend) UpdateResourceStatus(resourceARN, newStatus string) error {
	b.mu.Lock("UpdateResourceStatus")
	defer b.mu.Unlock()

	entry, ok := b.arnIndex[resourceARN]
	if !ok {
		return fmt.Errorf("%w: resource %q", ErrNotFound, resourceARN)
	}

	resource, _ := b.resources[entry.kind].Get(entry.name)
	resource.Status = newStatus
	resource.UpdatedAt = time.Now().UTC()

	return nil
}

// DeleteResourceTree deletes a resource and all dependent child resources
// transitively, mirroring AWS Forecast behavior.
func (b *InMemoryBackend) DeleteResourceTree(targetARN string) error {
	b.mu.Lock("DeleteResourceTree")
	defer b.mu.Unlock()

	if _, ok := b.arnIndex[targetARN]; !ok {
		return fmt.Errorf("%w: resource %q", ErrNotFound, targetARN)
	}

	b.deleteTreeLocked(targetARN)

	return nil
}

// deleteTreeLocked performs a BFS from targetARN to collect all resources that
// directly or indirectly reference it, then removes them all.
// Must be called with b.mu held for write.
func (b *InMemoryBackend) deleteTreeLocked(targetARN string) {
	toDelete := map[string]struct{}{targetARN: {}}
	queue := []string{targetARN}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, table := range b.resources {
			table.Range(func(r *Resource) bool {
				if _, already := toDelete[r.ARN]; already {
					return true
				}

				if arnReferencedBy(r, current) {
					toDelete[r.ARN] = struct{}{}
					queue = append(queue, r.ARN)
				}

				return true
			})
		}
	}

	for arnToDelete := range toDelete {
		if entry, ok := b.arnIndex[arnToDelete]; ok {
			b.resources[entry.kind].Delete(entry.name)
			delete(b.arnIndex, arnToDelete)
			delete(b.evaluations, arnToDelete)
			delete(b.tags, arnToDelete)
		}
	}
}

// arnReferencedBy returns true if any string value in r.Data equals
// targetARN, searched recursively: parent ARN references are not always
// top-level (e.g. CreatePredictorInput.InputDataConfig.DatasetGroupArn and
// CreateAutoPredictorInput.DataConfig.DatasetGroupArn nest the reference one
// level down), so DeleteResourceTree's cascade must look inside nested
// maps/slices too or it silently fails to find dependents like a
// DatasetGroup's Predictors.
func arnReferencedBy(r *Resource, targetARN string) bool {
	return valueReferencesARN(r.Data, targetARN)
}

func valueReferencesARN(v any, targetARN string) bool {
	switch value := v.(type) {
	case string:
		return value == targetARN
	case map[string]any:
		for _, nested := range value {
			if valueReferencesARN(nested, targetARN) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if valueReferencesARN(nested, targetARN) {
				return true
			}
		}
	}

	return false
}
