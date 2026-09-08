package securityhub

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) insightARN(seq int) string {
	return arn.Build("securityhub", b.region, b.accountID, fmt.Sprintf("insight/%s/%d", b.accountID, seq))
}

func (b *InMemoryBackend) CreateInsight(name, groupByAttribute string, filters map[string]any) (string, error) {
	b.mu.Lock("CreateInsight")
	defer b.mu.Unlock()

	if !b.hubEnabled {
		return "", ErrHubNotEnabled
	}

	b.insightSeq++
	arn := b.insightARN(b.insightSeq)

	b.insights.Put(&Insight{
		InsightArn:       arn,
		Name:             name,
		GroupByAttribute: groupByAttribute,
		Filters:          filters,
	})

	return arn, nil
}

func (b *InMemoryBackend) GetInsights(
	insightArns []string,
	nextToken string,
	maxResults int,
) ([]*Insight, string, error) {
	b.mu.RLock("GetInsights")
	defer b.mu.RUnlock()

	if !b.hubEnabled {
		return nil, "", ErrHubNotEnabled
	}

	var results []*Insight

	if len(insightArns) > 0 {
		for _, arn := range insightArns {
			if insight, ok := b.insights.Get(arn); ok {
				results = append(results, insight)
			}
		}
	} else {
		results = b.insights.All()
	}

	if maxResults <= 0 || maxResults > 100 {
		maxResults = 100
	}

	start := decodeToken(nextToken)
	if start >= len(results) {
		return []*Insight{}, "", nil
	}

	end := start + maxResults
	end = min(end, len(results))

	page := results[start:end]
	nextOut := ""

	if end < len(results) {
		nextOut = encodeToken(end)
	}

	return page, nextOut, nil
}

func (b *InMemoryBackend) UpdateInsight(insightArn, name, groupByAttribute string, filters map[string]any) error {
	b.mu.Lock("UpdateInsight")
	defer b.mu.Unlock()

	if !b.hubEnabled {
		return ErrHubNotEnabled
	}

	insight, ok := b.insights.Get(insightArn)
	if !ok {
		return fmt.Errorf("%w: insight %s", ErrNotFound, insightArn)
	}

	if name != "" {
		insight.Name = name
	}

	if groupByAttribute != "" {
		insight.GroupByAttribute = groupByAttribute
	}

	if filters != nil {
		insight.Filters = filters
	}

	return nil
}

func (b *InMemoryBackend) DeleteInsight(insightArn string) (string, error) {
	b.mu.Lock("DeleteInsight")
	defer b.mu.Unlock()

	if !b.hubEnabled {
		return "", ErrHubNotEnabled
	}

	if !b.insights.Delete(insightArn) {
		return "", fmt.Errorf("%w: insight %s", ErrNotFound, insightArn)
	}

	return insightArn, nil
}

func (b *InMemoryBackend) GetInsightResults(insightArn string) (*InsightResults, error) {
	b.mu.RLock("GetInsightResults")
	defer b.mu.RUnlock()

	if !b.hubEnabled {
		return nil, ErrHubNotEnabled
	}

	insight, ok := b.insights.Get(insightArn)
	if !ok {
		return nil, fmt.Errorf("%w: insight %s", ErrNotFound, insightArn)
	}

	return &InsightResults{
		InsightArn:       insightArn,
		GroupByAttribute: insight.GroupByAttribute,
		ResultValues:     b.aggregateInsightResults(insight),
	}, nil
}

// aggregateInsightResults groups every stored finding matching insight's
// Filters by insight.GroupByAttribute, counting occurrences per distinct
// value -- gopherstack-1qf: this backend previously always returned empty
// ResultValues regardless of Filters/GroupByAttribute or how many findings
// existed. Filters is evaluated via matchesFindingFilters (same
// field-name-mapped subset GetFindings already supports) and
// GroupByAttribute is resolved via findingFieldString, which already
// handles the SeverityLabel/WorkflowStatus/ComplianceStatus nested-field
// cases -- reusing both keeps this consistent with GetFindings rather than
// inventing a second filter/field-resolution implementation.
func (b *InMemoryBackend) aggregateInsightResults(insight *Insight) []map[string]any {
	counts := make(map[string]int)

	for _, f := range b.findings {
		if !matchesFindingFilters(f, insight.Filters) {
			continue
		}

		counts[findingFieldString(f, insight.GroupByAttribute)]++
	}

	values := make([]string, 0, len(counts))
	for v := range counts {
		values = append(values, v)
	}

	sort.Strings(values)

	resultValues := make([]map[string]any, 0, len(values))
	for _, v := range values {
		resultValues = append(resultValues, map[string]any{
			"GroupByAttributeValue": v,
			"Count":                 counts[v],
		})
	}

	return resultValues
}
