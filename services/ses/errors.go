package ses

import "errors"

// Errors returned by the SES backend.
//
// ErrTrackingOptionsNotFound / ErrTrackingOptionsExists deliberately carry the
// "Exception"-suffixed wire error codes (TrackingOptionsDoesNotExistException /
// TrackingOptionsAlreadyExistsException) even though every sibling *DoesNotExist
// / *AlreadyExists error in this list omits the suffix -- this asymmetry is not
// a typo, it is what aws-sdk-go-v2/service/ses/types/errors.go's
// TrackingOptions{DoesNotExist,AlreadyExists}Exception.ErrorCode() literally
// returns, confirmed against the SDK's deserializers.go error-code switch
// (case strings.EqualFold("TrackingOptionsDoesNotExistException", errorCode)).
// Sending the unsuffixed form (as this file did before this pass) causes a
// real AWS SDK client's error deserializer to miss the typed-exception match.
var (
	ErrEmailNotFound               = errors.New("EmailNotFound")
	ErrInvalidParameter            = errors.New("InvalidParameterValue")
	ErrInvalidPolicy               = errors.New("InvalidPolicy")
	ErrMessageRejected             = errors.New("MessageRejected")
	ErrTemplateNotFound            = errors.New("TemplateDoesNotExist")
	ErrTemplateExists              = errors.New("AlreadyExists")
	ErrConfigSetNotFound           = errors.New("ConfigurationSetDoesNotExist")
	ErrConfigSetExists             = errors.New("ConfigurationSetAlreadyExists")
	ErrReceiptRuleSetNotFound      = errors.New("RuleSetDoesNotExist")
	ErrReceiptRuleSetExists        = errors.New("AlreadyExists")
	ErrReceiptRuleSetActive        = errors.New("CannotDelete")
	ErrReceiptRuleNotFound         = errors.New("RuleDoesNotExist")
	ErrReceiptRuleExists           = errors.New("AlreadyExists")
	ErrReceiptFilterExists         = errors.New("AlreadyExists")
	ErrEventDestinationNotFound    = errors.New("EventDestinationDoesNotExist")
	ErrEventDestinationExists      = errors.New("EventDestinationAlreadyExists")
	ErrTrackingOptionsNotFound     = errors.New("TrackingOptionsDoesNotExistException")
	ErrTrackingOptionsExists       = errors.New("TrackingOptionsAlreadyExistsException")
	ErrCustomVerifTemplateNotFound = errors.New("CustomVerificationEmailTemplateDoesNotExist")
	ErrCustomVerifTemplateExists   = errors.New("CustomVerificationEmailTemplateAlreadyExists")
	ErrValidation                  = errors.New("ValidationError")
	// ErrAccountSendingPaused is returned by send operations when account-level
	// sending has been paused via UpdateAccountSendingEnabled(false), matching
	// real AWS SES's AccountSendingPausedException.
	ErrAccountSendingPaused = errors.New("AccountSendingPausedException")
)
