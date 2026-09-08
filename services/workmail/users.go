package workmail

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// --- Users ---

// CreateUser creates a new WorkMail user.
func (b *InMemoryBackend) CreateUser(orgID, name string, params CreateUserParams) (*User, error) {
	b.mu.Lock("CreateUser")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}
	validRoles := map[string]bool{roleUser: true, roleResource: true, roleSystemUser: true}
	if params.Role != "" && !validRoles[params.Role] {
		return nil, fmt.Errorf(
			"%w: invalid Role %q, must be USER, RESOURCE, or SYSTEM_USER",
			ErrValidation,
			params.Role,
		)
	}

	for _, u := range b.usersByOrg.Get(orgID) {
		if u.Name == name {
			return nil, fmt.Errorf("%w: user %q already exists", ErrNameUnavailable, name)
		}
	}

	userID := newID()
	now := time.Now().UTC()
	_ = params.Password // stored in real AWS but not needed for simulation

	u := &User{
		CreatedAt:                   now,
		UserID:                      userID,
		Name:                        name,
		DisplayName:                 params.DisplayName,
		FirstName:                   params.FirstName,
		LastName:                    params.LastName,
		Role:                        params.Role,
		State:                       stateDisabled,
		ARN:                         b.entityARN(orgID, "user", userID),
		IdentityProviderUserID:      params.IdentityProviderUserID,
		HiddenFromGlobalAddressList: params.HiddenFromGlobalAddressList,
		orgID:                       orgID,
	}

	b.users.Put(u)

	return u, nil
}

// DescribeUser returns details about a user.
func (b *InMemoryBackend) DescribeUser(orgID, entityID string) (*User, error) {
	b.mu.RLock("DescribeUser")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	u := b.findUser(orgID, entityID)
	if u == nil {
		return nil, fmt.Errorf("%w: user %q not found", ErrNotFound, entityID)
	}

	return u, nil
}

func (b *InMemoryBackend) findUser(orgID, entityID string) *User {
	if u, ok := b.users.Get(orgKey(orgID, entityID)); ok {
		return u
	}
	// search by name
	for _, u := range b.usersByOrg.Get(orgID) {
		if u.Name == entityID {
			return u
		}
	}

	return nil
}

// UpdateUser updates a user's profile fields. Empty strings and a nil
// HiddenFromGlobalAddressList leave the corresponding field unchanged,
// matching UpdateUserInput's optional-field semantics (see UpdateUserParams).
func (b *InMemoryBackend) UpdateUser(orgID, entityID string, params UpdateUserParams) error {
	b.mu.Lock("UpdateUser")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	u := b.findUser(orgID, entityID)
	if u == nil {
		return fmt.Errorf("%w: user %q not found", ErrNotFound, entityID)
	}

	applyUpdateUserParams(u, params)

	return nil
}

// applyUpdateUserParams copies every non-empty field from params onto u.
// Split into applyUpdateUserCoreFields/applyUpdateUserProfileFields to stay
// under the per-function cyclomatic-complexity budget.
func applyUpdateUserParams(u *User, params UpdateUserParams) {
	applyUpdateUserCoreFields(u, params)
	applyUpdateUserProfileFields(u, params)
}

// applyUpdateUserCoreFields copies the identity/role-adjacent fields
// (DisplayName, FirstName, LastName, IdentityProviderUserID, Role,
// HiddenFromGlobalAddressList).
func applyUpdateUserCoreFields(u *User, params UpdateUserParams) {
	if params.DisplayName != "" {
		u.DisplayName = params.DisplayName
	}
	if params.FirstName != "" {
		u.FirstName = params.FirstName
	}
	if params.LastName != "" {
		u.LastName = params.LastName
	}
	if params.IdentityProviderUserID != "" {
		u.IdentityProviderUserID = params.IdentityProviderUserID
	}
	if params.Role != "" {
		u.Role = params.Role
	}
	if params.HiddenFromGlobalAddressList != nil {
		u.HiddenFromGlobalAddressList = *params.HiddenFromGlobalAddressList
	}
}

// applyUpdateUserProfileFields copies the plain profile-metadata fields
// (City, Company, Country, Department, Initials, JobTitle, Office, Street,
// Telephone, ZipCode).
func applyUpdateUserProfileFields(u *User, params UpdateUserParams) {
	if params.City != "" {
		u.City = params.City
	}
	if params.Company != "" {
		u.Company = params.Company
	}
	if params.Country != "" {
		u.Country = params.Country
	}
	if params.Department != "" {
		u.Department = params.Department
	}
	if params.Initials != "" {
		u.Initials = params.Initials
	}
	if params.JobTitle != "" {
		u.JobTitle = params.JobTitle
	}
	if params.Office != "" {
		u.Office = params.Office
	}
	if params.Street != "" {
		u.Street = params.Street
	}
	if params.Telephone != "" {
		u.Telephone = params.Telephone
	}
	if params.ZipCode != "" {
		u.ZipCode = params.ZipCode
	}
}

