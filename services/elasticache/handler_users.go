package elasticache

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// parseUserPasswords collects password members from either the modern
// "AuthenticationMode.Passwords.member.N" location or the legacy top-level
// "Passwords.member.N" (both serialize identically -- see
// awsAwsquery_serializeDocumentPasswordListInput in the SDK -- AWS accepts
// either).
func parseUserPasswords(form url.Values) []string {
	if pw := parseRepeatedField(form, "AuthenticationMode.Passwords.member"); len(pw) > 0 {
		return pw
	}

	return parseRepeatedField(form, "Passwords.member")
}

// resolveUserAuth derives the wire-accurate output AuthType + PasswordCount
// from a Create/ModifyUser request's authentication fields: the modern
// AuthenticationMode.Type (translating the input-only "no-password-required"
// spelling to output's "no-password" -- see authType* constants), the legacy
// NoPasswordRequired boolean, or an implicit password auth when Passwords are
// given without an explicit mode. ok is false when password auth was
// requested with an invalid password count (AWS allows 1-2).
func resolveUserAuth(form url.Values) (string, int, bool) {
	passwords := parseUserPasswords(form)

	switch mode := form.Get("AuthenticationMode.Type"); {
	case mode == inputAuthTypeIAM:
		return authTypeIAM, 0, true
	case mode == inputAuthTypeNoPasswordRequired:
		return authTypeNoPasswordOutput, 0, true
	case mode == inputAuthTypePassword || len(passwords) > 0:
		if len(passwords) < 1 || len(passwords) > maxUserPasswords {
			return "", 0, false
		}

		return authTypePassword, len(passwords), true
	case strings.EqualFold(form.Get("NoPasswordRequired"), "true"):
		return authTypeNoPasswordOutput, 0, true
	default:
		// Nothing specified: AWS requires SOME authentication to be given,
		// but for a permissive default (matching this emulator's historical
		// behaviour when auth fields are omitted entirely) fall back to
		// no-password rather than rejecting the request.
		return authTypeNoPasswordOutput, 0, true
	}
}

// inputAuthType* mirror aws-sdk-go-v2/service/elasticache/types'
// InputAuthenticationType enum -- the wire strings accepted on
// AuthenticationMode.Type for Create/ModifyUser. Declared locally (not
// imported from the SDK types package) since the handler only parses raw
// form values, never SDK structs, for this query-protocol operation.
const (
	inputAuthTypePassword           = "password"
	inputAuthTypeNoPasswordRequired = "no-password-required"
	inputAuthTypeIAM                = "iam"
)

