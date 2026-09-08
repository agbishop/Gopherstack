package emrserverless

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	// SessionStateSubmitted identifies a newly submitted session.
	SessionStateSubmitted = "SUBMITTED"
	// SessionStateStarting identifies a session being provisioned.
	SessionStateStarting = "STARTING"
	// SessionStateStarted identifies an active session.
	SessionStateStarted = "STARTED"
	// SessionStateIdle identifies an idle active session.
	SessionStateIdle = "IDLE"
	// SessionStateBusy identifies a busy active session.
	SessionStateBusy = "BUSY"
	// SessionStateFailed identifies a failed session.
	SessionStateFailed = "FAILED"
	// SessionStateTerminated identifies a terminated session.
	SessionStateTerminated = "TERMINATED"
)

const resourceTypeSession = "SESSION"

func (b *InMemoryBackend) sessionARN(applicationID, sessionID string) string {
	return arn.Build("emr-serverless", b.region, b.accountID,
		fmt.Sprintf("/applications/%s/sessions/%s", applicationID, sessionID))
}

// Session represents an interactive EMR Serverless session.
type Session struct {
	Tags                   map[string]string `json:"tags,omitempty"`
	ConfigurationOverrides map[string]any    `json:"configurationOverrides,omitempty"`
	CreatedAt              time.Time         `json:"createdAt"`
	UpdatedAt              time.Time         `json:"updatedAt"`
	StartedAt              time.Time         `json:"startedAt"`
	EndedAt                time.Time         `json:"endedAt,omitzero"`
	ApplicationID          string            `json:"applicationId"`
	SessionID              string            `json:"sessionId"`
	Arn                    string            `json:"arn"`
	Name                   string            `json:"name,omitempty"`
	State                  string            `json:"state"`
	StateDetails           string            `json:"stateDetails"`
	CreatedBy              string            `json:"createdBy"`
	ExecutionRoleArn       string            `json:"executionRoleArn"`
	ReleaseLabel           string            `json:"releaseLabel"`
	IdleTimeoutMinutes     int64             `json:"idleTimeoutMinutes,omitempty"`
}

// StartSession creates an interactive session on a running application.
func (b *InMemoryBackend) StartSession(
	applicationID, clientToken, executionRoleArn, name string,
	idleTimeoutMinutes int64, configurationOverrides map[string]any, tags map[string]string,
) (*Session, error) {
	b.mu.Lock("StartSession")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(applicationID)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}
	if app.State != ApplicationStateStarted {
		if !applicationAutoStartEnabled(app) {
			return nil, fmt.Errorf(
				"%w: application %s must be STARTED to start a session and autoStartConfiguration disables implicit start",
				ErrConflict,
				applicationID,
			)
		}

		app.State = ApplicationStateStarted
		app.UpdatedAt = time.Now().UTC()
	}
	if clientToken == "" {
		return nil, fmt.Errorf("%w: clientToken is required", ErrValidation)
	}
	if executionRoleArn == "" {
		return nil, fmt.Errorf("%w: executionRoleArn is required", ErrValidation)
	}
	if session := b.sessionForToken(applicationID, clientToken); session != nil {
		return cloneSession(session), nil
	}

	id := newID()
	now := time.Now().UTC()
	session := &Session{
		ApplicationID:          applicationID,
		SessionID:              id,
		Arn:                    b.sessionARN(applicationID, id),
		Name:                   name,
		State:                  SessionStateStarted,
		StateDetails:           "Session started",
		CreatedBy:              executionRoleArn,
		ExecutionRoleArn:       executionRoleArn,
		ReleaseLabel:           app.ReleaseLabel,
		IdleTimeoutMinutes:     idleTimeoutMinutes,
		ConfigurationOverrides: maps.Clone(configurationOverrides),
		CreatedAt:              now,
		UpdatedAt:              now,
		StartedAt:              now,
		Tags:                   maps.Clone(tags),
	}
	if session.Tags == nil {
		session.Tags = make(map[string]string)
	}
	if b.sessionTokens[applicationID] == nil {
		b.sessionTokens[applicationID] = make(map[string]string)
	}
	b.sessions.Put(session)
	b.sessionTokens[applicationID][clientToken] = id

	return cloneSession(session), nil
}

func (b *InMemoryBackend) sessionForToken(applicationID, clientToken string) *Session {
	sessionID := b.sessionTokens[applicationID][clientToken]
	if sessionID == "" {
		return nil
	}

	session, ok := b.sessions.Get(sessionID)
	if !ok || session.ApplicationID != applicationID {
		return nil
	}

	return session
}

