package redshift

import (
	"fmt"
	"strings"
	"time"
)

const (
	strFalse            = "false"
	sourceEngineDefault = "engine-default"
	applyTypeDynamic    = "dynamic"
	dataTypeString      = "string"
)

// ClusterParameter represents a single parameter within a cluster parameter group.
type ClusterParameter struct {
	ParameterName        string `json:"parameterName"`
	ParameterValue       string `json:"parameterValue"`
	Description          string `json:"description"`
	Source               string `json:"source"`
	DataType             string `json:"dataType"`
	ApplyType            string `json:"applyType"`
	MinimumEngineVersion string `json:"minimumEngineVersion"`
	IsModifiable         bool   `json:"isModifiable"`
}

// ClusterParameterGroup represents a Redshift cluster parameter group.
type ClusterParameterGroup struct {
	CreatedAt            time.Time          `json:"createdAt"`
	ParameterGroupName   string             `json:"parameterGroupName"`
	ParameterGroupFamily string             `json:"parameterGroupFamily"`
	Description          string             `json:"description"`
	Parameters           []ClusterParameter `json:"parameters"`
}

// defaultParameters returns a minimal set of default Redshift parameters.
func defaultParameters() []ClusterParameter {
	return []ClusterParameter{
		{
			ParameterName:  "enable_user_activity_logging",
			ParameterValue: strFalse,
			Description:    "enable logging of user activity",
			Source:         sourceEngineDefault,
			DataType:       "boolean",
			ApplyType:      "static",
			IsModifiable:   true,
		},
		{
			ParameterName:  "max_cursor_result_set_size",
			ParameterValue: "0",
			Description:    "max cursor result set size",
			Source:         sourceEngineDefault,
			DataType:       "integer",
			ApplyType:      applyTypeDynamic,
			IsModifiable:   true,
		},
		{
			ParameterName:  "query_group",
			ParameterValue: "default",
			Description:    "query group label",
			Source:         sourceEngineDefault,
			DataType:       dataTypeString,
			ApplyType:      applyTypeDynamic,
			IsModifiable:   true,
		},
		{
			ParameterName:  "search_path",
			ParameterValue: "$user, public",
			Description:    "sets the schema search order for names that are not schema-qualified",
			Source:         sourceEngineDefault,
			DataType:       dataTypeString,
			ApplyType:      applyTypeDynamic,
			IsModifiable:   true,
		},
		{
			ParameterName:  "statement_timeout",
			ParameterValue: "0",
			Description:    "abort any statement that takes more than the specified number of milliseconds",
			Source:         sourceEngineDefault,
			DataType:       "integer",
			ApplyType:      applyTypeDynamic,
			IsModifiable:   true,
		},
		{
			ParameterName:  "wlm_json_configuration",
			ParameterValue: `[{"auto_wlm":true}]`,
			Description:    "Workload management configuration",
			Source:         sourceEngineDefault,
			DataType:       dataTypeString,
			ApplyType:      applyTypeDynamic,
			IsModifiable:   true,
		},
	}
}

// CreateClusterParameterGroup creates a new cluster parameter group.
func (b *InMemoryBackend) CreateClusterParameterGroup(
	name, family, description string,
) (*ClusterParameterGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: ParameterGroupName is required", ErrInvalidParameter)
	}
	if family == "" {
		return nil, fmt.Errorf("%w: ParameterGroupFamily is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateClusterParameterGroup")
	defer b.mu.Unlock()

	if _, exists := b.parameterGroups.Get(name); exists {
		return nil, fmt.Errorf("%w: parameter group %s already exists", ErrParameterGroupAlreadyExists, name)
	}

	pg := &ClusterParameterGroup{
		ParameterGroupName:   name,
		ParameterGroupFamily: family,
		Description:          description,
		Parameters:           defaultParameters(),
		CreatedAt:            time.Now(),
	}
	b.parameterGroups.Put(pg)

	cp := cloneParameterGroup(pg)

	return &cp, nil
}

// defaultParameterGroupPrefix names the auto-provisioned parameter groups
// every account has, one per parameter group family (e.g.
// "default.redshift-1.0"). Real AWS: DeleteClusterParameterGroupInput's own
// doc comment, "Cannot delete a default cluster parameter group".
const defaultParameterGroupPrefix = "default."

// DeleteClusterParameterGroup removes a cluster parameter group. Real AWS:
// "You cannot delete a parameter group if it is associated with a cluster,"
// and its own ParameterGroupName cannot name a default parameter group.
func (b *InMemoryBackend) DeleteClusterParameterGroup(name string) error {
	if name == "" {
		return fmt.Errorf("%w: ParameterGroupName is required", ErrInvalidParameter)
	}

	if strings.HasPrefix(name, defaultParameterGroupPrefix) {
		return fmt.Errorf("%w: cannot delete a default parameter group", ErrParameterGroupInvalidState)
	}

	b.mu.Lock("DeleteClusterParameterGroup")
	defer b.mu.Unlock()

	if _, exists := b.parameterGroups.Get(name); !exists {
		return fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, name)
	}

	for _, c := range b.clusters.All() {
		if c.ClusterParameterGroupName == name {
			return fmt.Errorf(
				"%w: parameter group %s is associated with cluster %s",
				ErrParameterGroupInvalidState, name, c.ClusterIdentifier,
			)
		}
	}

	b.parameterGroups.Delete(name)

	return nil
}

