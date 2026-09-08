package cloudformation

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
)

// typeSchemaFor returns the primary identifier property name for a given CloudFormation resource type.
// The primary identifier is used by Terraform's AWS provider to construct resource IDs.
// Falls back to "Id" for unknown types.
func typeSchemaFor(typeName string) string {
	primaryIdentifiers := map[string]string{
		resTypeLogGroup:       "LogGroupName",
		resTypeS3Bucket:       "BucketName",
		resTypeLambdaFunction: "FunctionName",
		resTypeSNSTopic:       "TopicArn",
		resTypeSQSQueue:       "QueueUrl",
		resTypeDynamoDBTable:  "TableName",
		resTypeIAMRole:        "RoleName",
		resTypeEC2VPC:         "VpcId",
		"AWS::EC2::Instance":  "InstanceId",
		resTypeRDSDB:          "DBInstanceIdentifier",
		resTypeECSCluster:     "ClusterArn",
		resTypeKMSKey:         "KeyId",
		resTypeSecret:         "Id",
	}

	if prop, ok := primaryIdentifiers[typeName]; ok {
		return prop
	}

	return "Id"
}

// handleDescribeType returns CloudFormation type information.
// It first checks the backend type registry for registered types, then falls back
// to a schema-based response for built-in AWS types.
func (h *Handler) handleDescribeType(form url.Values, c *echo.Context) error {
	// Try the backend registry first (for types registered via RegisterType).
	if handled, err := h.describeTypeFromRegistry(form, c); handled {
		return err
	}

	typeName := form.Get("TypeName")
	if typeName == "" {
		return h.xmlError(c, "CFNRegistryException", "TypeName is required")
	}

	schemaJSON, err := describeTypeSchemaJSON(typeName)
	if err != nil {
		return h.xmlError(c, "InternalFailure", "failed to marshal schema: "+err.Error())
	}

	typeResource := fmt.Sprintf("type/resource/%s/00000001", strings.ReplaceAll(typeName, "::", "-"))
	arnStr := arn.Build("cloudformation", config.DefaultRegion, "", typeResource)

	type typeResult struct {
		Arn              string `xml:"Arn"`
		DefaultVersionID string `xml:"DefaultVersionId"`
		Schema           string `xml:"Schema"`
		Type             string `xml:"Type"`
		TypeName         string `xml:"TypeName"`
		Visibility       string `xml:"Visibility"`
		ProvisioningType string `xml:"ProvisioningType"`
		IsActivated      bool   `xml:"IsActivated"`
		IsDefaultVersion bool   `xml:"IsDefaultVersion"`
	}
	type response struct {
		XMLName   xml.Name   `xml:"DescribeTypeResponse"`
		Xmlns     string     `xml:"xmlns,attr"`
		RequestID string     `xml:"ResponseMetadata>RequestId"`
		Result    typeResult `xml:"DescribeTypeResult"`
	}

	return writeXML(c, response{
		Xmlns:     cfnNS,
		RequestID: uuid.New().String(),
		Result: typeResult{
			Arn:              arnStr,
			DefaultVersionID: "00000001",
			IsActivated:      true,
			IsDefaultVersion: true,
			Schema:           schemaJSON,
			Type:             typeKindResource,
			TypeName:         typeName,
			Visibility:       typeVisibilityPublic,
			ProvisioningType: provisioningTypeFullyMutable,
		},
	})
}

// describeTypeSchemaJSON returns the CloudFormation Resource Provider Schema JSON
// document for typeName. If typeName is present in the modeled resource type
// catalog (resourceTypeSchemaCatalog), that realistic schema is returned verbatim
// (re-marshaled through a generic map to guarantee it is valid JSON). Otherwise a
// minimal but valid fallback schema is generated using only the primary
// identifier property name, so unmodeled/unknown types still get a usable
// response instead of an error.
func describeTypeSchemaJSON(typeName string) (string, error) {
	if raw, ok := resourceTypeSchema(typeName); ok {
		// Round-trip through a generic map to normalize formatting and to fail
		// loudly (via the error return) if a catalog entry is ever malformed.
		var generic map[string]any
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			return "", err
		}

		normalized, err := json.Marshal(generic)
		if err != nil {
			return "", err
		}

		return string(normalized), nil
	}

	// Fallback: minimal schema for types not in the modeled catalog.
	primaryProp := typeSchemaFor(typeName)

	type cfnSchema struct {
		Properties           map[string]struct{} `json:"properties"`
		TypeName             string              `json:"typeName"`
		Description          string              `json:"description"`
		PrimaryIdentifier    []string            `json:"primaryIdentifier"`
		AdditionalProperties bool                `json:"additionalProperties"`
	}

	schemaObj := cfnSchema{
		TypeName:             typeName,
		Description:          "Schema for " + typeName,
		PrimaryIdentifier:    []string{"/properties/" + primaryProp},
		Properties:           map[string]struct{}{primaryProp: {}},
		AdditionalProperties: true,
	}

	schemaBytes, err := json.Marshal(schemaObj)
	if err != nil {
		return "", err
	}

	return string(schemaBytes), nil
}

