package s3control

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

// ObjectLambdaConfigSink receives the Lambda ARN configured for an Object
// Lambda access point's underlying bucket, resolved from
// PutAccessPointConfigurationForObjectLambda's SupportingAccessPoint, so
// GetObject through that bucket actually invokes the Lambda transform
// instead of the config being accepted and never reaching S3 (gopherstack-6o0r).
type ObjectLambdaConfigSink interface {
	SetObjectLambdaConfig(bucket, lambdaARN string)
}

// SetObjectLambdaConfigSink wires the S3 backend that should receive
// resolved Object Lambda configuration.
func (b *InMemoryBackend) SetObjectLambdaConfigSink(sink ObjectLambdaConfigSink) {
	b.mu.Lock("SetObjectLambdaConfigSink")
	defer b.mu.Unlock()

	b.objectLambdaSink = sink
}

// CreateAccessPointForObjectLambda creates an Object Lambda access point.
func (b *InMemoryBackend) CreateAccessPointForObjectLambda(accountID, name string) *ObjectLambdaAccessPoint {
	b.mu.Lock("CreateAccessPointForObjectLambda")
	defer b.mu.Unlock()

	arn := fmt.Sprintf(arnFmtObjectLambda, b.region, accountID, name)

	const (
		maxAliasLen = 63
		aliasSuffix = "--ol-s3"
	)

	aliasPrefix := accountID
	if len(aliasPrefix) > aliasAccountIDMaxLen {
		aliasPrefix = aliasPrefix[:aliasAccountIDMaxLen]
	}

	alias := fmt.Sprintf("%s-%s%s", name, aliasPrefix, aliasSuffix)
	if len(alias) > maxAliasLen {
		alias = alias[:maxAliasLen]
	}

	ap := &ObjectLambdaAccessPoint{
		AccountID:                  accountID,
		Name:                       name,
		ObjectLambdaAccessPointArn: arn,
		Alias: &ObjectLambdaAccessPointAlias{
			Value:  alias,
			Status: "READY",
		},
	}
	b.objectLambdaAccessPoints.Put(ap)

	return cloneObjectLambdaAccessPoint(ap)
}

// ---- Object Lambda Access Points ----

// GetAccessPointForObjectLambda returns an Object Lambda access point.
func (b *InMemoryBackend) GetAccessPointForObjectLambda(
	accountID, name string,
) (*ObjectLambdaAccessPoint, error) {
	b.mu.RLock("GetAccessPointForObjectLambda")
	defer b.mu.RUnlock()

	key := accountID + ":" + name
	ap, ok := b.objectLambdaAccessPoints.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errObjectLambdaAPNotFound, name)
	}

	return cloneObjectLambdaAccessPoint(ap), nil
}

// DeleteAccessPointForObjectLambda removes an Object Lambda access point and
// cascade-cleans its policy, configuration, and generic resource tags so a
// delete/recreate cycle under the same name never resurfaces stale state.
func (b *InMemoryBackend) DeleteAccessPointForObjectLambda(accountID, name string) error {
	b.mu.Lock("DeleteAccessPointForObjectLambda")
	defer b.mu.Unlock()

	key := accountID + ":" + name

	ap, ok := b.objectLambdaAccessPoints.Get(key)
	if !ok {
		return fmt.Errorf("%w: %s", errObjectLambdaAPNotFound, name)
	}

	arn := ap.ObjectLambdaAccessPointArn

	b.objectLambdaAccessPoints.Delete(key)
	delete(b.objectLambdaAPPolicies, key)
	delete(b.objectLambdaAPConfigs, key)
	delete(b.resourceTags, arn)

	return nil
}

