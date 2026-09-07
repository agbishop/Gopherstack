package azureservicebus

import (
	"encoding/xml"
	"strconv"
)

// This file implements two directions of Service Bus's Atom+XML management
// wire format: parsing a create-request body (CreateQueue/CreateTopic/
// CreateSubscription) to determine entity kind and EntityConfig properties,
// and serializing Get/List responses in the same shape. See PARITY.md's
// entity_kind_detection and Get/List fidelity notes.

// Real Service Bus's management REST API is seen in the wild under two
// different XML namespace URIs for the entity description elements
// (QueueDescription/TopicDescription/SubscriptionDescription):
// ".../netservices/2010/10/servicebus/connect" and the older
// ".../servicebus/2010/10/". Rather than picking one and rejecting the
// other, parsing below matches purely on local element name (an
// encoding/xml struct field tag with no namespace prefix, e.g. `xml:
// "QueueDescription"`, matches that local name in any namespace) --
// deliberately tolerant of either. sbDescriptionNS is used only when this
// package itself serializes a response (Get/List), where it must commit to
// one namespace to emit.
const sbDescriptionNS = "http://schemas.microsoft.com/netservices/2010/10/servicebus/connect"

// atomNS is the Atom namespace URI real Service Bus's entry/feed envelopes
// declare.
const atomNS = "http://www.w3.org/2005/Atom"

// Content-Type values real Service Bus emits for a single Atom entry versus
// a feed of entries.
const (
	atomEntryContentType = "application/atom+xml;type=entry;charset=utf-8"
	atomFeedContentType  = "application/atom+xml;type=feed;charset=utf-8"
	// descriptionContentType is the value real Service Bus's <content type="..">
	// attribute carries for an entity description payload.
	descriptionContentType = "application/xml"
)

// ---- parsing (CreateQueue/CreateTopic/CreateSubscription request bodies) ----

// atomEntryIn is the minimal shape this package parses out of a Service Bus
// management create-request body:
//
//	<entry xmlns="...Atom...">
//	  <content type="application/xml">
//	    <QueueDescription xmlns="...">...</QueueDescription>
//	  </content>
//	</entry>
//
// (or <TopicDescription>/<SubscriptionDescription> in place of
// <QueueDescription>). Any <RuleDescription>/<SqlFilter> nested under a
// SubscriptionDescription is not modeled here at all -- encoding/xml simply
// ignores XML it has no matching struct field for, which is exactly the
// "accept and discard" behavior CreateSubscription's doc comment describes.
type atomEntryIn struct {
	Content struct {
		QueueDescription        *entityDescriptionIn `xml:"QueueDescription"`
		TopicDescription        *entityDescriptionIn `xml:"TopicDescription"`
		SubscriptionDescription *entityDescriptionIn `xml:"SubscriptionDescription"`
	} `xml:"content"`
	XMLName xml.Name `xml:"entry"`
}

// entityDescriptionIn holds the subset of a QueueDescription/
// TopicDescription/SubscriptionDescription element's children this MVP
// understands. Fields are strings (not time.Duration/int) because a missing
// or malformed value must fall back to the package default rather than fail
// the whole parse -- see toConfig.
type entityDescriptionIn struct {
	LockDuration             string `xml:"LockDuration"`
	MaxDeliveryCount         string `xml:"MaxDeliveryCount"`
	DefaultMessageTimeToLive string `xml:"DefaultMessageTimeToLive"`
}

// toConfig converts d's string fields to an EntityConfig, silently leaving a
// field zero-valued (i.e. falling back to the package default -- see
// EntityConfig) if it is absent or fails to parse. A nil d (element not
// present at all) yields a zero-valued EntityConfig the same way.
func (d *entityDescriptionIn) toConfig() EntityConfig {
	var cfg EntityConfig
	if d == nil {
		return cfg
	}

	if d.LockDuration != "" {
		if dur, err := parseISO8601Duration(d.LockDuration); err == nil {
			cfg.LockDuration = dur
		}
	}

	if d.DefaultMessageTimeToLive != "" {
		if dur, err := parseISO8601Duration(d.DefaultMessageTimeToLive); err == nil {
			cfg.DefaultMessageTTL = dur
		}
	}

	if d.MaxDeliveryCount != "" {
		if n, err := strconv.Atoi(d.MaxDeliveryCount); err == nil {
			cfg.MaxDeliveryCount = n
		}
	}

	return cfg
}

// entityKind identifies which Service Bus entity description an Atom+XML
// create-request body's <content> held.
type entityKind int

const (
	entityKindUnknown entityKind = iota
	entityKindQueue
	entityKindTopic
	entityKindSubscription
)

// parsedEntityBody is the result of successfully Atom+XML-parsing a
// create-request body.
type parsedEntityBody struct {
	Kind   entityKind
	Config EntityConfig
}

// parseAtomEntityBody attempts a full Atom+XML parse of body, returning
// ok=false for anything that isn't a well-formed entry with exactly one of
// QueueDescription/TopicDescription/SubscriptionDescription under <content>
// -- including plain malformed XML, which must NOT fail the caller's create
// operation (see handler.go's resolveEntityKind, which falls through to the
// substring-sniff heuristic and then to defaulting to a queue).
func parseAtomEntityBody(body []byte) (parsedEntityBody, bool) {
	var entry atomEntryIn

	if err := xml.Unmarshal(body, &entry); err != nil {
		return parsedEntityBody{}, false
	}

	switch {
	case entry.Content.QueueDescription != nil:
		return parsedEntityBody{Kind: entityKindQueue, Config: entry.Content.QueueDescription.toConfig()}, true
	case entry.Content.TopicDescription != nil:
		return parsedEntityBody{Kind: entityKindTopic, Config: entry.Content.TopicDescription.toConfig()}, true
	case entry.Content.SubscriptionDescription != nil:
		return parsedEntityBody{
			Kind: entityKindSubscription, Config: entry.Content.SubscriptionDescription.toConfig(),
		}, true
	default:
		return parsedEntityBody{}, false
	}
}

