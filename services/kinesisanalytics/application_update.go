package kinesisanalytics

import (
	"context"
	"fmt"
	"time"
)

// applyInputUpdates applies input update operations to the application.
func applyInputUpdates(app *Application, updates []inputUpdate) error {
	for _, iu := range updates {
		idx := findInputIndex(app.Inputs, iu.InputID)
		if idx < 0 {
			return fmt.Errorf("%w: InputId %q not found", ErrNotFound, iu.InputID)
		}

		if err := applyOneInputUpdate(&app.Inputs[idx], &iu); err != nil {
			return err
		}
	}

	return nil
}

// applyOneInputUpdate applies a single input update to an InputDescription.
func applyOneInputUpdate(inp *InputDescription, iu *inputUpdate) error {
	if iu.NamePrefixUpdate != "" {
		inp.NamePrefix = iu.NamePrefixUpdate
	}

	if iu.KinesisStreamsInputUpdate != nil {
		inp.KinesisStreamsInputDescription = &KinesisStreamsInputDesc{
			ResourceARN: iu.KinesisStreamsInputUpdate.ResourceARN,
			RoleARN:     iu.KinesisStreamsInputUpdate.RoleARN,
		}
		inp.KinesisFirehoseInputDescription = nil
	}

	if iu.KinesisFirehoseInputUpdate != nil {
		inp.KinesisFirehoseInputDescription = &KinesisFirehoseInputDesc{
			ResourceARN: iu.KinesisFirehoseInputUpdate.ResourceARN,
			RoleARN:     iu.KinesisFirehoseInputUpdate.RoleARN,
		}
		inp.KinesisStreamsInputDescription = nil
	}

	applyInputSchemaUpdate(inp, iu.InputSchemaUpdate)

	if iu.InputProcessingConfigurationUpdate != nil &&
		iu.InputProcessingConfigurationUpdate.InputLambdaProcessor != nil {
		inp.InputProcessingConfigurationDescription = &InputProcessingConfigurationDesc{
			InputLambdaProcessorDescription: &LambdaProcessorDesc{
				ResourceARN: iu.InputProcessingConfigurationUpdate.InputLambdaProcessor.ResourceARN,
				RoleARN:     iu.InputProcessingConfigurationUpdate.InputLambdaProcessor.RoleARN,
			},
		}
	}

	if iu.InputParallelismUpdate != nil {
		count := iu.InputParallelismUpdate.Count
		if count < minInputParallelism || count > maxInputParallelism {
			return fmt.Errorf("%w: InputParallelismUpdate.CountUpdate must be %d-%d",
				ErrValidation, minInputParallelism, maxInputParallelism)
		}

		inp.InputParallelism = &InputParallelism{Count: count}
	}

	if inp.InputParallelism != nil {
		inp.InAppStreamNames = inAppStreamNames(inp.NamePrefix, inp.InputParallelism.Count)
	}

	return nil
}

// applyInputSchemaUpdate merges an InputSchemaUpdate payload into an input's schema.
// Unlike ReferenceSchemaUpdate (a whole-object replace using the full SourceSchema shape),
// InputSchemaUpdate carries its own "Update"-suffixed sub-fields and AWS applies it as a
// partial patch: only the sub-fields the caller supplied are overwritten.
func applyInputSchemaUpdate(inp *InputDescription, update *inputSchemaUpdateInput) {
	if update == nil {
		return
	}

	if inp.InputSchema == nil {
		inp.InputSchema = &SourceSchema{}
	}

	if update.RecordFormat != nil {
		inp.InputSchema.RecordFormat = RecordFormat{
			RecordFormatType:  update.RecordFormat.RecordFormatType,
			MappingParameters: update.RecordFormat.MappingParameters,
		}
	}

	if update.RecordEncoding != "" {
		inp.InputSchema.RecordEncoding = update.RecordEncoding
	}

	if update.RecordColumns != nil {
		inp.InputSchema.RecordColumns = update.RecordColumns
	}
}

// applyOutputUpdates applies output update operations to the application.
func applyOutputUpdates(app *Application, updates []outputUpdate) error {
	for _, ou := range updates {
		idx := findOutputIndex(app.Outputs, ou.OutputID)
		if idx < 0 {
			return fmt.Errorf("%w: OutputId %q not found", ErrNotFound, ou.OutputID)
		}

		if err := applyOneOutputUpdate(&app.Outputs[idx], &ou); err != nil {
			return err
		}
	}

	return nil
}

// findOutputIndex returns the index of the output with the given ID, or -1 if not found.
func findOutputIndex(outputs []OutputDescription, outputID string) int {
	for i := range outputs {
		if outputs[i].OutputID == outputID {
			return i
		}
	}

	return -1
}

// applyOneOutputUpdate applies a single output update to an OutputDescription.
func applyOneOutputUpdate(out *OutputDescription, ou *outputUpdate) error {
	if ou.NameUpdate != "" {
		out.Name = ou.NameUpdate
	}

	if ou.KinesisStreamsOutputUpdate != nil {
		out.KinesisStreamsOutputDescription = &KinesisStreamsOutputDesc{
			ResourceARN: ou.KinesisStreamsOutputUpdate.ResourceARN,
			RoleARN:     ou.KinesisStreamsOutputUpdate.RoleARN,
		}
		out.KinesisFirehoseOutputDescription = nil
		out.LambdaOutputDescription = nil
	}

	if ou.KinesisFirehoseOutputUpdate != nil {
		out.KinesisFirehoseOutputDescription = &KinesisFirehoseOutputDesc{
			ResourceARN: ou.KinesisFirehoseOutputUpdate.ResourceARN,
			RoleARN:     ou.KinesisFirehoseOutputUpdate.RoleARN,
		}
		out.KinesisStreamsOutputDescription = nil
		out.LambdaOutputDescription = nil
	}

	if ou.LambdaOutputUpdate != nil {
		out.LambdaOutputDescription = &LambdaOutputDesc{
			ResourceARN: ou.LambdaOutputUpdate.ResourceARN,
			RoleARN:     ou.LambdaOutputUpdate.RoleARN,
		}
		out.KinesisStreamsOutputDescription = nil
		out.KinesisFirehoseOutputDescription = nil
	}

	if ou.DestinationSchemaUpdate != nil {
		ft := ou.DestinationSchemaUpdate.RecordFormatType
		if ft != recordFormatJSON && ft != "CSV" {
			return fmt.Errorf(
				"%w: DestinationSchema.RecordFormatType must be JSON or CSV",
				ErrValidation,
			)
		}

		out.DestinationSchema = &DestinationSchemaDesc{RecordFormatType: ft}
	}

	return nil
}