// GetSession retrieves an interactive session.
func (b *InMemoryBackend) GetSession(applicationID, sessionID string) (*Session, error) {
	b.mu.RLock("GetSession")
	defer b.mu.RUnlock()

	session, err := b.lookupSession(applicationID, sessionID)
	if err != nil {
		return nil, err
	}

	return cloneSession(session), nil
}

func (b *InMemoryBackend) lookupSession(applicationID, sessionID string) (*Session, error) {
	if !b.applications.Has(applicationID) {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}
	session, ok := b.sessions.Get(sessionID)
	if !ok || session.ApplicationID != applicationID {
		return nil, fmt.Errorf("%w: session %s not found", ErrNotFound, sessionID)
	}

	return session, nil
}

// ListSessions returns sessions ordered newest first with optional state and time filtering.
func (b *InMemoryBackend) ListSessions(
	applicationID, nextToken string, maxResults int, createdAfter, createdBefore time.Time, states ...string,
) ([]*Session, string, error) {
	b.mu.RLock("ListSessions")
	defer b.mu.RUnlock()

	if !b.applications.Has(applicationID) {
		return nil, "", fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}

	stateSet := make(map[string]struct{}, len(states))
	for _, state := range states {
		stateSet[state] = struct{}{}
	}
	sessionsForApp := b.sessionsByApplication.Get(applicationID)
	list := make([]*Session, 0, len(sessionsForApp))
	for _, session := range sessionsForApp {
		if len(stateSet) > 0 {
			if _, ok := stateSet[session.State]; !ok {
				continue
			}
		}
		if !createdAfter.IsZero() && !session.CreatedAt.After(createdAfter) {
			continue
		}
		if !createdBefore.IsZero() && !session.CreatedAt.Before(createdBefore) {
			continue
		}
		list = append(list, cloneSession(session))
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].SessionID < list[j].SessionID
		}

		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	page, token := emrPaginate(list, nextToken, maxResults)

	return page, token, nil
}

// TerminateSession moves a session to terminal state.
func (b *InMemoryBackend) TerminateSession(applicationID, sessionID string) (*Session, error) {
	b.mu.Lock("TerminateSession")
	defer b.mu.Unlock()

	session, err := b.lookupSession(applicationID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.State == SessionStateTerminated || session.State == SessionStateFailed {
		return nil, fmt.Errorf(
			"%w: session %s cannot be terminated from state %s",
			ErrInvalidState,
			sessionID,
			session.State,
		)
	}

	now := time.Now().UTC()
	session.State = SessionStateTerminated
	session.StateDetails = "Session terminated by user request"
	session.UpdatedAt = now
	session.EndedAt = now

	return cloneSession(session), nil
}

// GetResourceDashboard returns a dashboard URL for a supported session resource.
func (b *InMemoryBackend) GetResourceDashboard(applicationID, resourceID, resourceType string) (string, error) {
	b.mu.RLock("GetResourceDashboard")
	defer b.mu.RUnlock()

	if resourceType != resourceTypeSession {
		return "", fmt.Errorf("%w: resourceType must be SESSION", ErrValidation)
	}
	if _, err := b.lookupSession(applicationID, resourceID); err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"https://console.aws.amazon.com/emr-serverless/home?region=%s#/applications/%s/sessions/%s",
		b.region, applicationID, resourceID,
	), nil
}

// GetSessionEndpoint produces an active session endpoint and expiring token.
func (b *InMemoryBackend) GetSessionEndpoint(applicationID, sessionID string) (string, string, time.Time, error) {
	b.mu.RLock("GetSessionEndpoint")
	defer b.mu.RUnlock()

	session, err := b.lookupSession(applicationID, sessionID)
	if err != nil {
		return "", "", time.Time{}, err
	}
	if session.State != SessionStateStarted && session.State != SessionStateIdle && session.State != SessionStateBusy {
		return "", "", time.Time{}, fmt.Errorf("%w: session %s has no active endpoint", ErrInvalidState, sessionID)
	}

	endpoint := fmt.Sprintf("https://%s.%s.emr-serverless.amazonaws.com", sessionID, b.region)

	return endpoint, newID() + newID(), time.Now().UTC().Add(time.Hour), nil
}

func cloneSession(session *Session) *Session {
	cp := *session
	cp.Tags = maps.Clone(session.Tags)
	cp.ConfigurationOverrides = maps.Clone(session.ConfigurationOverrides)
	if cp.Tags == nil {
		cp.Tags = make(map[string]string)
	}

	return &cp
}