// ---- serialization (Get/List responses) ----

// queueDescriptionOut/topicDescriptionOut/subscriptionDescriptionOut are the
// response-side counterparts of entityDescriptionIn: real Service Bus's
// element shape, with duration fields formatted back to ISO 8601 (see
// formatISO8601Duration). Fidelity bar: this mirrors services/azurequeue's
// and services/azuretable's List responses (a flat, un-paginated listing of
// simple per-entity fields) rather than every property real Service Bus's
// full QueueDescription/TopicDescription/SubscriptionDescription can carry
// -- see PARITY.md's Get/List fidelity note.
type queueDescriptionOut struct {
	Xmlns                    string `xml:"xmlns,attr"`
	LockDuration             string `xml:"LockDuration"`
	DefaultMessageTimeToLive string `xml:"DefaultMessageTimeToLive"`
	MaxDeliveryCount         int    `xml:"MaxDeliveryCount"`
}

type topicDescriptionOut struct {
	Xmlns                    string `xml:"xmlns,attr"`
	DefaultMessageTimeToLive string `xml:"DefaultMessageTimeToLive"`
}

type subscriptionDescriptionOut struct {
	Xmlns            string `xml:"xmlns,attr"`
	LockDuration     string `xml:"LockDuration"`
	MaxDeliveryCount int    `xml:"MaxDeliveryCount"`
}

// atomContentOut wraps exactly one *Description in the "application/xml"
// content element real Service Bus uses for the entity description payload.
type atomContentOut struct {
	QueueDescription        *queueDescriptionOut        `xml:"QueueDescription,omitempty"`
	TopicDescription        *topicDescriptionOut        `xml:"TopicDescription,omitempty"`
	SubscriptionDescription *subscriptionDescriptionOut `xml:"SubscriptionDescription,omitempty"`
	Type                    string                      `xml:"type,attr"`
}

// atomEntryOut is one Get response, or one item of a List (feed) response.
type atomEntryOut struct {
	Content atomContentOut `xml:"content"`
	XMLName xml.Name       `xml:"entry"`
	Xmlns   string         `xml:"xmlns,attr"`
	Title   string         `xml:"title"`
}

// atomFeedOut is a List response: real Service Bus's $Resources/Queues and
// $Resources/Topics shape, and this MVP's addition of
// <topic>/subscriptions.
type atomFeedOut struct {
	XMLName xml.Name       `xml:"feed"`
	Xmlns   string         `xml:"xmlns,attr"`
	Title   string         `xml:"title"`
	Entries []atomEntryOut `xml:"entry"`
}

func queueEntryXML(info QueueInfo) atomEntryOut {
	return atomEntryOut{
		Xmlns: atomNS,
		Title: info.Name,
		Content: atomContentOut{
			Type: descriptionContentType,
			QueueDescription: &queueDescriptionOut{
				Xmlns:                    sbDescriptionNS,
				LockDuration:             formatISO8601Duration(info.LockDuration),
				MaxDeliveryCount:         info.MaxDeliveryCount,
				DefaultMessageTimeToLive: formatISO8601Duration(info.DefaultMessageTTL),
			},
		},
	}
}

func topicEntryXML(info TopicInfo) atomEntryOut {
	return atomEntryOut{
		Xmlns: atomNS,
		Title: info.Name,
		Content: atomContentOut{
			Type: descriptionContentType,
			TopicDescription: &topicDescriptionOut{
				Xmlns:                    sbDescriptionNS,
				DefaultMessageTimeToLive: formatISO8601Duration(info.DefaultMessageTTL),
			},
		},
	}
}

func subscriptionEntryXML(info SubscriptionInfo) atomEntryOut {
	return atomEntryOut{
		Xmlns: atomNS,
		Title: info.Name,
		Content: atomContentOut{
			Type: descriptionContentType,
			SubscriptionDescription: &subscriptionDescriptionOut{
				Xmlns:            sbDescriptionNS,
				LockDuration:     formatISO8601Duration(info.LockDuration),
				MaxDeliveryCount: info.MaxDeliveryCount,
			},
		},
	}
}

func queueFeedXML(title string, infos []QueueInfo) atomFeedOut {
	entries := make([]atomEntryOut, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, queueEntryXML(info))
	}

	return atomFeedOut{Xmlns: atomNS, Title: title, Entries: entries}
}

func topicFeedXML(title string, infos []TopicInfo) atomFeedOut {
	entries := make([]atomEntryOut, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, topicEntryXML(info))
	}

	return atomFeedOut{Xmlns: atomNS, Title: title, Entries: entries}
}

func subscriptionFeedXML(title string, infos []SubscriptionInfo) atomFeedOut {
	entries := make([]atomEntryOut, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, subscriptionEntryXML(info))
	}

	return atomFeedOut{Xmlns: atomNS, Title: title, Entries: entries}
}
