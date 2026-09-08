// Package dynamodb implements the AWS DynamoDB mock service.
// handler_import.go implements the wire-JSON handlers for
// DescribeImport/ImportTable/ListImports. Routing (dispatchExtraOps) stays
// in handler.go; these are the leaf implementations it calls into. Backend
// logic lives in import_export_s3.go.
package dynamodb

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkDDB "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/blackbirdworks/gopherstack/pkgs/ptrconv"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

type describeImportInput struct {
	ImportArn string `json:"ImportArn"`
}

type importTableS3BucketSourceWire struct {
	S3Bucket      string `json:"S3Bucket"`
	S3KeyPrefix   string `json:"S3KeyPrefix,omitempty"`
	S3BucketOwner string `json:"S3BucketOwner,omitempty"`
}

type importTableCsvOptionsWire struct {
	Delimiter  string   `json:"Delimiter,omitempty"`
	HeaderList []string `json:"HeaderList,omitempty"`
}

type importTableInputFormatOptionsWire struct {
	Csv *importTableCsvOptionsWire `json:"Csv,omitempty"`
}

// importTableDescriptionWire is a superset of the two real AWS wire shapes it
// serves: the full ImportTableDescription (Describe/ImportTable) and the
// narrower ImportSummary (ListImports, which has no TableId, ClientToken,
// item counts, or failure fields -- see importSummaryWireFromSDK, which
// deliberately leaves those unset for that path).
type importTableDescriptionWire struct {
	InputFormatOptions    *importTableInputFormatOptionsWire `json:"InputFormatOptions,omitempty"`
	S3BucketSource        *importTableS3BucketSourceWire     `json:"S3BucketSource,omitempty"`
	ImportArn             string                             `json:"ImportArn,omitempty"`
	ImportStatus          string                             `json:"ImportStatus,omitempty"`
	TableArn              string                             `json:"TableArn,omitempty"`
	TableID               string                             `json:"TableId,omitempty"`
	ClientToken           string                             `json:"ClientToken,omitempty"`
	CloudWatchLogGroupArn string                             `json:"CloudWatchLogGroupArn,omitempty"`
	InputFormat           string                             `json:"InputFormat,omitempty"`
	InputCompressionType  string                             `json:"InputCompressionType,omitempty"`
	FailureCode           string                             `json:"FailureCode,omitempty"`
	FailureMessage        string                             `json:"FailureMessage,omitempty"`
	StartTime             float64                            `json:"StartTime,omitempty"`
	EndTime               float64                            `json:"EndTime,omitempty"`
	ImportedItemCount     int64                              `json:"ImportedItemCount,omitempty"`
	ProcessedItemCount    int64                              `json:"ProcessedItemCount,omitempty"`
	ProcessedSizeBytes    int64                              `json:"ProcessedSizeBytes,omitempty"`
	ErrorCount            int64                              `json:"ErrorCount,omitempty"`
}

// importS3BucketSourceWireFromSDK converts the SDK S3BucketSource to the wire
// shape, or nil when absent (matching an omitted response member).
func importS3BucketSourceWireFromSDK(s *types.S3BucketSource) *importTableS3BucketSourceWire {
	if s == nil {
		return nil
	}

	return &importTableS3BucketSourceWire{
		S3Bucket:      ptrconv.String(s.S3Bucket),
		S3KeyPrefix:   ptrconv.String(s.S3KeyPrefix),
		S3BucketOwner: ptrconv.String(s.S3BucketOwner),
	}
}

// importDescriptionWireFromSDK maps the full SDK ImportTableDescription
// (Describe/ImportTable) to the wire shape.
func importDescriptionWireFromSDK(d *types.ImportTableDescription) importTableDescriptionWire {
	w := importTableDescriptionWire{}
	if d == nil {
		return w
	}
	w.ImportArn = ptrconv.String(d.ImportArn)
	w.ImportStatus = string(d.ImportStatus)
	w.TableArn = ptrconv.String(d.TableArn)
	w.TableID = ptrconv.String(d.TableId)
	w.ClientToken = ptrconv.String(d.ClientToken)
	w.CloudWatchLogGroupArn = ptrconv.String(d.CloudWatchLogGroupArn)
	w.InputFormat = string(d.InputFormat)
	w.InputCompressionType = string(d.InputCompressionType)
	w.FailureCode = ptrconv.String(d.FailureCode)
	w.FailureMessage = ptrconv.String(d.FailureMessage)
	w.ImportedItemCount = d.ImportedItemCount
	w.ProcessedItemCount = d.ProcessedItemCount
	w.ErrorCount = d.ErrorCount
	w.S3BucketSource = importS3BucketSourceWireFromSDK(d.S3BucketSource)
	if d.ProcessedSizeBytes != nil {
		w.ProcessedSizeBytes = *d.ProcessedSizeBytes
	}
	if d.StartTime != nil {
		w.StartTime = float64(d.StartTime.Unix())
	}
	if d.EndTime != nil {
		w.EndTime = float64(d.EndTime.Unix())
	}
	if d.InputFormatOptions != nil && d.InputFormatOptions.Csv != nil {
		w.InputFormatOptions = &importTableInputFormatOptionsWire{
			Csv: &importTableCsvOptionsWire{
				Delimiter:  ptrconv.String(d.InputFormatOptions.Csv.Delimiter),
				HeaderList: d.InputFormatOptions.Csv.HeaderList,
			},
		}
	}

	return w
}

