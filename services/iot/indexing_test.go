package iot_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

func TestBackend_IndexingConfiguration_DefaultIsOff(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	out := b.GetIndexingConfiguration()
	require.NotNil(t, out.ThingIndexingConfiguration)
	require.NotNil(t, out.ThingGroupIndexingConfiguration)
	assert.Equal(t, "OFF", out.ThingIndexingConfiguration.ThingIndexingMode)
	assert.Equal(t, "OFF", out.ThingGroupIndexingConfiguration.ThingGroupIndexingMode)

	assert.Empty(t, b.ListIndices())
}

func TestBackend_UpdateIndexingConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   *iot.UpdateIndexingConfigurationInput
		name    string
		wantErr bool
	}{
		{
			name: "enable_thing_indexing",
			input: &iot.UpdateIndexingConfigurationInput{
				ThingIndexingConfiguration: &iot.ThingIndexingConfiguration{
					ThingIndexingMode:             "REGISTRY_AND_SHADOW",
					ThingConnectivityIndexingMode: "STATUS",
					NamedShadowIndexingMode:       "ON",
				},
			},
		},
		{
			name: "enable_thing_group_indexing",
			input: &iot.UpdateIndexingConfigurationInput{
				ThingGroupIndexingConfiguration: &iot.ThingGroupIndexingConfiguration{
					ThingGroupIndexingMode: "ON",
				},
			},
		},
		{
			name: "invalid_thing_indexing_mode",
			input: &iot.UpdateIndexingConfigurationInput{
				ThingIndexingConfiguration: &iot.ThingIndexingConfiguration{ThingIndexingMode: "BOGUS"},
			},
			wantErr: true,
		},
		{
			name: "invalid_thing_group_indexing_mode",
			input: &iot.UpdateIndexingConfigurationInput{
				ThingGroupIndexingConfiguration: &iot.ThingGroupIndexingConfiguration{ThingGroupIndexingMode: "BOGUS"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()

			err := b.UpdateIndexingConfiguration(tt.input)
			if tt.wantErr {
				require.ErrorIs(t, err, iot.ErrValidation)

				return
			}

			require.NoError(t, err)

			out := b.GetIndexingConfiguration()
			if tt.input.ThingIndexingConfiguration != nil {
				assert.Equal(
					t,
					tt.input.ThingIndexingConfiguration.ThingIndexingMode,
					out.ThingIndexingConfiguration.ThingIndexingMode,
				)
			}

			if tt.input.ThingGroupIndexingConfiguration != nil {
				assert.Equal(
					t,
					tt.input.ThingGroupIndexingConfiguration.ThingGroupIndexingMode,
					out.ThingGroupIndexingConfiguration.ThingGroupIndexingMode,
				)
			}
		})
	}
}

func TestBackend_ListIndices_ReflectsConfiguration(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	assert.Empty(t, b.ListIndices())

	require.NoError(t, b.UpdateIndexingConfiguration(&iot.UpdateIndexingConfigurationInput{
		ThingIndexingConfiguration: &iot.ThingIndexingConfiguration{ThingIndexingMode: "REGISTRY"},
	}))
	assert.Equal(t, []string{"AWS_Things"}, b.ListIndices())

	require.NoError(t, b.UpdateIndexingConfiguration(&iot.UpdateIndexingConfigurationInput{
		ThingGroupIndexingConfiguration: &iot.ThingGroupIndexingConfiguration{ThingGroupIndexingMode: "ON"},
	}))
	assert.ElementsMatch(t, []string{"AWS_Things", "AWS_ThingGroups"}, b.ListIndices())
}

func TestBackend_DescribeIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		indexName string
		wantErr   bool
	}{
		{name: "things_index", indexName: "AWS_Things"},
		{name: "thing_groups_index", indexName: "AWS_ThingGroups"},
		{name: "unknown_index", indexName: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()

			idx, err := b.DescribeIndex(tt.indexName)
			if tt.wantErr {
				require.ErrorIs(t, err, iot.ErrIndexNotFound)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.indexName, idx.IndexName)
			assert.Equal(t, "ACTIVE", idx.IndexStatus)
			assert.NotEmpty(t, idx.Schema)
		})
	}
}