// dispatchTypeOps handles CloudFormation type/extension registration and management.
func (h *Handler) dispatchTypeOps(action string, form url.Values, c *echo.Context) (bool, error) {
	if handled, err := h.dispatchTypeRegistrationOps(action, form, c); handled {
		return true, err
	}

	return h.dispatchTypeManagementOps(action, form, c)
}

// dispatchTypeRegistrationOps handles type registration and activation operations.
func (h *Handler) dispatchTypeRegistrationOps(
	action string,
	form url.Values,
	c *echo.Context,
) (bool, error) {
	switch action {
	case "ActivateType":
		return true, h.handleActivateType(form, c)
	case "DeactivateType":
		return true, h.handleDeactivateType(form, c)
	case "RegisterType":
		return true, h.handleRegisterType(form, c)
	case "DeregisterType":
		return true, h.handleDeregisterType(form, c)
	case "PublishType":
		return true, h.handlePublishType(form, c)
	case "SetTypeDefaultVersion":
		return true, h.handleSetTypeDefaultVersion(form, c)
	case "SetTypeConfiguration":
		return true, h.handleSetTypeConfiguration(form, c)
	case "BatchDescribeTypeConfigurations":
		return true, h.handleBatchDescribeTypeConfigurations(form, c)
	case "TestType":
		return true, h.handleTestType(form, c)
	}

	return false, nil
}

// dispatchTypeManagementOps handles type listing, publishing, and access management.
func (h *Handler) dispatchTypeManagementOps(
	action string,
	form url.Values,
	c *echo.Context,
) (bool, error) {
	switch action {
	case "ListTypes":
		return true, h.handleListTypes(form, c)
	case "ListTypeVersions":
		return true, h.handleListTypeVersions(form, c)
	case "ListTypeRegistrations":
		return true, h.handleListTypeRegistrations(form, c)
	case "DescribeTypeRegistration":
		return true, h.handleDescribeTypeRegistration(form, c)
	case "RegisterPublisher":
		return true, h.handleRegisterPublisher(form, c)
	case "DescribePublisher":
		return true, h.handleDescribePublisher(form, c)
	}

	return false, nil
}

func (h *Handler) handleActivateType(form url.Values, c *echo.Context) error {
	// ActivateTypeInput has no "TypeArn" member; the ARN identifier is
	// "PublicTypeArn" (cloudformation@v1.76.1 serializers.go:7181).
	arn, err := h.Backend.ActivateType(form.Get("TypeName"), form.Get("PublicTypeArn"))
	if err != nil {
		return h.xmlError(c, "TypeNotFoundException", err.Error())
	}
	type result struct {
		Arn string `xml:"Arn"`
	}
	type response struct {
		XMLName   xml.Name `xml:"ActivateTypeResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"ActivateTypeResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cfnNS, Result: result{Arn: arn}, RequestID: uuid.New().String()})
}

func (h *Handler) handleDeactivateType(form url.Values, c *echo.Context) error {
	// DeactivateTypeInput sends "Arn", not "TypeArn"
	// (cloudformation@v1.76.1 serializers.go:7751).
	if err := h.Backend.DeactivateType(form.Get("TypeName"), form.Get("Arn")); err != nil {
		return h.xmlError(c, "TypeNotFoundException", err.Error())
	}
	type result struct{}
	type response struct {
		XMLName   xml.Name `xml:"DeactivateTypeResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"DeactivateTypeResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cfnNS, RequestID: uuid.New().String()})
}

