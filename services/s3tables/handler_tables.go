package s3tables

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

func (h *Handler) handleGetTableMaintenanceJobStatus(
	ctx context.Context,
	r *http.Request,
	_ []byte,
) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	table, err := h.Backend.GetTable(bucketARN, splitNamespace(nsName), name)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: got table maintenance job status", keyName, name)

	// GetTableMaintenanceJobStatusOutput.Status is a map keyed by
	// maintenance type (types.TableMaintenanceJobStatusValue per entry,
	// confirmed via deserializeDocumentTableMaintenanceJobStatusValue in
	// aws-sdk-go-v2/service/s3tables's deserializers.go), one entry per
	// maintenance type actually configured on the table via
	// PutTableMaintenanceConfiguration. This in-memory backend runs no
	// background maintenance jobs, so every configured type is reported
	// with the "Not_Yet_Run" JobStatus enum value (types.JobStatusNotYetRun)
	// rather than a fabricated Successful/Failed outcome.
	jobStatus := make(map[string]any, len(table.MaintenanceConfiguration))
	for maintenanceType := range table.MaintenanceConfiguration {
		jobStatus[maintenanceType] = map[string]any{keyStatusField: maintenanceJobStatusNotYetRun}
	}

	return json.Marshal(map[string]any{
		keyTableARN:    table.ARN,
		keyStatusField: jobStatus,
	})
}

func (h *Handler) handleGetTableMetadataLocation(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	table, err := h.Backend.GetTable(bucketARN, splitNamespace(nsName), name)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: got table metadata location", keyName, name)

	return json.Marshal(map[string]any{
		keyVersionToken:     table.VersionToken,
		"warehouseLocation": table.WarehouseLocation,
		keyMetadataLocation: table.MetadataLocation,
	})
}

func (h *Handler) handleGetTableEncryption(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	ns := segs[2]
	name := segs[3]

	encCfg, err := h.Backend.GetTableEncryption(bucketARN, splitNamespace(ns), name)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: got table encryption", keyName, name)

	return json.Marshal(map[string]any{
		"encryptionConfiguration": encCfg,
	})
}

// createTableRequest is the request body for CreateTable.
// createTableRequest is the request body for CreateTable. Real
// CreateTableInput also accepts encryptionConfiguration,
// storageClassConfiguration, and tags alongside the required name/format --
// see CreateTableInput in aws-sdk-go-v2/service/s3tables.
type createTableRequest struct {
	EncryptionConfiguration   map[string]any    `json:"encryptionConfiguration"`
	StorageClassConfiguration map[string]any    `json:"storageClassConfiguration"`
	Tags                      map[string]string `json:"tags"`
	Name                      string            `json:"name"`
	Format                    string            `json:"format"`
}

func (h *Handler) handleCreateTable(ctx context.Context, r *http.Request, body []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 3 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN or namespace", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]

	var req createTableRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", errInvalidRequest)
	}

	if req.Format == "" {
		req.Format = "ICEBERG"
	}

	table, err := h.Backend.CreateTable(bucketARN, splitNamespace(nsName), req.Name, req.Format, CreateTableOptions{
		Encryption:   req.EncryptionConfiguration,
		StorageClass: storageClassFromConfig(req.StorageClassConfiguration),
		Tags:         req.Tags,
	})
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: created table", keyName, table.Name, keyArn, table.ARN)

	return json.Marshal(map[string]string{
		keyTableARN:     table.ARN,
		keyVersionToken: table.VersionToken,
	})
}