func seedThingsForSearch(t *testing.T, b *iot.InMemoryBackend) {
	t.Helper()

	things := []struct {
		attrs    map[string]string
		name     string
		typeName string
	}{
		{
			name: "sensor-1", typeName: "TemperatureSensor",
			attrs: map[string]string{"location": "lab-a", "temperature": "21.5"},
		},
		{
			name: "sensor-2", typeName: "TemperatureSensor",
			attrs: map[string]string{"location": "lab-b", "temperature": "30.0"},
		},
		{
			name: "sensor-3", typeName: "HumiditySensor",
			attrs: map[string]string{"location": "lab-a", "temperature": "18.25"},
		},
	}

	for _, th := range things {
		_, err := b.CreateThing(&iot.CreateThingInput{
			ThingName:        th.name,
			ThingTypeName:    th.typeName,
			AttributePayload: &iot.AttributePayload{Attributes: th.attrs},
		})
		require.NoError(t, err)
	}
}

func TestBackend_SearchIndex_Things(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		queryString string
		wantNames   []string
	}{
		{name: "empty_matches_all", queryString: "", wantNames: []string{"sensor-1", "sensor-2", "sensor-3"}},
		{name: "substring_thing_name", queryString: "sensor-1", wantNames: []string{"sensor-1"}},
		{name: "thing_type_name_key", queryString: "thingTypeName:HumiditySensor", wantNames: []string{"sensor-3"}},
		{
			name:        "attribute_key_value",
			queryString: "attributes.location:lab-a",
			wantNames:   []string{"sensor-1", "sensor-3"},
		},
		{name: "attribute_substring", queryString: "lab-b", wantNames: []string{"sensor-2"}},
		{name: "no_match", queryString: "attributes.location:nowhere", wantNames: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()
			seedThingsForSearch(t, b)

			out, err := b.SearchIndex(&iot.SearchIndexInput{QueryString: tt.queryString})
			require.NoError(t, err)

			gotNames := make([]string, 0, len(out.Things))
			for _, th := range out.Things {
				gotNames = append(gotNames, th.ThingName)
			}

			assert.ElementsMatch(t, tt.wantNames, gotNames)
		})
	}
}

func TestBackend_SearchIndex_Pagination(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	seedThingsForSearch(t, b)

	page1, err := b.SearchIndex(&iot.SearchIndexInput{MaxResults: 2})
	require.NoError(t, err)
	assert.Len(t, page1.Things, 2)
	assert.NotEmpty(t, page1.NextToken)

	page2, err := b.SearchIndex(&iot.SearchIndexInput{MaxResults: 2, NextToken: page1.NextToken})
	require.NoError(t, err)
	assert.Len(t, page2.Things, 1)
	assert.Empty(t, page2.NextToken)
}

func TestBackend_SearchIndex_UnknownIndex(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	_, err := b.SearchIndex(&iot.SearchIndexInput{IndexName: "bogus"})
	require.ErrorIs(t, err, iot.ErrValidation)
}

func TestBackend_SearchIndex_ThingGroups(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{
		ThingGroupName: "floor-1",
		Attributes:     map[string]string{"building": "hq"},
	})
	require.NoError(t, err)

	_, err = b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "floor-2"})
	require.NoError(t, err)

	out, err := b.SearchIndex(&iot.SearchIndexInput{
		IndexName:   "AWS_ThingGroups",
		QueryString: "attributes.building:hq",
	})
	require.NoError(t, err)
	require.Len(t, out.ThingGroups, 1)
	assert.Equal(t, "floor-1", out.ThingGroups[0].ThingGroupName)
}