func (h *Handler) handleRegisterType(form url.Values, c *echo.Context) error {
	token, err := h.Backend.RegisterType(form.Get("TypeName"), form.Get("SchemaHandlerPackage"))
	if err != nil {
		return h.xmlError(c, "ValidationError", err.Error())
	}
	type result struct {
		RegistrationToken string `xml:"RegistrationToken"`
	}
	type response struct {
		XMLName   xml.Name `xml:"RegisterTypeResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"RegisterTypeResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(
		c,
		response{
			Xmlns:     cfnNS,
			Result:    result{RegistrationToken: token},
			RequestID: uuid.New().String(),
		},
	)
}

func (h *Handler) handleDeregisterType(form url.Values, c *echo.Context) error {
	err := h.Backend.DeregisterType(form.Get("TypeName"), form.Get("Arn"), form.Get("VersionId"))
	if err != nil {
		if errors.Is(err, ErrCannotDeregisterDefaultVersion) {
			return h.xmlError(c, "CFNRegistryException", err.Error())
		}

		return h.xmlError(c, "TypeNotFoundException", err.Error())
	}
	type result struct{}
	type response struct {
		XMLName   xml.Name `xml:"DeregisterTypeResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"DeregisterTypeResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cfnNS, RequestID: uuid.New().String()})
}

func (h *Handler) handlePublishType(form url.Values, c *echo.Context) error {
	publicTypeArn, err := h.Backend.PublishType(form.Get("TypeName"))
	if err != nil {
		return h.xmlError(c, "TypeNotFoundException", err.Error())
	}
	type result struct {
		PublicTypeArn string `xml:"PublicTypeArn"`
	}
	type response struct {
		XMLName   xml.Name `xml:"PublishTypeResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"PublishTypeResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(
		c,
		response{Xmlns: cfnNS, Result: result{PublicTypeArn: publicTypeArn}, RequestID: uuid.New().String()},
	)
}

func (h *Handler) handleSetTypeDefaultVersion(form url.Values, c *echo.Context) error {
	if err := h.Backend.SetTypeDefaultVersion(form.Get("Arn"), form.Get("VersionId")); err != nil {
		return h.xmlError(c, "TypeNotFoundException", err.Error())
	}
	type result struct{}
	type response struct {
		XMLName   xml.Name `xml:"SetTypeDefaultVersionResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"SetTypeDefaultVersionResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cfnNS, RequestID: uuid.New().String()})
}

func (h *Handler) handleSetTypeConfiguration(form url.Values, c *echo.Context) error {
	configArn, err := h.Backend.SetTypeConfiguration(form.Get("TypeName"), form.Get("Configuration"))
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	type result struct {
		ConfigurationArn string `xml:"ConfigurationArn"`
	}
	type response struct {
		XMLName   xml.Name `xml:"SetTypeConfigurationResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"SetTypeConfigurationResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(
		c,
		response{Xmlns: cfnNS, Result: result{ConfigurationArn: configArn}, RequestID: uuid.New().String()},
	)
}

// parseTypeConfigurationIdentifiers parses the TypeConfigurationIdentifiers
// list of structs — NOT scalars (verified against serializers.go:7114, each
// member has Type/TypeArn/TypeConfigurationAlias/TypeConfigurationArn/TypeName).
func parseTypeConfigurationIdentifiers(form url.Values) []TypeConfigurationIdentifier {
	var result []TypeConfigurationIdentifier
	for i := 1; ; i++ {
		p := fmt.Sprintf("TypeConfigurationIdentifiers.member.%d.", i)
		typeName := form.Get(p + "TypeName")
		typeArn := form.Get(p + "TypeArn")
		alias := form.Get(p + "TypeConfigurationAlias")
		cfgArn := form.Get(p + "TypeConfigurationArn")
		typ := form.Get(p + "Type")
		if typeName == "" && typeArn == "" && alias == "" && cfgArn == "" && typ == "" {
			return result
		}
		result = append(result, TypeConfigurationIdentifier{
			Type:                   typ,
			TypeArn:                typeArn,
			TypeConfigurationAlias: alias,
			TypeConfigurationArn:   cfgArn,
			TypeName:               typeName,
		})
	}
}

func (h *Handler) handleBatchDescribeTypeConfigurations(form url.Values, c *echo.Context) error {
	identifiers := parseTypeConfigurationIdentifiers(form)
	details, errs, unprocessed := h.Backend.BatchDescribeTypeConfigurations(identifiers)
	type result struct {
		Errors             []BatchDescribeTypeConfigurationsError `xml:"Errors>member,omitempty"`
		TypeConfigurations []TypeConfigurationDetail              `xml:"TypeConfigurations>member,omitempty"`

		UnprocessedTypeConfigurations []TypeConfigurationIdentifier `xml:"UnprocessedTypeConfigurations>member,omitempty"`
	}
	type response struct {
		XMLName   xml.Name `xml:"BatchDescribeTypeConfigurationsResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		Result    result   `xml:"BatchDescribeTypeConfigurationsResult"`
	}

	return writeXML(
		c,
		response{
			Xmlns: cfnNS,
			Result: result{
				Errors:                        errs,
				TypeConfigurations:            details,
				UnprocessedTypeConfigurations: unprocessed,
			},
			RequestID: uuid.New().String(),
		},
	)
}