func (h *Handler) createUser(ctx context.Context, c *echo.Context, form url.Values) error {
	userID := form.Get("UserId")
	userName := form.Get("UserName")
	accessString := form.Get("AccessString")
	engine := form.Get("Engine")

	authType, passwordCount, ok := resolveUserAuth(form)
	if !ok {
		return xmlError(c, http.StatusBadRequest, "InvalidParameterValue",
			fmt.Sprintf("A user can have between 1 and %d passwords", maxUserPasswords))
	}

	u, err := h.Backend.CreateUserWithAuth(ctx, userID, userName, accessString, engine, authType, passwordCount)
	if err != nil {
		if errors.Is(err, ErrUserAlreadyExists) {
			return xmlError(c, http.StatusBadRequest, "UserAlreadyExists", "User already exists")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	h.applyCreateTimeTags(ctx, form, u.ARN)

	// The SDK deserializer reads the user fields directly from CreateUserResult (not under a User element).
	type result struct {
		XMLName      xml.Name          `xml:"CreateUserResponse"`
		Xmlns        string            `xml:"xmlns,attr"`
		ARN          string            `xml:"CreateUserResult>ARN"`
		UserID       string            `xml:"CreateUserResult>UserId"`
		UserName     string            `xml:"CreateUserResult>UserName"`
		Status       string            `xml:"CreateUserResult>Status"`
		Engine       string            `xml:"CreateUserResult>Engine,omitempty"`
		AccessString string            `xml:"CreateUserResult>AccessString,omitempty"`
		Auth         authenticationXML `xml:"CreateUserResult>Authentication"`
		UserGroupIDs userGroupIDsXML   `xml:"CreateUserResult>UserGroupIds"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:        elasticacheNS,
		ARN:          u.ARN,
		UserID:       u.UserID,
		UserName:     u.UserName,
		Status:       u.Status,
		Engine:       u.Engine,
		AccessString: u.AccessString,
		Auth:         authToXML(u),
		UserGroupIDs: userGroupIDsXML{Member: u.UserGroupIDs},
	})
}

// ----------------------------------------
// Shared batch update action types and helpers
// ----------------------------------------

// describeUsersResultXML is the XML envelope for DescribeUsers responses.
type describeUsersResultXML struct {
	XMLName xml.Name `xml:"DescribeUsersResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
	Marker  string   `xml:"DescribeUsersResult>Marker,omitempty"`
	Users   struct {
		Member []userXML `xml:"member"`
	} `xml:"DescribeUsersResult>Users"`
}

// authenticationXML is the wire shape of a User's Authentication struct
// (types.Authentication): Type is one of "password"/"no-password"/"iam",
// PasswordCount the number of passwords on file (never the passwords
// themselves -- AWS never echoes password material back).
type authenticationXML struct {
	Type          string `xml:"Type"`
	PasswordCount int    `xml:"PasswordCount"`
}

// userGroupIDsXML is the unlabeled <member> list wrapper for a User's
// UserGroupIds (locationName "member", matching every other unlabeled list
// in this API -- verified against the SDK's
// awsAwsquery_deserializeDocumentUserGroupIdList).
type userGroupIDsXML struct {
	Member []string `xml:"member"`
}

type userXML struct {
	ARN          string            `xml:"ARN"`
	UserID       string            `xml:"UserId"`
	UserName     string            `xml:"UserName"`
	Status       string            `xml:"Status"`
	Engine       string            `xml:"Engine,omitempty"`
	AccessString string            `xml:"AccessString,omitempty"`
	Auth         authenticationXML `xml:"Authentication"`
	UserGroupIDs userGroupIDsXML   `xml:"UserGroupIds"`
}

// authToXML translates a User's stored AuthType/PasswordCount into the wire
// Authentication struct.
func authToXML(u *User) authenticationXML {
	return authenticationXML{Type: u.AuthType, PasswordCount: u.PasswordCount}
}

func userToXML(u *User) userXML {
	return userXML{
		ARN:          u.ARN,
		UserID:       u.UserID,
		UserName:     u.UserName,
		Status:       u.Status,
		Engine:       u.Engine,
		AccessString: u.AccessString,
		Auth:         authToXML(u),
		UserGroupIDs: userGroupIDsXML{Member: u.UserGroupIDs},
	}
}

func (h *Handler) deleteUser(ctx context.Context, c *echo.Context, form url.Values) error {
	userID := form.Get("UserId")

	u, err := h.Backend.DeleteUser(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return xmlError(c, http.StatusNotFound, "UserNotFound", "User not found")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName      xml.Name          `xml:"DeleteUserResponse"`
		Xmlns        string            `xml:"xmlns,attr"`
		ARN          string            `xml:"DeleteUserResult>ARN"`
		UserID       string            `xml:"DeleteUserResult>UserId"`
		UserName     string            `xml:"DeleteUserResult>UserName"`
		Status       string            `xml:"DeleteUserResult>Status"`
		Engine       string            `xml:"DeleteUserResult>Engine,omitempty"`
		AccessString string            `xml:"DeleteUserResult>AccessString,omitempty"`
		Auth         authenticationXML `xml:"DeleteUserResult>Authentication"`
		UserGroupIDs userGroupIDsXML   `xml:"DeleteUserResult>UserGroupIds"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:        elasticacheNS,
		ARN:          u.ARN,
		UserID:       u.UserID,
		UserName:     u.UserName,
		Status:       u.Status,
		Engine:       u.Engine,
		AccessString: u.AccessString,
		Auth:         authToXML(u),
		UserGroupIDs: userGroupIDsXML{Member: u.UserGroupIDs},
	})
}

func (h *Handler) describeUsers(ctx context.Context, c *echo.Context, form url.Values) error {
	userID := form.Get("UserId")
	engine := form.Get("Engine")
	filterUserIDs := parseUserIDFilters(form)

	p, err := describeListChecked(c, form,
		func(marker string, maxRecords int) (page.Page[User], error) {
			return h.Backend.DescribeUsers(ctx, userID, marker, engine, maxRecords, filterUserIDs)
		},
		ErrUserNotFound, http.StatusNotFound, "UserNotFound", "User not found")
	if err != nil {
		return err
	}

	var res describeUsersResultXML
	res.Xmlns = elasticacheNS
	res.Marker = p.Next

	for i := range p.Data {
		res.Users.Member = append(res.Users.Member, userToXML(&p.Data[i]))
	}

	return xmlResp(c, http.StatusOK, res)
}

func (h *Handler) modifyUser(ctx context.Context, c *echo.Context, form url.Values) error {
	userID := form.Get("UserId")
	accessString := form.Get("AccessString")
	appendAccessString := form.Get("AppendAccessString")
	engine := form.Get("Engine")

	var (
		authType      string
		passwordCount *int
	)

	// Auth fields are all optional on Modify (unset = keep existing), unlike
	// Create where resolveUserAuth's permissive default always applies.
	switch {
	case form.Get("AuthenticationMode.Type") != "" || len(parseUserPasswords(form)) > 0:
		t, n, ok := resolveUserAuth(form)
		if !ok {
			return xmlError(c, http.StatusBadRequest, "InvalidParameterValue",
				fmt.Sprintf("A user can have between 1 and %d passwords", maxUserPasswords))
		}
		authType, passwordCount = t, &n
	case strings.EqualFold(form.Get("NoPasswordRequired"), "true"):
		authType = authTypeNoPasswordOutput
		zero := 0
		passwordCount = &zero
	}

	u, err := h.Backend.ModifyUserWithAuth(
		ctx, userID, accessString, appendAccessString, engine, authType, passwordCount,
	)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return xmlError(c, http.StatusNotFound, "UserNotFound", "User not found")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName      xml.Name          `xml:"ModifyUserResponse"`
		Xmlns        string            `xml:"xmlns,attr"`
		ARN          string            `xml:"ModifyUserResult>ARN"`
		UserID       string            `xml:"ModifyUserResult>UserId"`
		UserName     string            `xml:"ModifyUserResult>UserName"`
		Status       string            `xml:"ModifyUserResult>Status"`
		Engine       string            `xml:"ModifyUserResult>Engine,omitempty"`
		AccessString string            `xml:"ModifyUserResult>AccessString,omitempty"`
		Auth         authenticationXML `xml:"ModifyUserResult>Authentication"`
		UserGroupIDs userGroupIDsXML   `xml:"ModifyUserResult>UserGroupIds"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:        elasticacheNS,
		ARN:          u.ARN,
		UserID:       u.UserID,
		UserName:     u.UserName,
		Status:       u.Status,
		Engine:       u.Engine,
		AccessString: u.AccessString,
		Auth:         authToXML(u),
		UserGroupIDs: userGroupIDsXML{Member: u.UserGroupIDs},
	})
}