// DescribeClusterParameterGroups returns all parameter groups, or a specific one if name is non-empty.
func (b *InMemoryBackend) DescribeClusterParameterGroups(name string) ([]ClusterParameterGroup, error) {
	b.mu.RLock("DescribeClusterParameterGroups")
	defer b.mu.RUnlock()

	if name != "" {
		pg, exists := b.parameterGroups.Get(name)
		if !exists {
			return nil, fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, name)
		}

		cp := cloneParameterGroup(pg)

		return []ClusterParameterGroup{cp}, nil
	}

	result := make([]ClusterParameterGroup, 0, b.parameterGroups.Len())
	for _, pg := range b.parameterGroups.All() {
		result = append(result, cloneParameterGroup(pg))
	}

	return result, nil
}

// DescribeClusterParameters returns the parameters for a specific parameter group.
func (b *InMemoryBackend) DescribeClusterParameters(groupName string) ([]ClusterParameter, error) {
	if groupName == "" {
		return nil, fmt.Errorf("%w: ParameterGroupName is required", ErrInvalidParameter)
	}

	b.mu.RLock("DescribeClusterParameters")
	defer b.mu.RUnlock()

	pg, exists := b.parameterGroups.Get(groupName)
	if !exists {
		return nil, fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, groupName)
	}

	params := make([]ClusterParameter, len(pg.Parameters))
	copy(params, pg.Parameters)

	return params, nil
}

// ModifyClusterParameterGroup updates parameters within a parameter group.
func (b *InMemoryBackend) ModifyClusterParameterGroup(
	groupName string,
	params []ClusterParameter,
) (*ClusterParameterGroup, error) {
	if groupName == "" {
		return nil, fmt.Errorf("%w: ParameterGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyClusterParameterGroup")
	defer b.mu.Unlock()

	pg, exists := b.parameterGroups.Get(groupName)
	if !exists {
		return nil, fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, groupName)
	}

	// Update existing parameters by name; add new ones if not found.
	for _, newParam := range params {
		found := false

		for i, existing := range pg.Parameters {
			if existing.ParameterName == newParam.ParameterName {
				pg.Parameters[i].ParameterValue = newParam.ParameterValue
				pg.Parameters[i].Source = "user"
				found = true

				break
			}
		}

		if !found {
			newParam.Source = "user"
			pg.Parameters = append(pg.Parameters, newParam)
		}
	}

	cp := cloneParameterGroup(pg)

	return &cp, nil
}

// ResetClusterParameterGroup resets all or specific parameters to their default values.
func (b *InMemoryBackend) ResetClusterParameterGroup(
	groupName string,
	resetAllParameters bool,
	params []ClusterParameter,
) (*ClusterParameterGroup, error) {
	if groupName == "" {
		return nil, fmt.Errorf("%w: ParameterGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("ResetClusterParameterGroup")
	defer b.mu.Unlock()

	pg, exists := b.parameterGroups.Get(groupName)
	if !exists {
		return nil, fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, groupName)
	}

	defaults := defaultParameters()
	defaultMap := make(map[string]ClusterParameter, len(defaults))

	for _, d := range defaults {
		defaultMap[d.ParameterName] = d
	}

	if resetAllParameters {
		pg.Parameters = defaults
	} else {
		for _, p := range params {
			for i, existing := range pg.Parameters {
				if existing.ParameterName == p.ParameterName {
					if def, ok := defaultMap[p.ParameterName]; ok {
						pg.Parameters[i] = def
					}

					break
				}
			}
		}
	}

	cp := cloneParameterGroup(pg)

	return &cp, nil
}

// DescribeDefaultClusterParameters returns the default parameters for a given parameter group family.
func (b *InMemoryBackend) DescribeDefaultClusterParameters(family string) ([]ClusterParameter, error) {
	if family == "" {
		return nil, fmt.Errorf("%w: ParameterGroupFamily is required", ErrInvalidParameter)
	}

	return defaultParameters(), nil
}

// cloneParameterGroup returns a deep copy of a ClusterParameterGroup.
func cloneParameterGroup(pg *ClusterParameterGroup) ClusterParameterGroup {
	cp := *pg
	cp.Parameters = make([]ClusterParameter, len(pg.Parameters))
	copy(cp.Parameters, pg.Parameters)

	return cp
}

// AddParameterGroupInternal seeds a parameter group directly into the backend.
func (b *InMemoryBackend) AddParameterGroupInternal(pg *ClusterParameterGroup) {
	b.mu.Lock("AddParameterGroupInternal")
	defer b.mu.Unlock()
	b.parameterGroups.Put(pg)
}