func (h *Handler) handleListTypes(_ url.Values, c *echo.Context) error {
	types, err := h.Backend.ListTypes("")
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	type typeXML struct {
		TypeName         string `xml:"TypeName,omitempty"`
		TypeArn          string `xml:"TypeArn,omitempty"`
		Type             string `xml:"Type,omitempty"`
		DefaultVersionID string `xml:"DefaultVersionId,omitempty"`
		IsActivated      bool   `xml:"IsActivated,omitempty"`
	}
	members := make([]typeXML, 0, len(types))
	for _, t := range types {
		members = append(members, typeXML{
			TypeName:         t.TypeName,
			TypeArn:          t.TypeArn,
			Type:             t.Type,
			DefaultVersionID: t.DefaultVersionID,
			IsActivated:      t.IsActivated,
		})
	}
	type result struct {
		TypeSummaries []typeXML `xml:"TypeSummaries>member"`
	}
	type response struct {
		XMLName   xml.Name `xml:"ListTypesResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		Result    result   `xml:"ListTypesResult"`
	}

	return writeXML(
		c,
		response{
			Xmlns:     cfnNS,
			Result:    result{TypeSummaries: members},
			RequestID: uuid.New().String(),
		},
	)
}

func (h *Handler) handleListTypeVersions(form url.Values, c *echo.Context) error {
	versionIDs, err := h.Backend.ListTypeVersions(form.Get("TypeName"), form.Get("DeprecatedStatus"))
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	// Real TypeVersionSummary's ARN member is "Arn", not "TypeArn"
	// (cloudformation@v1.76.1 types/types.go:3578).
	type versionXML struct {
		Arn       string `xml:"Arn,omitempty"`
		VersionID string `xml:"VersionId,omitempty"`
	}
	members := make([]versionXML, 0, len(versionIDs))
	typeArn := "arn:aws:cloudformation:::type/resource/" + form.Get("TypeName")
	for _, v := range versionIDs {
		members = append(members, versionXML{Arn: typeArn, VersionID: v})
	}
	type result struct {
		TypeVersionSummaries []versionXML `xml:"TypeVersionSummaries>member"`
	}
	type response struct {
		XMLName   xml.Name `xml:"ListTypeVersionsResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		Result    result   `xml:"ListTypeVersionsResult"`
	}

	return writeXML(
		c,
		response{
			Xmlns:     cfnNS,
			Result:    result{TypeVersionSummaries: members},
			RequestID: uuid.New().String(),
		},
	)
}

