package iot

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

func (b *InMemoryBackend) SetDefaultAuthorizer(authorizerName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.defaultAuthorizer = authorizerName

	return nil
}

func (b *InMemoryBackend) ClearDefaultAuthorizer() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.defaultAuthorizer = ""

	return nil
}

func (b *InMemoryBackend) DescribeDefaultAuthorizer() (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.defaultAuthorizer == "" {
		return "", fmt.Errorf("no default authorizer set: %w", ErrResourceNotFound)
	}

	return b.defaultAuthorizer, nil
}

// Authorizer represents an IoT authorizer.
type Authorizer struct {
	Tags                   map[string]string `json:"tags,omitempty"`
	TokenSigningPublicKeys map[string]string `json:"tokenSigningPublicKeys,omitempty"`
	AuthorizerName         string            `json:"authorizerName"`
	AuthorizerARN          string            `json:"authorizerArn"`
	AuthorizerFunctionARN  string            `json:"authorizerFunctionArn,omitempty"`
	TokenKeyName           string            `json:"tokenKeyName,omitempty"`
	Status                 string            `json:"status"`
	SigningDisabled        bool              `json:"signingDisabled"`
	EnableCachingForHTTP   bool              `json:"enableCachingForHttp"`
	CreationDate           float64           `json:"creationDate,omitempty"`
	LastModifiedDate       float64           `json:"lastModifiedDate,omitempty"`
}

func cloneAuthorizer(a *Authorizer) *Authorizer {
	cp := *a

	return &cp
}

func (b *InMemoryBackend) authorizerARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("authorizer/%s", name))
}

// CreateAuthorizerInput holds input for CreateAuthorizer.
type CreateAuthorizerInput struct {
	TokenSigningPublicKeys map[string]string `json:"tokenSigningPublicKeys,omitempty"`
	AuthorizerName         string            `json:"authorizerName"`
	AuthorizerFunctionARN  string            `json:"authorizerFunctionArn,omitempty"`
	TokenKeyName           string            `json:"tokenKeyName,omitempty"`
	Status                 string            `json:"status,omitempty"`
	// []types.Tag on the wire, not a map (serializers.go:1651, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags                 []tags.KV `json:"tags,omitempty"`
	SigningDisabled      bool      `json:"signingDisabled"`
	EnableCachingForHTTP bool      `json:"enableCachingForHttp"`
}

func (b *InMemoryBackend) CreateAuthorizer(input *CreateAuthorizerInput) (*Authorizer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.authorizers.Has(input.AuthorizerName) {
		return nil, fmt.Errorf(
			"authorizer %q already exists: %w",
			input.AuthorizerName,
			ErrAlreadyExists,
		)
	}
	now := float64(time.Now().Unix())
	a := &Authorizer{
		AuthorizerName:         input.AuthorizerName,
		AuthorizerARN:          b.authorizerARN(input.AuthorizerName),
		AuthorizerFunctionARN:  input.AuthorizerFunctionARN,
		TokenKeyName:           input.TokenKeyName,
		SigningDisabled:        input.SigningDisabled,
		EnableCachingForHTTP:   input.EnableCachingForHTTP,
		TokenSigningPublicKeys: input.TokenSigningPublicKeys,
		Status:                 input.Status,
		Tags:                   tags.MapFromKV(input.Tags),
		CreationDate:           now,
		LastModifiedDate:       now,
	}
	if a.Status == "" {
		a.Status = "ACTIVE"
	}
	b.authorizers.Put(a)
	b.putResourceTagsLocked(a.AuthorizerARN, a.Tags)

	return cloneAuthorizer(a), nil
}

func (b *InMemoryBackend) DescribeAuthorizer(name string) (*Authorizer, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	a, ok := b.authorizers.Get(name)
	if !ok {
		return nil, fmt.Errorf("authorizer %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneAuthorizer(a), nil
}

func (b *InMemoryBackend) ListAuthorizers() []*Authorizer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*Authorizer, 0, b.authorizers.Len())
	for _, v := range b.authorizers.Snapshot() {
		out = append(out, cloneAuthorizer(v))
	}

	return out
}

func (b *InMemoryBackend) UpdateAuthorizer(name, functionARN, status string) (*Authorizer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	a, ok := b.authorizers.Get(name)
	if !ok {
		return nil, fmt.Errorf("authorizer %q not found: %w", name, ErrResourceNotFound)
	}
	if functionARN != "" {
		a.AuthorizerFunctionARN = functionARN
	}
	if status != "" {
		a.Status = status
	}
	a.LastModifiedDate = float64(time.Now().Unix())

	return cloneAuthorizer(a), nil
}

func (b *InMemoryBackend) DeleteAuthorizer(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.authorizers.Has(name) {
		return fmt.Errorf("authorizer %q not found: %w", name, ErrResourceNotFound)
	}
	b.authorizers.Delete(name)
	delete(b.resourceTags, b.authorizerARN(name))

	return nil
}
