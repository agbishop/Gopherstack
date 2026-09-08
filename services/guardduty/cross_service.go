package guardduty

import (
	"github.com/blackbirdworks/gopherstack/pkgs/service"

	organizationsbackend "github.com/blackbirdworks/gopherstack/services/organizations"
)

// siblingServices is the subset of *CLI's method set this backend needs: the
// Organizations backend, so DeleteMembers/DisassociateMembers/
// StopMonitoringMembers can tell an account still in the AWS Organization
// apart from one that has already left it, instead of rejecting the whole
// call whenever autoEnableOrganizationMembers is ALL regardless of the
// requested accounts' actual membership (gopherstack-uu0n). Matched
// structurally against *CLI (no import of the top-level package, which
// would cycle) -- same pattern as services/grafana's cross_service.go.
type siblingServices interface {
	GetOrganizationsHandler() service.Registerable
}

// SetAppConfig records the service.AppContext.Config value Provider.Init
// received, so this backend can resolve the Organizations backend on demand
// -- see services/grafana/cross_service.go's SetAppConfig doc comment for
// why this must be lazy rather than resolved at construction time.
func (b *InMemoryBackend) SetAppConfig(cfg any) {
	b.appConfig = cfg
}

func (b *InMemoryBackend) siblings() (siblingServices, bool) {
	s, ok := b.appConfig.(siblingServices)

	return s, ok
}

// organizationsBackend returns the emulator's Organizations backend, if wired.
func (b *InMemoryBackend) organizationsBackend() (organizationsbackend.StorageBackend, bool) {
	s, ok := b.siblings()
	if !ok {
		return nil, false
	}

	h, ok := s.GetOrganizationsHandler().(*organizationsbackend.Handler)
	if !ok || h == nil {
		return nil, false
	}

	return h.Backend, true
}

// stillInOrganization reports whether accountID is a current member of the
// AWS Organization, per the wired services/organizations backend.
// RemoveAccountFromOrganization deletes the account from that backend's
// table (services/organizations/accounts.go), so DescribeAccount returning
// ErrAccountNotFound reliably means the account has left -- no separate
// membership flag is needed. When no Organizations backend is wired, or no
// organization/account is found there, membership cannot be confirmed, so
// this returns false: real DisassociateMembers/DeleteMembers/
// StopMonitoringMembers only error for accounts still in the organization,
// never for accounts whose organization membership is merely unknown.
func (b *InMemoryBackend) stillInOrganization(accountID string) bool {
	orgBk, ok := b.organizationsBackend()
	if !ok {
		return false
	}

	_, err := orgBk.DescribeAccount(accountID)

	return err == nil
}