func (h *Handler) handleGetTable(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	q := r.URL.Query()
	bucketARN := q.Get(keyTableBucketARN)
	nsName := q.Get(keyNamespace)
	name := q.Get(keyName)
	tableArn := q.Get(keyTableArnLower)

	// Real GetTableInput accepts EITHER tableArn alone OR the
	// tableBucketARN+namespace+name triple (all four fields are optional on
	// the input shape) -- previously only the triple was honored, so a
	// client identifying the table purely by ARN always got a 400.
	var (
		table *Table
		err   error
	)

	switch {
	case tableArn != "":
		table, err = h.Backend.GetTableByARN(tableArn)
	case bucketARN != "" && nsName != "" && name != "":
		table, err = h.Backend.GetTable(bucketARN, splitNamespace(nsName), name)
	default:
		return nil, fmt.Errorf(
			"%w: either tableArn, or tableBucketARN, namespace and name, are required",
			errInvalidRequest,
		)
	}

	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: got table", keyName, table.Name, keyArn, table.ARN)

	// GetTableOutput has no tableBucketARN member -- its bucket-identifying
	// field is the system-assigned tableBucketId (confirmed via
	// awsRestjson1_deserializeOpDocumentGetTableOutput, gopherstack-wla0).
	return json.Marshal(map[string]any{
		keyName:             table.Name,
		keyNamespace:        table.Namespace,
		keyTableARN:         table.ARN,
		keyTableBucketID:    table.TableBucketID,
		"format":            table.Format,
		keyType:             bucketTypeCustomer,
		keyVersionToken:     table.VersionToken,
		keyMetadataLocation: table.MetadataLocation,
		"warehouseLocation": table.WarehouseLocation,
		keyCreatedAt:        table.CreatedAt.UTC().Format("2006-01-02T15:04:05.999Z"),
		"modifiedAt":        table.ModifiedAt.UTC().Format("2006-01-02T15:04:05.999Z"),
		keyCreatedBy:        table.OwnerAccountID,
		"modifiedBy":        table.OwnerAccountID,
		keyOwnerAccountID:   table.OwnerAccountID,
	})
}

func (h *Handler) handleDeleteTable(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	versionToken := r.URL.Query().Get(keyVersionToken)

	if err := h.Backend.DeleteTable(bucketARN, splitNamespace(nsName), name, versionToken); err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: deleted table", keyName, name, "bucket", bucketARN)

	return nil, nil
}

func (h *Handler) handleListTables(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 2 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN", errInvalidRequest)
	}

	bucketARN := segs[1]
	q := r.URL.Query()
	namespace := q.Get(keyNamespace)

	pg, err := h.Backend.ListTables(bucketARN, namespace, ListTablesParams{
		Prefix:            q.Get("prefix"),
		ContinuationToken: q.Get(keyContinuationToken),
		MaxTables:         queryInt(q, "maxTables"),
	})
	if err != nil {
		return nil, err
	}

	summaries := make([]map[string]any, 0, len(pg.Data))

	// TableSummary has no tableBucketARN member either -- same fix as
	// GetTable above (gopherstack-wla0).
	for _, t := range pg.Data {
		summaries = append(summaries, map[string]any{
			keyName:          t.Name,
			keyNamespace:     t.Namespace,
			keyTableARN:      t.ARN,
			keyTableBucketID: t.TableBucketID,
			keyType:          bucketTypeCustomer,
			keyCreatedAt:     t.CreatedAt.UTC().Format("2006-01-02T15:04:05.999Z"),
			"modifiedAt":     t.ModifiedAt.UTC().Format("2006-01-02T15:04:05.999Z"),
		})
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: listed tables", "bucket", bucketARN, "count", len(summaries))

	resp := map[string]any{
		"tables": summaries,
	}
	if pg.Next != "" {
		resp[keyContinuationToken] = pg.Next
	}

	return json.Marshal(resp)
}

// renameTableRequest is the request body for RenameTable.
type renameTableRequest struct {
	NewNamespaceName *string `json:"newNamespaceName"`
	NewName          *string `json:"newName"`
	VersionToken     *string `json:"versionToken"`
}

func (h *Handler) handleRenameTable(ctx context.Context, r *http.Request, body []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	var req renameTableRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	newNs := ""
	if req.NewNamespaceName != nil {
		newNs = *req.NewNamespaceName
	}

	newName := ""
	if req.NewName != nil {
		newName = *req.NewName
	}

	versionToken := ""
	if req.VersionToken != nil {
		versionToken = *req.VersionToken
	}

	if err := h.Backend.RenameTable(bucketARN, splitNamespace(nsName), name, newNs, newName, versionToken); err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: renamed table", "from", name, "to", newName)

	return nil, nil
}

// updateTableMetadataLocationRequest is the request body for UpdateTableMetadataLocation.
type updateTableMetadataLocationRequest struct {
	MetadataLocation string `json:"metadataLocation"`
	VersionToken     string `json:"versionToken"`
}

