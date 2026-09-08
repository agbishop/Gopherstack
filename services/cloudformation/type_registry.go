package cloudformation

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

func (b *InMemoryBackend) ActivateType(typeName, typeArn string) (string, error) {
	b.mu.Lock("ActivateType")
	defer b.mu.Unlock()
	key := typeArn
	if key == "" {
		key = "arn:aws:cloudformation:::type/resource/" + typeName
	}
	if t, ok := b.typeRegistry.Get(key); ok {
		t.IsActivated = true
	} else {
		b.typeRegistry.Put(&RegisteredType{
			TypeArn:     key,
			TypeName:    typeName,
			Type:        typeKindResource,
			VersionID:   "00000001",
			Status:      statusComplete,
			IsActivated: true,
		})
	}

	return key, nil
}

func (b *InMemoryBackend) DeactivateType(typeName, typeArn string) error {
	b.mu.Lock("DeactivateType")
	defer b.mu.Unlock()
	key := typeArn
	if key == "" {
		key = "arn:aws:cloudformation:::type/resource/" + typeName
	}
	t, ok := b.typeRegistry.Get(key)
	if !ok || !t.IsActivated {
		return fmt.Errorf("%w: %s", ErrTypeNotFound, key)
	}
	t.IsActivated = false

	return nil
}

func (b *InMemoryBackend) RegisterType(typeName, _ string) (string, error) {
	b.mu.Lock("RegisterType")
	defer b.mu.Unlock()
	token := uuid.New().String()
	typeArn := "arn:aws:cloudformation:::type/resource/" + typeName
	// Each call to RegisterType creates a new version.
	existingVersions := b.typeVersions[typeArn]
	versionNum := len(existingVersions) + 1
	versionID := fmt.Sprintf("%08d", versionNum)
	b.typeVersions[typeArn] = append(b.typeVersions[typeArn], &RegisteredTypeVersion{
		TypeArn:   typeArn,
		VersionID: versionID,
		IsDefault: true,
		Status:    statusComplete,
	})
	// Mark prior versions as non-default.
	for i := range b.typeVersions[typeArn][:len(b.typeVersions[typeArn])-1] {
		b.typeVersions[typeArn][i].IsDefault = false
	}
	if t, ok := b.typeRegistry.Get(typeArn); ok {
		t.VersionID = versionID
		t.DefaultVersion = versionID
	} else {
		b.typeRegistry.Put(&RegisteredType{
			TypeArn:        typeArn,
			TypeName:       typeName,
			Type:           "RESOURCE",
			VersionID:      versionID,
			DefaultVersion: versionID,
			Status:         statusComplete,
		})
	}
	b.typeRegistrations.Put(&TypeRegistrationRecord{
		Token:    token,
		TypeName: typeName,
		TypeArn:  typeArn,
		Status:   statusComplete,
	})

	return token, nil
}

// DeregisterType deprecates a type or a single version of it (cloudformation@v1.76.1
// api_op_DeregisterType.go doc comment). With no versionID it deprecates the whole
// type. With a versionID: deregistering the default version while other active
// versions exist is rejected; deregistering the last active version (including the
// only version) deprecates the whole type along with it.
func (b *InMemoryBackend) DeregisterType(typeName, typeArn, versionID string) error {
	b.mu.Lock("DeregisterType")
	defer b.mu.Unlock()

	key := typeArn
	if key == "" {
		key = "arn:aws:cloudformation:::type/resource/" + typeName
	}
	t, ok := b.typeRegistry.Get(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrTypeNotFound, key)
	}

	if versionID == "" {
		t.Status = typeStatusDeprecated
		for _, v := range b.typeVersions[key] {
			v.Status = typeStatusDeprecated
		}

		return nil
	}

	versions := b.typeVersions[key]
	if len(versions) == 0 {
		if versionID != t.VersionID {
			return fmt.Errorf("%w: %s version %s", ErrTypeVersionNotFound, key, versionID)
		}
		t.Status = typeStatusDeprecated

		return nil
	}

	var target *RegisteredTypeVersion
	activeCount := 0
	for _, v := range versions {
		if v.Status != typeStatusDeprecated {
			activeCount++
		}
		if v.VersionID == versionID {
			target = v
		}
	}
	if target == nil || target.Status == typeStatusDeprecated {
		return fmt.Errorf("%w: %s version %s", ErrTypeVersionNotFound, key, versionID)
	}

	isDefault := versionID == t.DefaultVersion
	if isDefault && activeCount > 1 {
		return fmt.Errorf("%w: %s version %s", ErrCannotDeregisterDefaultVersion, key, versionID)
	}

	target.Status = typeStatusDeprecated
	target.IsDefault = false
	if isDefault {
		t.Status = typeStatusDeprecated
	}

	return nil
}

func (b *InMemoryBackend) PublishType(typeName string) (string, error) {
	b.mu.Lock("PublishType")
	defer b.mu.Unlock()
	typeArn := "arn:aws:cloudformation:::type/resource/" + typeName
	t, ok := b.typeRegistry.Get(typeArn)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTypeNotFound, typeArn)
	}
	t.IsPublished = true

	return typeArn, nil
}