// importSummaryWireFromSDK maps the narrower SDK ImportSummary (ListImports)
// to the wire shape. Deliberately does not set TableId, ClientToken, item
// counts, or failure fields -- ImportSummary carries none of them.
func importSummaryWireFromSDK(s types.ImportSummary) importTableDescriptionWire {
	w := importTableDescriptionWire{
		ImportArn:    ptrconv.String(s.ImportArn),
		ImportStatus: string(s.ImportStatus),
		TableArn:     ptrconv.String(s.TableArn),
		InputFormat:  string(s.InputFormat),

		S3BucketSource:        importS3BucketSourceWireFromSDK(s.S3BucketSource),
		CloudWatchLogGroupArn: ptrconv.String(s.CloudWatchLogGroupArn)}
	if s.StartTime != nil {
		w.StartTime = float64(s.StartTime.Unix())
	}
	if s.EndTime != nil {
		w.EndTime = float64(s.EndTime.Unix())
	}

	return w
}

type describeImportOutput struct {
	ImportTableDescription importTableDescriptionWire `json:"ImportTableDescription"`
}

func (h *DynamoDBHandler) handleDescribeImport(ctx context.Context, body []byte) (any, error) {
	var req describeImportInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	out, err := h.Backend.DescribeImport(ctx, &sdkDDB.DescribeImportInput{
		ImportArn: &req.ImportArn,
	})
	if err != nil {
		return nil, err
	}

	return &describeImportOutput{
		ImportTableDescription: importDescriptionWireFromSDK(out.ImportTableDescription),
	}, nil
}

// --- ImportTable handler ---

type importTableInput struct {
	InputFormatOptions      *importTableInputFormatOptionsWire `json:"InputFormatOptions,omitempty"`
	ClientToken             string                             `json:"ClientToken,omitempty"`
	S3BucketSource          importTableS3BucketSourceWire      `json:"S3BucketSource"`
	InputFormat             string                             `json:"InputFormat,omitempty"`
	InputCompressionType    string                             `json:"InputCompressionType,omitempty"`
	TableCreationParameters models.CreateTableInput            `json:"TableCreationParameters"`
}

type importTableOutput struct {
	ImportTableDescription importTableDescriptionWire `json:"ImportTableDescription"`
}

func (h *DynamoDBHandler) handleImportTable(ctx context.Context, body []byte) (any, error) {
	var req importTableInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	// Reuse the CreateTable conversion so KeySchema / AttributeDefinitions / GSIs /
	// throughput are all carried into the imported table.
	cti := models.ToSDKCreateTableInput(&req.TableCreationParameters)

	in := &sdkDDB.ImportTableInput{
		InputFormat:          types.InputFormat(req.InputFormat),
		InputCompressionType: types.InputCompressionType(req.InputCompressionType),
		S3BucketSource: &types.S3BucketSource{
			S3Bucket:      aws.String(req.S3BucketSource.S3Bucket),
			S3KeyPrefix:   aws.String(req.S3BucketSource.S3KeyPrefix),
			S3BucketOwner: aws.String(req.S3BucketSource.S3BucketOwner),
		},
		TableCreationParameters: &types.TableCreationParameters{
			TableName:              cti.TableName,
			KeySchema:              cti.KeySchema,
			AttributeDefinitions:   cti.AttributeDefinitions,
			BillingMode:            cti.BillingMode,
			GlobalSecondaryIndexes: cti.GlobalSecondaryIndexes,
			ProvisionedThroughput:  cti.ProvisionedThroughput,
		},
	}

	if req.ClientToken != "" {
		in.ClientToken = aws.String(req.ClientToken)
	}

	if req.InputFormatOptions != nil && req.InputFormatOptions.Csv != nil {
		in.InputFormatOptions = &types.InputFormatOptions{
			Csv: &types.CsvOptions{
				Delimiter:  aws.String(req.InputFormatOptions.Csv.Delimiter),
				HeaderList: req.InputFormatOptions.Csv.HeaderList,
			},
		}
	}

	out, err := h.Backend.ImportTable(ctx, in)
	if err != nil {
		return nil, err
	}

	return &importTableOutput{
		ImportTableDescription: importDescriptionWireFromSDK(out.ImportTableDescription),
	}, nil
}

// --- ListImports handler ---

type listImportsOutput struct {
	NextToken         string                       `json:"NextToken,omitempty"`
	ImportSummaryList []importTableDescriptionWire `json:"ImportSummaryList"`
}

type listImportsInput struct {
	TableArn  string `json:"TableArn,omitempty"`
	NextToken string `json:"NextToken,omitempty"`
	PageSize  int32  `json:"PageSize,omitempty"`
}

func (h *DynamoDBHandler) handleListImports(ctx context.Context, body []byte) (any, error) {
	var req listImportsInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	in := &sdkDDB.ListImportsInput{}
	if req.TableArn != "" {
		in.TableArn = &req.TableArn
	}
	if req.NextToken != "" {
		in.NextToken = &req.NextToken
	}
	if req.PageSize > 0 {
		in.PageSize = &req.PageSize
	}

	out, err := h.Backend.ListImports(ctx, in)
	if err != nil {
		return nil, err
	}

	summaries := make([]importTableDescriptionWire, 0, len(out.ImportSummaryList))
	for _, s := range out.ImportSummaryList {
		summaries = append(summaries, importSummaryWireFromSDK(s))
	}

	return &listImportsOutput{
		ImportSummaryList: summaries,
		NextToken:         ptrconv.String(out.NextToken),
	}, nil
}