func (h *Handler) handleUpdateTableMetadataLocation(ctx context.Context, r *http.Request, body []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	var req updateTableMetadataLocationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.MetadataLocation == "" || req.VersionToken == "" {
		return nil, fmt.Errorf("%w: metadataLocation and versionToken are required", errInvalidRequest)
	}

	table, err := h.Backend.UpdateTableMetadataLocation(
		bucketARN,
		splitNamespace(nsName),
		name,
		req.MetadataLocation,
		req.VersionToken,
	)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: updated table metadata location", keyName, name)

	// UpdateTableMetadataLocationOutput has no tableBucketARN/tableBucketId
	// member at all (confirmed via
	// awsRestjson1_deserializeOpDocumentUpdateTableMetadataLocationOutput) --
	// the previous tableBucketARN key here was a harmless-but-invented extra,
	// not a wrong-key bug like GetTable/ListTables above; simply dropped
	// (gopherstack-wla0).
	return json.Marshal(map[string]any{
		keyName:             table.Name,
		keyTableARN:         table.ARN,
		keyNamespace:        table.Namespace,
		keyVersionToken:     table.VersionToken,
		keyMetadataLocation: table.MetadataLocation,
	})
}

func (h *Handler) handleGetTableMaintenanceConfiguration(
	ctx context.Context,
	r *http.Request,
	_ []byte,
) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	cfg, tableARN, err := h.Backend.GetTableMaintenanceConfiguration(bucketARN, splitNamespace(nsName), name)
	if err != nil {
		return nil, err
	}

	if cfg == nil {
		cfg = make(map[string]any)
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: got table maintenance configuration", keyName, name)

	return json.Marshal(map[string]any{
		keyTableARN:      tableARN,
		keyConfiguration: cfg,
	})
}

// putTableMaintenanceRequest is the request body for PutTableMaintenanceConfiguration.
type putTableMaintenanceRequest struct {
	Value map[string]any `json:"value"`
}

func (h *Handler) handlePutTableMaintenanceConfiguration(
	ctx context.Context,
	r *http.Request,
	body []byte,
) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 6 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace, name or type", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]
	maintenanceType := segs[5]

	var req putTableMaintenanceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if !validTableMaintenanceType(maintenanceType) {
		return nil, fmt.Errorf("%w: unsupported table maintenance type %q", errInvalidRequest, maintenanceType)
	}

	if req.Value == nil {
		return nil, fmt.Errorf("%w: value is required", errInvalidRequest)
	}

	if err := h.Backend.PutTableMaintenanceConfiguration(
		bucketARN,
		splitNamespace(nsName),
		name,
		maintenanceType,
		req.Value,
	); err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: put table maintenance configuration", keyName, name, "type", maintenanceType)

	return nil, nil
}

func validTableMaintenanceType(maintenanceType string) bool {
	return maintenanceType == maintenanceTypeIcebergCompaction ||
		maintenanceType == maintenanceTypeIcebergSnapshotManagement
}

func (h *Handler) handleGetTablePolicy(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	policy, err := h.Backend.GetTablePolicy(bucketARN, splitNamespace(nsName), name)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: got table policy", keyName, name)

	return json.Marshal(map[string]string{
		"resourcePolicy": policy,
	})
}

// putTablePolicyRequest is the request body for PutTablePolicy.
type putTablePolicyRequest struct {
	ResourcePolicy string `json:"resourcePolicy"`
}

func (h *Handler) handlePutTablePolicy(ctx context.Context, r *http.Request, body []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	var req putTablePolicyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := h.Backend.PutTablePolicy(bucketARN, splitNamespace(nsName), name, req.ResourcePolicy); err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: put table policy", keyName, name)

	return nil, nil
}

func (h *Handler) handleDeleteTablePolicy(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	if err := h.Backend.DeleteTablePolicy(bucketARN, splitNamespace(nsName), name); err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: deleted table policy", keyName, name)

	return nil, nil
}

func (h *Handler) handleGetTableStorageClass(ctx context.Context, r *http.Request, _ []byte) ([]byte, error) {
	segs := rawPathSegments(r)
	if len(segs) < 4 { //nolint:mnd // minimum required segments
		return nil, fmt.Errorf("%w: missing tableBucketARN, namespace or name", errInvalidRequest)
	}

	bucketARN := segs[1]
	nsName := segs[2]
	name := segs[3]

	sc, err := h.Backend.GetTableStorageClass(bucketARN, splitNamespace(nsName), name)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "s3tables: got table storage class", keyName, name)

	return json.Marshal(map[string]any{
		"storageClassConfiguration": map[string]any{
			"storageClass": sc,
		},
	})
}