func (b *InMemoryBackend) SetTypeDefaultVersion(typeArn, version string) error {
	b.mu.Lock("SetTypeDefaultVersion")
	defer b.mu.Unlock()
	t, ok := b.typeRegistry.Get(typeArn)
	if !ok {
		return fmt.Errorf("%w: %s", ErrTypeNotFound, typeArn)
	}
	t.DefaultVersion = version
	t.VersionID = version
	// Update typeVersions IsDefault flags.
	for _, v := range b.typeVersions[typeArn] {
		v.IsDefault = v.VersionID == version
	}

	return nil
}

func (b *InMemoryBackend) SetTypeConfiguration(typeName, configuration string) (string, error) {
	b.mu.Lock("SetTypeConfiguration")
	defer b.mu.Unlock()
	b.typeConfigs[typeName] = configuration

	return "arn:aws:cloudformation:::type-configuration/resource/" + typeName + "/default", nil
}

func (b *InMemoryBackend) BatchDescribeTypeConfigurations(
	identifiers []TypeConfigurationIdentifier,
) ([]TypeConfigurationDetail, []BatchDescribeTypeConfigurationsError, []TypeConfigurationIdentifier) {
	b.mu.RLock("BatchDescribeTypeConfigurations")
	defer b.mu.RUnlock()

	var (
		details     []TypeConfigurationDetail
		errs        []BatchDescribeTypeConfigurationsError
		unprocessed []TypeConfigurationIdentifier
	)

	for _, ident := range identifiers {
		name := ident.TypeName
		if name == "" {
			name = ident.TypeConfigurationArn
		}
		if name == "" {
			unprocessed = append(unprocessed, ident)

			continue
		}

		typeArn := ident.TypeArn
		if typeArn == "" {
			typeArn = "arn:aws:cloudformation:::type/resource/" + name
		}
		cfg, hasCfg := b.typeConfigs[name]
		_, registered := b.typeRegistry.Get(typeArn)
		if !hasCfg && !registered {
			errs = append(errs, BatchDescribeTypeConfigurationsError{
				TypeConfigurationIdentifier: &ident,
				// BatchDescribeTypeConfigurations' own deserializer declares
				// CFNRegistryException/TypeConfigurationNotFoundException, not
				// TypeNotFoundException -- that code belongs to
				// ActivateType/DeactivateType/DeregisterType/DescribeType/
				// PublishType, which operate on types rather than type
				// configurations (confirmed against
				// aws-sdk-go-v2/service/cloudformation@v1.76.1/deserializers.go).
				ErrorCode:    "TypeConfigurationNotFoundException",
				ErrorMessage: fmt.Sprintf("type configuration not found: %s", name),
			})

			continue
		}
		if cfg == "" {
			cfg = "{}"
		}
		const defaultConfigSuffix = "/default"
		configArn := ident.TypeConfigurationArn
		if configArn == "" {
			configArn = "arn:aws:cloudformation:::type-configuration/resource/" + name + defaultConfigSuffix
		}
		details = append(details, TypeConfigurationDetail{
			Arn:                    configArn,
			TypeName:               name,
			TypeArn:                typeArn,
			Alias:                  ident.TypeConfigurationAlias,
			Configuration:          cfg,
			IsDefaultConfiguration: !hasCfg,
		})
	}

	return details, errs, unprocessed
}

func (b *InMemoryBackend) ListTypes(_ string) ([]TypeSummary, error) {
	b.mu.RLock("ListTypes")
	defer b.mu.RUnlock()
	result := make([]TypeSummary, 0, b.typeRegistry.Len())
	for _, t := range b.typeRegistry.All() {
		if t.Status == typeStatusDeprecated {
			continue
		}
		if t.Status == statusComplete || t.IsActivated {
			visibility := "PRIVATE"
			if t.IsPublished {
				visibility = typeVisibilityPublic
			}
			result = append(result, TypeSummary{
				TypeName:         t.TypeName,
				TypeArn:          t.TypeArn,
				Type:             t.Type,
				Visibility:       visibility,
				Description:      t.Configuration,
				DefaultVersionID: t.DefaultVersion,
				IsActivated:      t.IsActivated,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TypeName < result[j].TypeName })

	return result, nil
}

// ListTypeVersions defaults to LIVE versions only, matching ListTypeVersionsInput's
// DeprecatedStatus field ("The default is LIVE", cloudformation@v1.76.1
// api_op_ListTypeVersions.go).
func (b *InMemoryBackend) ListTypeVersions(typeName, deprecatedStatus string) ([]string, error) {
	b.mu.RLock("ListTypeVersions")
	defer b.mu.RUnlock()
	typeArn := "arn:aws:cloudformation:::type/resource/" + typeName
	wantDeprecated := deprecatedStatus == typeStatusDeprecated
	if versions, ok := b.typeVersions[typeArn]; ok && len(versions) > 0 {
		ids := make([]string, 0, len(versions))
		for _, v := range versions {
			if (v.Status == typeStatusDeprecated) != wantDeprecated {
				continue
			}
			ids = append(ids, v.VersionID)
		}

		return ids, nil
	}
	// Fallback: if no version records but type exists, return its current version.
	if t, ok := b.typeRegistry.Get(typeArn); ok {
		if (t.Status == typeStatusDeprecated) != wantDeprecated {
			return []string{}, nil
		}

		return []string{t.VersionID}, nil
	}

	return []string{}, nil
}

func (b *InMemoryBackend) ListTypeRegistrations(typeName, _ string) ([]string, error) {
	b.mu.RLock("ListTypeRegistrations")
	defer b.mu.RUnlock()
	var tokens []string
	for _, rec := range b.typeRegistrations.All() {
		if typeName == "" || rec.TypeName == typeName {
			tokens = append(tokens, rec.Token)
		}
	}

	return tokens, nil
}

func (b *InMemoryBackend) DescribeTypeRegistration(registrationToken string) (string, error) {
	b.mu.RLock("DescribeTypeRegistration")
	defer b.mu.RUnlock()
	rec, ok := b.typeRegistrations.Get(registrationToken)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrRegistrationTokenNotFound, registrationToken)
	}

	return rec.Status, nil
}