// DeleteUser removes a user.
func (b *InMemoryBackend) DeleteUser(orgID, entityID string) error {
	b.mu.Lock("DeleteUser")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}
	u := b.findUser(orgID, entityID)
	if u == nil {
		// DeleteUser's own error model declares no not-found type for the
		// user itself (only Organization*); no correct code exists to send
		// here (gopherstack-6flj/uox6 error-envelope sweep).
		return fmt.Errorf("%w: user %q not found", ErrNotFound, entityID)
	}

	if u.State == stateEnabled {
		return fmt.Errorf(
			"%w: user %q is in ENABLED state and cannot be deleted; call DeregisterFromWorkMail first",
			ErrEntityState,
			entityID,
		)
	}

	actualID := u.UserID
	if u.Email != "" {
		delete(b.usersByEmail[orgID], u.Email)
		b.globalAliases.Delete(u.Email)
	}
	delete(b.mailboxQuotas[orgID], actualID)
	b.cascadeCleanEntity(orgID, actualID, u.ARN)
	b.users.Delete(orgKey(orgID, actualID))

	return nil
}

// ListUsers returns a paginated list of users, optionally narrowed by
// filter (see UserFilter -- mirrors ListUsersInput.Filters; a nil filter
// matches every user, matching the real API's "no Filters" behavior).
func (b *InMemoryBackend) ListUsers(
	orgID string,
	filter *UserFilter,
	maxResults int32,
	nextToken string,
) ([]*UserSummary, string, error) {
	b.mu.RLock("ListUsers")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, "", fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}

	users := make([]*UserSummary, 0)
	for _, u := range b.usersByOrg.Get(orgID) {
		if !userMatchesFilter(u, filter) {
			continue
		}
		users = append(users, &UserSummary{
			UserID:                          u.UserID,
			Name:                            u.Name,
			Email:                           u.Email,
			DisplayName:                     u.DisplayName,
			State:                           u.State,
			Role:                            u.Role,
			EnabledDate:                     u.EnabledDate,
			DisabledDate:                    u.DisabledDate,
			IdentityProviderIdentityStoreID: u.IdentityProviderIdentityStoreID,
			IdentityProviderUserID:          u.IdentityProviderUserID,
		})
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })

	items, next := paginate(users, maxResults, nextToken)

	return items, next, nil
}

// userMatchesFilter reports whether u satisfies every non-empty dimension
// of filter. A nil filter (or one with every field zero) matches everything.
func userMatchesFilter(u *User, filter *UserFilter) bool {
	if filter == nil {
		return true
	}
	if filter.DisplayNamePrefix != "" && !strings.HasPrefix(u.DisplayName, filter.DisplayNamePrefix) {
		return false
	}
	if filter.PrimaryEmailPrefix != "" && !strings.HasPrefix(u.Email, filter.PrimaryEmailPrefix) {
		return false
	}
	if filter.State != "" && u.State != filter.State {
		return false
	}
	if filter.UsernamePrefix != "" && !strings.HasPrefix(u.Name, filter.UsernamePrefix) {
		return false
	}
	if filter.IdentityProviderUserIDPrefix != "" &&
		!strings.HasPrefix(u.IdentityProviderUserID, filter.IdentityProviderUserIDPrefix) {
		return false
	}

	return true
}

// RegisterToWorkMail assigns an email address to a user/group/resource.
// Per its own doc (api_op_RegisterToWorkMail.go), it "performs no change if
// the user, group, or resource is enabled" -- an already-ENABLED entity is
// left untouched (including its existing email) rather than reassigned.
func (b *InMemoryBackend) RegisterToWorkMail(orgID, entityID, email string) error {
	b.mu.Lock("RegisterToWorkMail")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	if u := b.findUser(orgID, entityID); u != nil {
		return b.registerUserToWorkMail(orgID, u, email)
	}
	if g := b.findGroup(orgID, entityID); g != nil {
		return b.registerGroupToWorkMail(orgID, g, email)
	}
	if r := b.findResource(orgID, entityID); r != nil {
		return b.registerResourceToWorkMail(orgID, r, email)
	}

	return fmt.Errorf("%w: entity %q not found", ErrNotFound, entityID)
}

func (b *InMemoryBackend) registerUserToWorkMail(orgID string, u *User, email string) error {
	if u.State == stateEnabled {
		return nil
	}
	if err := b.checkEmailAvailable(orgID, email); err != nil {
		return err
	}
	if u.Email != "" {
		delete(b.usersByEmail[orgID], u.Email)
		b.globalAliases.Delete(u.Email)
	}

	now := time.Now().UTC()
	u.Email = email
	u.State = stateEnabled
	u.EnabledDate = now
	u.MailboxProvisionedDate = now
	b.usersByEmail[orgID][email] = u.UserID
	b.globalAliases.Put(&trackedAlias{Alias: email, OrgID: orgID, EntityID: u.UserID})

	return nil
}

