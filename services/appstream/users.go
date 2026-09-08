package appstream

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const userStatusCreated = "CREATED"

type storedUser struct {
	CreatedTime        time.Time `json:"createdTime"`
	UserName           string    `json:"userName"`
	Arn                string    `json:"arn"`
	FirstName          string    `json:"firstName"`
	LastName           string    `json:"lastName"`
	AuthenticationType string    `json:"authenticationType"`
	Status             string    `json:"status"`
	Enabled            bool      `json:"enabled"`
}

func (u *storedUser) toUser() *User {
	return &User{
		CreatedTime:        u.CreatedTime,
		UserName:           u.UserName,
		Arn:                u.Arn,
		FirstName:          u.FirstName,
		LastName:           u.LastName,
		AuthenticationType: u.AuthenticationType,
		Status:             u.Status,
		Enabled:            u.Enabled,
	}
}

func userKey(userName, authType string) string { return userName + "\x00" + authType }

func (b *InMemoryBackend) userARN(userName, authType string) string {
	return arn.Build("appstream", b.region, b.accountID, fmt.Sprintf("user/%s/%s", authType, userName))
}

// CreateUser creates a new UserPool user.
func (b *InMemoryBackend) CreateUser(userName, firstName, lastName, authType string) (*User, error) {
	b.mu.Lock("CreateUser")
	defer b.mu.Unlock()

	key := userKey(userName, authType)
	if b.users.Has(key) {
		return nil, ErrAlreadyExists
	}

	u := &storedUser{
		CreatedTime:        time.Now().UTC(),
		UserName:           userName,
		Arn:                b.userARN(userName, authType),
		FirstName:          firstName,
		LastName:           lastName,
		AuthenticationType: authType,
		Status:             userStatusCreated,
		Enabled:            true,
	}
	b.users.Put(u)

	return u.toUser(), nil
}

// DeleteUser removes a user.
func (b *InMemoryBackend) DeleteUser(userName, authType string) error {
	b.mu.Lock("DeleteUser")
	defer b.mu.Unlock()

	key := userKey(userName, authType)
	if !b.users.Has(key) {
		return ErrNotFound
	}

	b.users.Delete(key)
	delete(b.userStackAssoc, key)

	return nil
}

// DescribeUsers returns users, optionally filtered by authentication type.
func (b *InMemoryBackend) DescribeUsers(authType string) ([]*User, error) {
	b.mu.RLock("DescribeUsers")
	defer b.mu.RUnlock()

	var result []*User

	for _, u := range b.users.All() {
		if authType != "" && u.AuthenticationType != authType {
			continue
		}

		result = append(result, u.toUser())
	}

	return result, nil
}

// DisableUser disables a user.
func (b *InMemoryBackend) DisableUser(userName, authType string) error {
	b.mu.Lock("DisableUser")
	defer b.mu.Unlock()

	key := userKey(userName, authType)
	u, ok := b.users.Get(key)
	if !ok {
		return ErrNotFound
	}

	u.Enabled = false

	return nil
}

// EnableUser re-enables a user.
func (b *InMemoryBackend) EnableUser(userName, authType string) error {
	b.mu.Lock("EnableUser")
	defer b.mu.Unlock()

	key := userKey(userName, authType)
	u, ok := b.users.Get(key)
	if !ok {
		return ErrNotFound
	}

	u.Enabled = true

	return nil
}

// BatchAssociateUserStack links users to stacks.
func (b *InMemoryBackend) BatchAssociateUserStack(
	associations []UserStackAssociation,
) ([]UserStackAssociationError, error) {
	b.mu.Lock("BatchAssociateUserStack")
	defer b.mu.Unlock()

	var errs []UserStackAssociationError

	for _, assoc := range associations {
		key := userKey(assoc.UserName, assoc.AuthenticationType)
		if !b.users.Has(key) {
			a := assoc
			errs = append(errs, UserStackAssociationError{
				UserStackAssociation: &a,
				ErrorCode:            "USER_NAME_NOT_FOUND",
				ErrorMessage:         "User not found",
			})

			continue
		}

		if !b.stacks.Has(assoc.StackName) {
			a := assoc
			errs = append(errs, UserStackAssociationError{
				UserStackAssociation: &a,
				ErrorCode:            "STACK_NOT_FOUND",
				ErrorMessage:         "Stack not found",
			})

			continue
		}

		if b.userStackAssoc[key] == nil {
			b.userStackAssoc[key] = make(map[string]bool)
		}

		b.userStackAssoc[key][assoc.StackName] = true
	}

	return errs, nil
}

// BatchDisassociateUserStack unlinks users from stacks.
func (b *InMemoryBackend) BatchDisassociateUserStack(
	associations []UserStackAssociation,
) ([]UserStackAssociationError, error) {
	b.mu.Lock("BatchDisassociateUserStack")
	defer b.mu.Unlock()

	var errs []UserStackAssociationError

	for _, assoc := range associations {
		key := userKey(assoc.UserName, assoc.AuthenticationType)
		if !b.users.Has(key) {
			a := assoc
			errs = append(errs, UserStackAssociationError{
				UserStackAssociation: &a,
				ErrorCode:            "USER_NAME_NOT_FOUND",
				ErrorMessage:         "User not found",
			})

			continue
		}

		if !b.stacks.Has(assoc.StackName) {
			a := assoc
			errs = append(errs, UserStackAssociationError{
				UserStackAssociation: &a,
				ErrorCode:            "STACK_NOT_FOUND",
				ErrorMessage:         "Stack not found",
			})

			continue
		}

		if b.userStackAssoc[key] != nil {
			delete(b.userStackAssoc[key], assoc.StackName)
		}
	}

	return errs, nil
}

// DescribeUserStackAssociations returns user-stack links, optionally filtered.
func (b *InMemoryBackend) DescribeUserStackAssociations(
	stackName, userName, authType string,
) ([]*UserStackAssociation, error) {
	b.mu.RLock("DescribeUserStackAssociations")
	defer b.mu.RUnlock()

	var result []*UserStackAssociation

	for uKey, stacks := range b.userStackAssoc {
		u, ok := b.users.Get(uKey)
		if !ok {
			continue
		}

		if userName != "" && u.UserName != userName {
			continue
		}

		if authType != "" && u.AuthenticationType != authType {
			continue
		}

		for sName := range stacks {
			if stackName != "" && sName != stackName {
				continue
			}

			result = append(result, &UserStackAssociation{
				UserName:           u.UserName,
				StackName:          sName,
				AuthenticationType: u.AuthenticationType,
			})
		}
	}

	return result, nil
}