func (h *Handler) handleListTypeRegistrations(form url.Values, c *echo.Context) error {
	tokens, err := h.Backend.ListTypeRegistrations(form.Get("TypeName"), form.Get("Type"))
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	type result struct {
		RegistrationTokenList []string `xml:"RegistrationTokenList>member"`
	}
	type response struct {
		XMLName   xml.Name `xml:"ListTypeRegistrationsResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		Result    result   `xml:"ListTypeRegistrationsResult"`
	}

	return writeXML(
		c,
		response{
			Xmlns:     cfnNS,
			Result:    result{RegistrationTokenList: tokens},
			RequestID: uuid.New().String(),
		},
	)
}

func (h *Handler) handleDescribeTypeRegistration(form url.Values, c *echo.Context) error {
	status, err := h.Backend.DescribeTypeRegistration(form.Get("RegistrationToken"))
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	type result struct {
		ProgressStatus string `xml:"ProgressStatus"`
	}
	type response struct {
		XMLName   xml.Name `xml:"DescribeTypeRegistrationResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"DescribeTypeRegistrationResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(
		c,
		response{
			Xmlns:     cfnNS,
			Result:    result{ProgressStatus: status},
			RequestID: uuid.New().String(),
		},
	)
}

func (h *Handler) handleTestType(form url.Values, c *echo.Context) error {
	token, err := h.Backend.TestType(form.Get("TypeName"), form.Get("Arn"))
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	type result struct {
		TypeVersionArn string `xml:"TypeVersionArn,omitempty"`
	}
	type response struct {
		XMLName   xml.Name `xml:"TestTypeResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"TestTypeResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(
		c,
		response{
			Xmlns:     cfnNS,
			Result:    result{TypeVersionArn: token},
			RequestID: uuid.New().String(),
		},
	)
}

func (h *Handler) handleRegisterPublisher(form url.Values, c *echo.Context) error {
	id, err := h.Backend.RegisterPublisher(form.Get("ConnectionArn"))
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	type result struct {
		PublisherID string `xml:"PublisherId"`
	}
	type response struct {
		XMLName   xml.Name `xml:"RegisterPublisherResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"RegisterPublisherResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(
		c,
		response{Xmlns: cfnNS, Result: result{PublisherID: id}, RequestID: uuid.New().String()},
	)
}

func (h *Handler) handleDescribePublisher(form url.Values, c *echo.Context) error {
	publisherID := form.Get("PublisherId")
	status, err := h.Backend.DescribePublisher(publisherID)
	if err != nil {
		return h.xmlError(c, "CFNRegistryException", err.Error())
	}
	type result struct {
		PublisherID     string `xml:"PublisherId,omitempty"`
		PublisherStatus string `xml:"PublisherStatus"`
	}
	type response struct {
		XMLName   xml.Name `xml:"DescribePublisherResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		Result    result   `xml:"DescribePublisherResult"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(
		c,
		response{
			Xmlns:     cfnNS,
			Result:    result{PublisherID: publisherID, PublisherStatus: status},
			RequestID: uuid.New().String(),
		},
	)
}

// describeTypeFromRegistry returns a DescribeType XML response from the backend registry.
// Returns (true, nil) if the type was found in the registry, (false, nil) if not found,
// or (true, err) if an error occurred during XML serialization.
func (h *Handler) describeTypeFromRegistry(form url.Values, c *echo.Context) (bool, error) {
	details, err := h.Backend.DescribeType(form.Get("TypeName"), form.Get("Arn"), form.Get("VersionId"))
	if err == nil {
		type typeDetailXML struct {
			TypeName         string `xml:"TypeName,omitempty"`
			TypeArn          string `xml:"Arn,omitempty"`
			Type             string `xml:"Type,omitempty"`
			Visibility       string `xml:"Visibility,omitempty"`
			Status           string `xml:"TypeVersionStatus,omitempty"`
			Description      string `xml:"Description,omitempty"`
			Schema           string `xml:"Schema,omitempty"`
			VersionID        string `xml:"VersionId,omitempty"`
			DefaultVersionID string `xml:"DefaultVersionId,omitempty"`
			DeprecatedStatus string `xml:"DeprecatedStatus,omitempty"`
			IsActivated      bool   `xml:"IsActivated"`
			IsDefaultVersion bool   `xml:"IsDefaultVersion"`
		}
		type response struct {
			XMLName   xml.Name      `xml:"DescribeTypeResponse"`
			Xmlns     string        `xml:"xmlns,attr"`
			RequestID string        `xml:"ResponseMetadata>RequestId"`
			Result    typeDetailXML `xml:"DescribeTypeResult"`
		}

		return true, writeXML(c, response{
			Xmlns: cfnNS,
			Result: typeDetailXML{
				TypeName:         details.TypeName,
				TypeArn:          details.TypeArn,
				Type:             details.Type,
				Visibility:       details.Visibility,
				Status:           details.Status,
				Description:      details.Description,
				Schema:           details.Schema,
				VersionID:        details.VersionID,
				DefaultVersionID: details.DefaultVersionID,
				IsActivated:      details.IsActivated,
				IsDefaultVersion: details.IsDefaultVersion,
				DeprecatedStatus: details.DeprecatedStatus,
			},
			RequestID: uuid.New().String(),
		})
	}

	return false, nil // not in registry — fall through to schema-based handler
}