func (b *InMemoryBackend) registerGroupToWorkMail(orgID string, g *Group, email string) error {
	if g.State == stateEnabled {
		return nil
	}
	if err := b.checkEmailAvailable(orgID, email); err != nil {
		return err
	}
	if g.Email != "" {
		delete(b.groupsByEmail[orgID], g.Email)
		b.globalAliases.Delete(g.Email)
	}

	g.Email = email
	g.State = stateEnabled
	g.EnabledDate = time.Now().UTC()
	b.groupsByEmail[orgID][email] = g.GroupID
	b.globalAliases.Put(&trackedAlias{Alias: email, OrgID: orgID, EntityID: g.GroupID})

	return nil
}

func (b *InMemoryBackend) registerResourceToWorkMail(orgID string, r *Resource, email string) error {
	if r.State == stateEnabled {
		return nil
	}
	if err := b.checkEmailAvailable(orgID, email); err != nil {
		return err
	}
	if r.Email != "" {
		delete(b.resourcesByEmail[orgID], r.Email)
		b.globalAliases.Delete(r.Email)
	}

	r.Email = email
	r.State = stateEnabled
	r.EnabledDate = time.Now().UTC()
	b.resourcesByEmail[orgID][email] = r.ResourceID
	b.globalAliases.Put(&trackedAlias{Alias: email, OrgID: orgID, EntityID: r.ResourceID})

	return nil
}

func (b *InMemoryBackend) checkEmailAvailable(orgID, email string) error {
	if ta, exists := b.globalAliases.Get(email); exists && ta.OrgID == orgID {
		return fmt.Errorf("%w: email %q already in use", ErrEmailInUse, email)
	}

	return nil
}

// DeregisterFromWorkMail removes an email address assignment.
func (b *InMemoryBackend) DeregisterFromWorkMail(orgID, entityID string) error {
	b.mu.Lock("DeregisterFromWorkMail")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	now := time.Now().UTC()

	if u := b.findUser(orgID, entityID); u != nil {
		if u.Email != "" {
			delete(b.usersByEmail[orgID], u.Email)
			b.globalAliases.Delete(u.Email)
		}
		u.Email = ""
		u.State = stateDisabled
		u.DisabledDate = now
		u.MailboxDeprovisionedDate = now

		return nil
	}

	if g := b.findGroup(orgID, entityID); g != nil {
		if g.Email != "" {
			delete(b.groupsByEmail[orgID], g.Email)
			b.globalAliases.Delete(g.Email)
		}
		g.Email = ""
		g.State = stateDisabled
		g.DisabledDate = now

		return nil
	}

	if r := b.findResource(orgID, entityID); r != nil {
		if r.Email != "" {
			delete(b.resourcesByEmail[orgID], r.Email)
			b.globalAliases.Delete(r.Email)
		}
		r.Email = ""
		r.State = stateDisabled
		r.DisabledDate = now

		return nil
	}

	return fmt.Errorf("%w: entity %q not found", ErrNotFound, entityID)
}

// ResetPassword updates the user's password (simulated — no-op).
func (b *InMemoryBackend) ResetPassword(orgID, userID, _ string) error {
	b.mu.RLock("ResetPassword")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	u := b.findUser(orgID, userID)
	if u == nil {
		return fmt.Errorf("%w: user %q not found", ErrNotFound, userID)
	}

	return nil
}

// UpdatePrimaryEmailAddress updates the primary email of an entity.
func (b *InMemoryBackend) UpdatePrimaryEmailAddress(orgID, entityID, email string) error {
	b.mu.Lock("UpdatePrimaryEmailAddress")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	if u := b.findUser(orgID, entityID); u != nil {
		if u.Email != "" {
			delete(b.usersByEmail[orgID], u.Email)
			b.globalAliases.Delete(u.Email)
		}
		u.Email = email
		b.usersByEmail[orgID][email] = u.UserID
		b.globalAliases.Put(&trackedAlias{Alias: email, OrgID: orgID, EntityID: u.UserID})

		return nil
	}

	if g := b.findGroup(orgID, entityID); g != nil {
		if g.Email != "" {
			delete(b.groupsByEmail[orgID], g.Email)
			b.globalAliases.Delete(g.Email)
		}
		g.Email = email
		b.groupsByEmail[orgID][email] = g.GroupID
		b.globalAliases.Put(&trackedAlias{Alias: email, OrgID: orgID, EntityID: g.GroupID})

		return nil
	}

	if r := b.findResource(orgID, entityID); r != nil {
		if r.Email != "" {
			delete(b.resourcesByEmail[orgID], r.Email)
			b.globalAliases.Delete(r.Email)
		}
		r.Email = email
		b.resourcesByEmail[orgID][email] = r.ResourceID
		b.globalAliases.Put(&trackedAlias{Alias: email, OrgID: orgID, EntityID: r.ResourceID})

		return nil
	}

	return fmt.Errorf("%w: entity %q not found", ErrNotFound, entityID)
}