// applyReferenceDataSourceUpdates applies reference data source updates to the application.
func applyReferenceDataSourceUpdates(
	app *Application,
	updates []referenceDataSourceUpdate,
) error {
	for _, ru := range updates {
		idx := findReferenceIndex(app.ReferenceDataSources, ru.ReferenceID)
		if idx < 0 {
			return fmt.Errorf("%w: ReferenceId %q not found", ErrNotFound, ru.ReferenceID)
		}

		ref := &app.ReferenceDataSources[idx]

		if ru.TableNameUpdate != "" {
			ref.TableName = ru.TableNameUpdate
		}

		if ru.S3ReferenceDataSourceUpdate != nil {
			ref.S3ReferenceDataSourceDescription = &S3ReferenceDataSourceDesc{
				BucketARN:        ru.S3ReferenceDataSourceUpdate.BucketARN,
				FileKey:          ru.S3ReferenceDataSourceUpdate.FileKey,
				ReferenceRoleARN: ru.S3ReferenceDataSourceUpdate.ReferenceRoleARN,
			}
		}

		if ru.ReferenceSchemaUpdate != nil {
			schema, err := convertSourceSchema(ru.ReferenceSchemaUpdate)
			if err != nil {
				return err
			}

			ref.ReferenceSchema = &schema
		}
	}

	return nil
}

// findReferenceIndex returns the index of the reference with the given ID, or -1.
func findReferenceIndex(refs []ReferenceDataSourceDescription, refID string) int {
	for i := range refs {
		if refs[i].ReferenceID == refID {
			return i
		}
	}

	return -1
}

// applyCWLOptionUpdates applies CloudWatch logging option updates to the application.
func applyCWLOptionUpdates(
	app *Application,
	updates []cwlOptionUpdate,
) error {
	for _, cu := range updates {
		idx := findCWLOptionIndex(
			app.CloudWatchLoggingOptions,
			cu.CloudWatchLoggingOptionID,
		)
		if idx < 0 {
			return fmt.Errorf(
				"%w: CloudWatchLoggingOptionId %q not found",
				ErrNotFound, cu.CloudWatchLoggingOptionID,
			)
		}

		opt := &app.CloudWatchLoggingOptions[idx]

		if cu.LogStreamARNUpdate != "" {
			opt.LogStreamARN = cu.LogStreamARNUpdate
		}

		if cu.RoleARNUpdate != "" {
			opt.RoleARN = cu.RoleARNUpdate
		}
	}

	return nil
}

// findCWLOptionIndex returns the index of the CWL option with the given ID, or -1.
func findCWLOptionIndex(opts []CloudWatchLoggingOptionDesc, optID string) int {
	for i := range opts {
		if opts[i].CloudWatchLoggingOptionID == optID {
			return i
		}
	}

	return -1
}

// applyUpdate applies the full application update payload. Must be called under b.mu.
func applyUpdate(app *Application, update *applicationUpdate) error {
	if update == nil {
		return nil
	}

	if update.ApplicationCodeUpdate != "" {
		// AWS docs (kinesisanalytics limits page): "The SQL code in an application is
		// limited to 100 KB" -- CreateApplication enforces this via validateApplicationCode
		// but UpdateApplication previously let ApplicationCodeUpdate bypass it entirely.
		if err := validateApplicationCode(update.ApplicationCodeUpdate); err != nil {
			return err
		}

		app.ApplicationCode = update.ApplicationCodeUpdate
	}

	if err := applyInputUpdates(app, update.InputUpdates); err != nil {
		return err
	}

	if err := applyOutputUpdates(app, update.OutputUpdates); err != nil {
		return err
	}

	if err := applyReferenceDataSourceUpdates(app, update.ReferenceDataSourceUpdates); err != nil {
		return err
	}

	return applyCWLOptionUpdates(app, update.CloudWatchLoggingOptionUpdates)
}

// UpdateApplication updates the application with the full update payload and bumps the version.
func (b *InMemoryBackend) UpdateApplication(
	ctx context.Context,
	name string,
	currentVersionID int64,
	update *applicationUpdate,
) (*Application, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("UpdateApplication")
	defer b.mu.Unlock()

	app, exists := b.apps.Get(applicationKey(region, name))
	if !exists {
		return nil, ErrNotFound
	}

	if app.ApplicationStatus != statusReady && app.ApplicationStatus != statusRunning {
		return nil, fmt.Errorf(
			"%w: application must be in READY or RUNNING state to update (current: %s)",
			ErrResourceInUse, app.ApplicationStatus,
		)
	}

	if app.ApplicationVersionID != currentVersionID {
		return nil, ErrConcurrentUpdate
	}

	if err := applyUpdate(app, update); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	app.ApplicationVersionID++
	app.LastUpdateTimestamp = &now

	return appCopy(app), nil
}
