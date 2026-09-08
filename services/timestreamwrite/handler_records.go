package timestreamwrite

import (
	"context"
	"fmt"
	"sort"
)

// maxRecordsPerRequest is the maximum number of records accepted in a single WriteRecords call
// per the AWS API.
const maxRecordsPerRequest = 100

type writeRecordsInput struct {
	CommonAttributes *recordInput  `json:"CommonAttributes,omitempty"`
	DatabaseName     string        `json:"DatabaseName"`
	TableName        string        `json:"TableName"`
	Records          []recordInput `json:"Records"`
}

type measureValueInput struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
	Type  string `json:"Type"`
}

type recordInput struct {
	MeasureName      string              `json:"MeasureName"`
	MeasureValue     string              `json:"MeasureValue"`
	MeasureValueType string              `json:"MeasureValueType"`
	Time             string              `json:"Time"`
	TimeUnit         string              `json:"TimeUnit"`
	Dimensions       []dimensionInput    `json:"Dimensions"`
	MeasureValues    []measureValueInput `json:"MeasureValues"`
	Version          int64               `json:"Version"`
}

type dimensionInput struct {
	Name               string `json:"Name"`
	Value              string `json:"Value"`
	DimensionValueType string `json:"DimensionValueType,omitempty"`
}

type writeRecordsOutput struct {
	RecordsIngested struct {
		Total         int32 `json:"Total"`
		MemoryStore   int32 `json:"MemoryStore"`
		MagneticStore int32 `json:"MagneticStore"`
	} `json:"RecordsIngested"`
}

func (h *Handler) handleWriteRecords(
	_ context.Context,
	in *writeRecordsInput,
) (*writeRecordsOutput, error) {
	if in.DatabaseName == "" || in.TableName == "" {
		return nil, fmt.Errorf("%w: DatabaseName and TableName are required", errInvalidRequest)
	}

	// AWS API enforces a maximum of 100 records per WriteRecords request.
	if len(in.Records) > maxRecordsPerRequest {
		return nil, fmt.Errorf(
			"%w: WriteRecords accepts at most %d records per request, got %d",
			errInvalidRequest, maxRecordsPerRequest, len(in.Records),
		)
	}

	records := make([]Record, 0, len(in.Records))

	for i, r := range in.Records {
		if err := checkCommonAttributesDimensionOverlap(r, in.CommonAttributes, i); err != nil {
			return nil, err
		}

		merged := mergeRecordWithCommon(r, in.CommonAttributes)
		if err := validateRecord(merged, i); err != nil {
			return nil, err
		}

		records = append(records, recordInputToBackend(merged))
	}

	result, err := h.Backend.WriteRecords(in.DatabaseName, in.TableName, records)
	if err != nil {
		return nil, err
	}

	out := &writeRecordsOutput{}
	out.RecordsIngested.Total = result.Total
	out.RecordsIngested.MemoryStore = result.MemoryStore
	out.RecordsIngested.MagneticStore = result.MagneticStore

	return out, nil
}

// checkCommonAttributesDimensionOverlap rejects a record whose Dimensions share a
// name with CommonAttributes.Dimensions. Per WriteRecordsInput.CommonAttributes'
// doc comment (api_op_WriteRecords.go, timestreamwrite@v1.38.4): "Dimensions may
// not overlap, or a ValidationException will be thrown. In other words, a record
// must contain dimensions with unique names".
func checkCommonAttributesDimensionOverlap(r recordInput, common *recordInput, idx int) error {
	if common == nil || len(common.Dimensions) == 0 || len(r.Dimensions) == 0 {
		return nil
	}

	commonNames := make(map[string]struct{}, len(common.Dimensions))
	for _, d := range common.Dimensions {
		commonNames[d.Name] = struct{}{}
	}

	for _, d := range r.Dimensions {
		if _, ok := commonNames[d.Name]; ok {
			return fmt.Errorf(
				"%w: record[%d] dimension %q overlaps with a CommonAttributes dimension of the same name",
				errInvalidRequest, idx, d.Name,
			)
		}
	}

	return nil
}

// mergeRecordWithCommon merges CommonAttributes into a record per AWS semantics:
// record-specific values take priority; common fills in missing fields and dimensions.
func mergeRecordWithCommon(r recordInput, common *recordInput) recordInput {
	if common == nil {
		return r
	}

	merged := r

	// Merge dimensions: start with common, override with record-specific on name conflict.
	dimSet := make(map[string]dimensionInput, len(common.Dimensions)+len(r.Dimensions))
	for _, d := range common.Dimensions {
		dimSet[d.Name] = d
	}

	for _, d := range r.Dimensions {
		dimSet[d.Name] = d
	}

	if len(dimSet) > 0 {
		dims := make([]dimensionInput, 0, len(dimSet))
		for _, d := range dimSet {
			dims = append(dims, d)
		}

		sort.Slice(dims, func(i, j int) bool { return dims[i].Name < dims[j].Name })
		merged.Dimensions = dims
	}

	// Fill scalar fields from common when record does not set them.
	if merged.MeasureName == "" {
		merged.MeasureName = common.MeasureName
	}

	if merged.MeasureValue == "" {
		merged.MeasureValue = common.MeasureValue
	}

	if merged.MeasureValueType == "" {
		merged.MeasureValueType = common.MeasureValueType
	}

	if merged.Time == "" {
		merged.Time = common.Time
	}

	if merged.TimeUnit == "" {
		merged.TimeUnit = common.TimeUnit
	}

	if merged.Version == 0 {
		merged.Version = common.Version
	}

	if len(merged.MeasureValues) == 0 {
		merged.MeasureValues = common.MeasureValues
	}

	return merged
}

// recordInputToBackend converts a handler-level recordInput to the backend Record type.
func recordInputToBackend(r recordInput) Record {
	dims := make([]Dimension, 0, len(r.Dimensions))
	for _, d := range r.Dimensions {
		dims = append(dims, Dimension(d))
	}

	mvs := make([]MeasureValue, 0, len(r.MeasureValues))
	for _, mv := range r.MeasureValues {
		mvs = append(mvs, MeasureValue(mv))
	}

	return Record{
		Dimensions:       dims,
		MeasureName:      r.MeasureName,
		MeasureValue:     r.MeasureValue,
		MeasureValueType: r.MeasureValueType,
		Time:             r.Time,
		TimeUnit:         r.TimeUnit,
		MeasureValues:    mvs,
		Version:          r.Version,
	}
}
