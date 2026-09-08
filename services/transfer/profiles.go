package transfer

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"
)

// CreateProfile creates an AS2 profile. ProfileType must be LOCAL or PARTNER.
func (b *InMemoryBackend) CreateProfile(
	profileType, as2ID string,
	tags map[string]string,
) (*Profile, error) {
	switch profileType {
	case profileTypeLocal, profileTypePartner:
		// valid
	default:
		return nil, fmt.Errorf(
			"%w: ProfileType must be LOCAL or PARTNER, got %q",
			ErrValidation,
			profileType,
		)
	}

	b.mu.Lock("CreateProfile")
	defer b.mu.Unlock()

	profileID := "p-" + uuid.NewString()[:20]

	merged := make(map[string]string, len(tags))
	maps.Copy(merged, tags)

	p := &Profile{
		ProfileID:   profileID,
		ProfileType: profileType,
		As2ID:       as2ID,
		CreatedAt:   time.Now(),
		Tags:        merged,
		AccountID:   b.accountID,
		Region:      b.region,
	}
	b.profiles.Put(p)
	b.initTagsStore(profileARN(b.accountID, b.region, profileID), merged)

	return cloneProfile(p), nil
}

// DeleteProfile removes a profile by ID.
func (b *InMemoryBackend) DeleteProfile(profileID string) error {
	b.mu.Lock("DeleteProfile")
	defer b.mu.Unlock()

	if !b.profiles.Has(profileID) {
		return fmt.Errorf("%w: profile %s not found", ErrProfileNotFound, profileID)
	}

	b.profiles.Delete(profileID)
	delete(b.tagsStore, profileARN(b.accountID, b.region, profileID))

	return nil
}

// DescribeProfile returns a profile by ID.
func (b *InMemoryBackend) DescribeProfile(profileID string) (*Profile, error) {
	b.mu.RLock("DescribeProfile")
	defer b.mu.RUnlock()

	p, ok := b.profiles.Get(profileID)
	if !ok {
		return nil, fmt.Errorf("%w: profile %s not found", ErrProfileNotFound, profileID)
	}

	return cloneProfile(p), nil
}

// ListProfiles returns all profiles sorted by profileID.
func (b *InMemoryBackend) ListProfiles() []*Profile {
	b.mu.RLock("ListProfiles")
	defer b.mu.RUnlock()

	all := b.profiles.All()
	out := make([]*Profile, 0, len(all))

	for _, p := range all {
		out = append(out, cloneProfile(p))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ProfileID < out[j].ProfileID
	})

	return out
}

// UpdateProfileInput holds mutable fields for UpdateProfile.
type UpdateProfileInput struct {
	ProfileID         string
	As2ID             string
	CertificateIDs    []string
	SetCertificateIDs bool
}

// UpdateProfile updates mutable fields on a profile (simplified, single-arg form).
func (b *InMemoryBackend) UpdateProfile(profileID, as2ID string) (*Profile, error) {
	return b.UpdateProfileFull(&UpdateProfileInput{ProfileID: profileID, As2ID: as2ID})
}

// UpdateProfileFull updates all mutable fields on a profile.
func (b *InMemoryBackend) UpdateProfileFull(in *UpdateProfileInput) (*Profile, error) {
	b.mu.Lock("UpdateProfileFull")
	defer b.mu.Unlock()

	p, ok := b.profiles.Get(in.ProfileID)
	if !ok {
		return nil, fmt.Errorf("%w: profile %s not found", ErrProfileNotFound, in.ProfileID)
	}

	if in.As2ID != "" {
		p.As2ID = in.As2ID
	}

	if in.SetCertificateIDs {
		if in.CertificateIDs != nil {
			cp := make([]string, len(in.CertificateIDs))
			copy(cp, in.CertificateIDs)
			p.CertificateIDs = cp
		} else {
			p.CertificateIDs = nil
		}
	}

	return cloneProfile(p), nil
}

// AddProfileInternal seeds a profile for testing purposes.
func (b *InMemoryBackend) AddProfileInternal(profileID, profileType string) {
	b.mu.Lock("AddProfileInternal")
	defer b.mu.Unlock()

	b.profiles.Put(&Profile{
		ProfileID:   profileID,
		ProfileType: profileType,
		CreatedAt:   time.Now(),
		Tags:        make(map[string]string),
		AccountID:   b.accountID,
		Region:      b.region,
	})
}