// TestBackend_SearchIndex_ThingGroupWireShape covers SearchIndex's
// ThingGroupDocument shape: "parentGroupNames" is the full ancestor chain
// as a list, not a single "parentGroupName" string, and
// "thingGroupDescription" is present (aws-sdk-go-v2/service/iot@v1.77.4's
// awsRestjson1_deserializeDocumentThingGroupDocument).
func TestBackend_SearchIndex_ThingGroupWireShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		groupName          string
		wantDescription    string
		wantParentGroupSeq []string
	}{
		{
			name:               "root_group_has_no_parents",
			groupName:          "root",
			wantDescription:    "root group",
			wantParentGroupSeq: nil,
		},
		{
			name:               "child_group_has_one_parent",
			groupName:          "child",
			wantDescription:    "child group",
			wantParentGroupSeq: []string{"root"},
		},
		{
			name:               "grandchild_group_has_full_ancestor_chain",
			groupName:          "grandchild",
			wantDescription:    "grandchild group",
			wantParentGroupSeq: []string{"child", "root"},
		},
	}

	b := iot.NewInMemoryBackend()

	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{
		ThingGroupName: "root", Description: "root group",
	})
	require.NoError(t, err)
	_, err = b.CreateThingGroup(&iot.CreateThingGroupInput{
		ThingGroupName: "child", Description: "child group", ParentGroupName: "root",
	})
	require.NoError(t, err)
	_, err = b.CreateThingGroup(&iot.CreateThingGroupInput{
		ThingGroupName: "grandchild", Description: "grandchild group", ParentGroupName: "child",
	})
	require.NoError(t, err)

	out, err := b.SearchIndex(&iot.SearchIndexInput{IndexName: "AWS_ThingGroups"})
	require.NoError(t, err)

	byName := make(map[string]*iot.SearchIndexThingGroupResult, len(out.ThingGroups))
	for _, g := range out.ThingGroups {
		byName[g.ThingGroupName] = g
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g, ok := byName[tt.groupName]
			require.True(t, ok, "group %q missing from SearchIndex results", tt.groupName)
			assert.Equal(t, tt.wantDescription, g.ThingGroupDescription)
			assert.Equal(t, tt.wantParentGroupSeq, g.ParentGroupNames)
		})
	}
}

func seedNumericThings(t *testing.T, b *iot.InMemoryBackend) {
	t.Helper()

	values := []string{"10", "20", "30", "40", "50"}
	for i, v := range values {
		_, err := b.CreateThing(&iot.CreateThingInput{
			ThingName:        "device-" + string(rune('a'+i)),
			AttributePayload: &iot.AttributePayload{Attributes: map[string]string{"temperature": v}},
		})
		require.NoError(t, err)
	}
}

func TestBackend_GetCardinality(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	seedNumericThings(t, b)

	card, err := b.GetCardinality(&iot.AggregationInput{AggregationField: "attributes.temperature"})
	require.NoError(t, err)
	assert.Equal(t, int64(5), card)
}

func TestBackend_GetCardinality_MissingField(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	_, err := b.GetCardinality(&iot.AggregationInput{})
	require.ErrorIs(t, err, iot.ErrValidation)
}

func TestBackend_GetStatistics(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	seedNumericThings(t, b)

	stats, err := b.GetStatistics(&iot.AggregationInput{AggregationField: "attributes.temperature"})
	require.NoError(t, err)
	assert.Equal(t, int64(5), stats.Count)
	assert.InDelta(t, 10, stats.Minimum, 0.001)
	assert.InDelta(t, 50, stats.Maximum, 0.001)
	assert.InDelta(t, 30, stats.Average, 0.001)
	assert.InDelta(t, 150, stats.Sum, 0.001)
	// Regression: SumOfSquares was entirely missing from the Statistics type
	// (and therefore from GetStatistics' wire response) despite being a real
	// field on aws-sdk-go-v2/service/iot/types.Statistics.
	assert.InDelta(t, 5500, stats.SumOfSquares, 0.001) // 10^2+20^2+30^2+40^2+50^2
	assert.Greater(t, stats.StdDeviation, 0.0)
}

func TestBackend_GetPercentiles(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	seedNumericThings(t, b)

	percentiles, err := b.GetPercentiles(&iot.PercentilesInput{
		AggregationField: "attributes.temperature",
		Percents:         []float64{0, 50, 100},
	})
	require.NoError(t, err)
	require.Len(t, percentiles, 3)
	assert.InDelta(t, 10, percentiles[0].Value, 0.001)
	assert.InDelta(t, 30, percentiles[1].Value, 0.001)
	assert.InDelta(t, 50, percentiles[2].Value, 0.001)
}

func TestBackend_GetBucketsAggregation(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	seedThingsForSearch(t, b)

	buckets, err := b.GetBucketsAggregation(&iot.BucketsAggregationInput{
		AggregationField: "thingTypeName",
	})
	require.NoError(t, err)
	require.Len(t, buckets, 2)
	assert.Equal(t, "TemperatureSensor", buckets[0].KeyValue)
	assert.Equal(t, int64(2), buckets[0].Count)
	assert.Equal(t, "HumiditySensor", buckets[1].KeyValue)
	assert.Equal(t, int64(1), buckets[1].Count)
}

func TestBackend_GetBucketsAggregation_MissingField(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	_, err := b.GetBucketsAggregation(&iot.BucketsAggregationInput{})
	require.ErrorIs(t, err, iot.ErrValidation)
}