func (b *InMemoryBackend) TestType(typeName, typeArn string) (string, error) {
	b.mu.Lock("TestType")
	defer b.mu.Unlock()
	token := uuid.New().String()
	key := typeArn
	if key == "" {
		key = "arn:aws:cloudformation:::type/resource/" + typeName
	}
	b.typeRegistrations.Put(&TypeRegistrationRecord{
		Token:    token,
		TypeName: typeName,
		TypeArn:  key,
		Status:   statusComplete,
	})

	return token, nil
}

func (b *InMemoryBackend) RegisterPublisher(connectionArn string) (string, error) {
	b.mu.Lock("RegisterPublisher")
	defer b.mu.Unlock()
	publisherID := uuid.New().String()
	b.publishers.Put(&Publisher{
		PublisherID:   publisherID,
		ConnectionArn: connectionArn,
		Status:        "VERIFIED",
	})

	return publisherID, nil
}

func (b *InMemoryBackend) DescribePublisher(publisherID string) (string, error) {
	b.mu.RLock("DescribePublisher")
	defer b.mu.RUnlock()
	p, ok := b.publishers.Get(publisherID)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrPublisherNotFound, publisherID)
	}

	return p.Status, nil
}

// typeVersionDeprecatedStatus reports LIVE/DEPRECATED for a resolved version,
// falling back to the whole type's status when the version itself isn't
// individually tracked as deprecated.
func (b *InMemoryBackend) typeVersionDeprecatedStatus(reg *RegisteredType, resolvedVersionID string) string {
	if reg.Status == typeStatusDeprecated {
		return typeStatusDeprecated
	}
	for _, v := range b.typeVersions[reg.TypeArn] {
		if v.VersionID == resolvedVersionID && v.Status == typeStatusDeprecated {
			return typeStatusDeprecated
		}
	}

	return "LIVE"
}

// DescribeType returns detailed information about a registered CloudFormation type.
// Lookup is by typeName, arn, or versionID — at least one must be non-empty.
func (b *InMemoryBackend) DescribeType(typeName, arn, versionID string) (*TypeDetails, error) {
	b.mu.RLock("DescribeType")
	defer b.mu.RUnlock()

	var reg *RegisteredType
	switch {
	case arn != "":
		r, ok := b.typeRegistry.Get(arn)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrTypeNotFound, arn)
		}
		reg = r
	case typeName != "":
		key := "arn:aws:cloudformation:::type/resource/" + typeName
		r, ok := b.typeRegistry.Get(key)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrTypeNotFound, typeName)
		}
		reg = r
	default:
		return nil, fmt.Errorf("%w: TypeName or Arn is required", ErrTypeNotFound)
	}

	// Determine which version to return.
	resolvedVersionID := reg.DefaultVersion
	if versionID != "" {
		// Verify the version exists.
		found := false
		for _, v := range b.typeVersions[reg.TypeArn] {
			if v.VersionID == versionID {
				found = true
				resolvedVersionID = versionID

				break
			}
		}
		if !found && len(b.typeVersions[reg.TypeArn]) > 0 {
			return nil, fmt.Errorf("%w: %s version %s", ErrTypeVersionNotFound, reg.TypeName, versionID)
		}
	}

	isDefaultVersion := resolvedVersionID == reg.DefaultVersion
	visibility := typeVisibilityPrivate
	if reg.IsPublished {
		visibility = typeVisibilityPublic
	}
	deprecatedStatus := b.typeVersionDeprecatedStatus(reg, resolvedVersionID)

	return &TypeDetails{
		TypeName:         reg.TypeName,
		TypeArn:          reg.TypeArn,
		Type:             reg.Type,
		Visibility:       visibility,
		Status:           reg.Status,
		VersionID:        resolvedVersionID,
		DefaultVersionID: reg.DefaultVersion,
		IsActivated:      reg.IsActivated,
		IsDefaultVersion: isDefaultVersion,
		DeprecatedStatus: deprecatedStatus,
	}, nil
}