// ListAccessPointsForObjectLambda lists Object Lambda access points for an account.
func (b *InMemoryBackend) ListAccessPointsForObjectLambda(
	accountID string,
) []*ObjectLambdaAccessPoint {
	b.mu.RLock("ListAccessPointsForObjectLambda")
	defer b.mu.RUnlock()

	var out []*ObjectLambdaAccessPoint
	for _, ap := range b.objectLambdaAccessPoints.All() {
		if ap.AccountID == accountID {
			out = append(out, cloneObjectLambdaAccessPoint(ap))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// GetAccessPointPolicyForObjectLambda returns the policy for an Object Lambda AP.
func (b *InMemoryBackend) GetAccessPointPolicyForObjectLambda(
	accountID, name string,
) (string, error) {
	b.mu.RLock("GetAccessPointPolicyForObjectLambda")
	defer b.mu.RUnlock()

	key := accountID + ":" + name
	if !b.objectLambdaAccessPoints.Has(key) {
		return "", fmt.Errorf("%w: %s", errObjectLambdaAPNotFound, name)
	}

	return b.objectLambdaAPPolicies[key], nil
}

// PutAccessPointPolicyForObjectLambda sets the policy for an Object Lambda AP.
func (b *InMemoryBackend) PutAccessPointPolicyForObjectLambda(
	accountID, name, policy string,
) error {
	b.mu.Lock("PutAccessPointPolicyForObjectLambda")
	defer b.mu.Unlock()

	key := accountID + ":" + name
	if !b.objectLambdaAccessPoints.Has(key) {
		return fmt.Errorf("%w: %s", errObjectLambdaAPNotFound, name)
	}
	b.objectLambdaAPPolicies[key] = policy

	return nil
}

// DeleteAccessPointPolicyForObjectLambda removes the policy from an Object Lambda AP.
func (b *InMemoryBackend) DeleteAccessPointPolicyForObjectLambda(accountID, name string) error {
	b.mu.Lock("DeleteAccessPointPolicyForObjectLambda")
	defer b.mu.Unlock()

	key := accountID + ":" + name
	delete(b.objectLambdaAPPolicies, key)

	return nil
}

// GetAccessPointPolicyStatusForObjectLambda returns the policy status for an Object Lambda AP.
func (b *InMemoryBackend) GetAccessPointPolicyStatusForObjectLambda(
	accountID, name string,
) (bool, error) {
	b.mu.RLock("GetAccessPointPolicyStatusForObjectLambda")
	defer b.mu.RUnlock()

	key := accountID + ":" + name
	if !b.objectLambdaAccessPoints.Has(key) {
		return false, fmt.Errorf("%w: %s", errObjectLambdaAPNotFound, name)
	}

	return b.objectLambdaAPPolicies[key] != "", nil
}

// GetAccessPointConfigurationForObjectLambda returns the configuration for an Object Lambda AP.
func (b *InMemoryBackend) GetAccessPointConfigurationForObjectLambda(
	accountID, name string,
) (string, error) {
	b.mu.RLock("GetAccessPointConfigurationForObjectLambda")
	defer b.mu.RUnlock()

	key := accountID + ":" + name
	if !b.objectLambdaAccessPoints.Has(key) {
		return "", fmt.Errorf("%w: %s", errObjectLambdaAPNotFound, name)
	}

	return b.objectLambdaAPConfigs[key], nil
}

// PutAccessPointConfigurationForObjectLambda sets the configuration for an Object Lambda AP.
func (b *InMemoryBackend) PutAccessPointConfigurationForObjectLambda(
	accountID, name, config string,
) error {
	b.mu.Lock("PutAccessPointConfigurationForObjectLambda")
	defer b.mu.Unlock()

	key := accountID + ":" + name
	if !b.objectLambdaAccessPoints.Has(key) {
		return fmt.Errorf("%w: %s", errObjectLambdaAPNotFound, name)
	}
	b.objectLambdaAPConfigs[key] = config

	if b.objectLambdaSink != nil {
		if bucket, lambdaARN, ok := b.resolveObjectLambdaTarget(accountID, config); ok {
			b.objectLambdaSink.SetObjectLambdaConfig(bucket, lambdaARN)
		}
	}

	return nil
}

// objectLambdaConfigXML mirrors the fields of types.ObjectLambdaConfiguration
// (aws-sdk-go-v2/service/s3control@v1.73.4 types.go:1770) needed to resolve
// the underlying bucket and Lambda ARN. It is unmarshalled from the raw inner
// XML captured by createJobXMLCapture.
type objectLambdaConfigXML struct {
	SupportingAccessPoint        string `xml:"SupportingAccessPoint"`
	TransformationConfigurations []struct {
		ContentTransformation struct {
			AwsLambda struct {
				FunctionArn string `xml:"FunctionArn"`
			} `xml:"AwsLambda"`
		} `xml:"ContentTransformation"`
	} `xml:"TransformationConfigurations>TransformationConfiguration"`
}

// resolveObjectLambdaTarget parses a PutAccessPointConfigurationForObjectLambda
// config's SupportingAccessPoint and Lambda FunctionArn, then resolves the
// supporting access point to its underlying bucket. Must be called with b.mu held.
func (b *InMemoryBackend) resolveObjectLambdaTarget(accountID, config string) (string, string, bool) {
	var parsed objectLambdaConfigXML
	if err := xml.Unmarshal([]byte("<c>"+config+"</c>"), &parsed); err != nil {
		return "", "", false
	}

	if len(parsed.TransformationConfigurations) == 0 {
		return "", "", false
	}

	lambdaARN := parsed.TransformationConfigurations[0].ContentTransformation.AwsLambda.FunctionArn
	if lambdaARN == "" {
		return "", "", false
	}

	apName, ok := accessPointNameFromARN(parsed.SupportingAccessPoint)
	if !ok {
		return "", "", false
	}

	ap, ok := b.accessPoints.Get(accountID + ":" + apName)
	if !ok {
		return "", "", false
	}

	return ap.Bucket, lambdaARN, true
}

// accessPointNameFromARN extracts the name from an
// "arn:aws:s3:region:account:accesspoint/name" ARN.
func accessPointNameFromARN(arn string) (string, bool) {
	_, name, found := strings.Cut(arn, "accesspoint/")

	return name, found
}
