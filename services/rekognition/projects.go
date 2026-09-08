package rekognition

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) projectARN(name string) string {
	return arn.Build("rekognition", b.region, b.accountID, fmt.Sprintf("project/%s", name))
}

// =============================================================================
// Projects
// =============================================================================

// defaultProjectFeature is CreateProjectInput.Feature's documented default
// ("If no value is provided CUSTOM_LABELS is used as a default.",
// api_op_CreateProject.go).
const defaultProjectFeature = "CUSTOM_LABELS"

// CreateProject creates a new Rekognition Custom Labels project.
//
// CreateProjectInput.Tags is deliberately NOT accepted here: unlike
// Collection/StreamProcessor/model tags, TagResource's and
// ListTagsForResource's own docs scope ResourceArn to "the model,
// collection, or stream processor" -- Project ARNs are absent from both, so
// this backend's own API surface has no read path that could ever observe
// project tags, real or fabricated. Left disclosed rather than half-wired.
func (b *InMemoryBackend) CreateProject(name string, params CreateProjectParams) (*Project, error) {
	b.mu.Lock("CreateProject")
	defer b.mu.Unlock()

	arn := b.projectARN(name)

	if b.projects.Has(arn) {
		return nil, ErrProjectAlreadyExists
	}

	feature := params.Feature
	if feature == "" {
		feature = defaultProjectFeature
	}

	p := &storedProject{
		CreationTimestamp: time.Now(),
		ProjectARN:        arn,
		Name:              name,
		Status:            "CREATING",
		AutoUpdate:        params.AutoUpdate,
		Feature:           feature,
	}
	b.projects.Put(p)

	return p.toProject(), nil
}

// DeleteProject deletes a project. DeleteProjectInput's own doc comment
// (api_op_DeleteProject.go): "To delete a project you must first delete all
// models or adapters associated with the project." and "deleting a given
// project will also delete any ProjectPolicies associated with that
// project" -- so ProjectPolicies cascade while ProjectVersions block.
func (b *InMemoryBackend) DeleteProject(projectARN string) error {
	b.mu.Lock("DeleteProject")
	defer b.mu.Unlock()

	if !b.projects.Has(projectARN) {
		return ErrProjectNotFound
	}

	var hasVersions bool

	b.projectVersions.Range(func(v *storedProjectVersion) bool {
		if v.ProjectARN == projectARN {
			hasVersions = true

			return false
		}

		return true
	})

	if hasVersions {
		return ErrProjectHasVersions
	}

	for _, p := range slices.Clone(b.projectPoliciesByProject.Get(projectARN)) {
		b.projectPolicies.Delete(projectPolicyKey(p.ProjectARN, p.PolicyName))
	}

	b.projects.Delete(projectARN)

	return nil
}

// DescribeProjects lists projects, optionally filtered by name and/or
// customization feature. DescribeProjectsInput.ProjectNames filters by name
// (see storedProject's doc comment), not by ARN -- there is no ProjectArns
// filter member on the real input at all. An absent/empty features filter
// defaults to CUSTOM_LABELS only (own doc comment, api_op_DescribeProjects.go).
func (b *InMemoryBackend) DescribeProjects(
	projectNames, features []string, maxResults int32, nextToken string,
) ([]*Project, string, error) {
	b.mu.RLock("DescribeProjects")
	defer b.mu.RUnlock()

	// store.Table.Snapshot returns items ordered by key (ProjectARN), ascending.
	items := b.projects.Snapshot()

	// Build a filter set if requested.
	filter := make(map[string]bool, len(projectNames))
	for _, name := range projectNames {
		filter[name] = true
	}

	if len(features) == 0 {
		features = []string{defaultProjectFeature}
	}
	featureFilter := make(map[string]bool, len(features))
	for _, f := range features {
		featureFilter[f] = true
	}

	// Apply nextToken offset.
	start := 0
	if nextToken != "" {
		for i, v := range items {
			if v.ProjectARN == nextToken {
				start = i

				break
			}
		}
	}

	const maxPerPage = 100
	limit := int32(maxPerPage)
	if maxResults > 0 && maxResults < limit {
		limit = maxResults
	}

	var result []*Project
	var outToken string
	count := int32(0)

	for i := start; i < len(items); i++ {
		v := items[i]
		if len(filter) > 0 && !filter[v.Name] {
			continue
		}
		if !featureFilter[v.Feature] {
			continue
		}

		if count >= limit {
			outToken = v.ProjectARN

			break
		}

		result = append(result, v.toProject())
		count++
	}

	return result, outToken, nil
}

// =============================================================================
// Project Policies
// =============================================================================

// ListProjectPolicies lists policies for a project.
func (b *InMemoryBackend) ListProjectPolicies(
	projectARN string, maxResults int32, nextToken string,
) ([]*ProjectPolicy, string, error) {
	b.mu.RLock("ListProjectPolicies")
	defer b.mu.RUnlock()

	// Index result slices are insertion-ordered, not sorted by PolicyName --
	// clone (per the Index.Get contract) and sort to match the original
	// nested-map's collections.SortedKeys(policyName) pagination order.
	group := slices.Clone(b.projectPoliciesByProject.Get(projectARN))
	slices.SortFunc(group, func(a, c *storedProjectPolicy) int { return strings.Compare(a.PolicyName, c.PolicyName) })

	start := 0
	if nextToken != "" {
		for i, p := range group {
			if p.PolicyName == nextToken {
				start = i

				break
			}
		}
	}

	// ListProjectPoliciesInput.MaxResults doc: "The largest value you can
	// specify is 5 ... The default value is 5" -- unlike every other
	// List/Describe op in this service, which default/cap at 100.
	const maxPerPage = 5
	limit := int32(maxPerPage)
	if maxResults > 0 && maxResults < limit {
		limit = maxResults
	}

	end := min(start+int(limit), len(group))

	result := make([]*ProjectPolicy, 0, end-start)
	for _, p := range group[start:end] {
		result = append(result, p.toProjectPolicy())
	}

	var outToken string
	if end < len(group) {
		outToken = group[end].PolicyName
	}

	return result, outToken, nil
}

// PutProjectPolicy creates or updates a project policy.
func (b *InMemoryBackend) PutProjectPolicy(
	projectARN, policyName, policyDocument, policyRevisionID string, //nolint:revive // existing issue.
) (string, error) {
	b.mu.Lock("PutProjectPolicy")
	defer b.mu.Unlock()

	if !b.projects.Has(projectARN) {
		return "", ErrProjectNotFound
	}

	now := time.Now()
	newRevID := uuid.NewString()

	key := projectPolicyKey(projectARN, policyName)

	existing, exists := b.projectPolicies.Get(key)
	if exists {
		existing.LastUpdatedTimestamp = now
		existing.PolicyDocument = policyDocument
		existing.PolicyRevisionID = newRevID
	} else {
		b.projectPolicies.Put(&storedProjectPolicy{
			CreationTimestamp:    now,
			LastUpdatedTimestamp: now,
			ProjectARN:           projectARN,
			PolicyName:           policyName,
			PolicyRevisionID:     newRevID,
			PolicyDocument:       policyDocument,
		})
	}

	return newRevID, nil
}

// DeleteProjectPolicy deletes a project policy.
func (b *InMemoryBackend) DeleteProjectPolicy(
	projectARN, policyName, policyRevisionID string, //nolint:revive // existing issue.
) error {
	b.mu.Lock("DeleteProjectPolicy")
	defer b.mu.Unlock()

	// Mirrors the original nested-map's two-level existence check: a project
	// with no policies at all (never an entry in the outer map) is
	// indistinguishable from one whose named policy is simply missing.
	if len(b.projectPoliciesByProject.Get(projectARN)) == 0 {
		return ErrProjectNotFound
	}

	key := projectPolicyKey(projectARN, policyName)
	if !b.projectPolicies.Has(key) {
		return ErrProjectNotFound
	}

	b.projectPolicies.Delete(key)

	return nil
}
