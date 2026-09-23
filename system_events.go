package main

import (
	"regexp"
	"strings"
)

// EventMessageDetail contains the shared fields exposed by Microsoft Graph's
// concrete eventMessageDetail resource types.
type EventMessageDetail struct {
	ODataType                string        `json:"@odata.type,omitempty"`
	CallEventType            string        `json:"callEventType,omitempty"`
	CallDuration             string        `json:"callDuration,omitempty"`
	CallRecordingDuration    string        `json:"callRecordingDuration,omitempty"`
	CallRecordingDisplayName string        `json:"callRecordingDisplayName,omitempty"`
	CallRecordingStatus      string        `json:"callRecordingStatus,omitempty"`
	CallRecordingURL         string        `json:"callRecordingUrl,omitempty"`
	CallID                   string        `json:"callId,omitempty"`
	CallTranscriptICalUID    string        `json:"callTranscriptICalUid,omitempty"`
	ChatDisplayName          string        `json:"chatDisplayName,omitempty"`
	ChannelDisplayName       string        `json:"channelDisplayName,omitempty"`
	TeamDisplayName          string        `json:"teamDisplayName,omitempty"`
	Initiator                *MessageFrom  `json:"initiator,omitempty"`
	MeetingOrganizer         *MessageFrom  `json:"meetingOrganizer,omitempty"`
	Members                  []MessageUser `json:"members,omitempty"`
}

var (
	eventTypeWordBoundary = regexp.MustCompile(`([[:lower:][:digit:]])([[:upper:]])`)
	isoDurationPattern    = regexp.MustCompile(`^P(?:([0-9]+)D)?T(?:([0-9]+)H)?(?:([0-9]+)M)?(?:([0-9]+(?:\.[0-9]+)?)S)?$`)
)

// IsSystemEvent reports whether the message represents a Teams-generated event.
func (msg *Message) IsSystemEvent() bool {
	if msg == nil {
		return false
	}
	if msg.MessageType == "systemEventMessage" || msg.EventDetail != nil {
		return true
	}
	return msg.Body != nil && msg.Body.Content != nil && *msg.Body.Content == "<systemEventMessage/>"
}

func identityDisplayName(identity *MessageFrom) string {
	if identity == nil {
		return ""
	}
	for _, candidate := range []*MessageUser{identity.User, identity.Application, identity.Device} {
		if candidate != nil && candidate.DisplayName != nil {
			if name := strings.TrimSpace(*candidate.DisplayName); name != "" {
				return name
			}
		}
	}
	return ""
}

// SenderName returns a display label suitable for message headers and exports.
func (msg *Message) SenderName() string {
	if msg == nil {
		return ""
	}
	if sender := identityDisplayName(msg.From); sender != "" {
		return sender
	}
	if msg.IsSystemEvent() {
		return "Teams"
	}
	return ""
}

func (detail *EventMessageDetail) shortType() string {
	if detail == nil {
		return ""
	}
	typeName := strings.TrimPrefix(detail.ODataType, "#")
	if index := strings.LastIndex(typeName, "."); index >= 0 {
		typeName = typeName[index+1:]
	}
	return strings.TrimSuffix(typeName, "EventMessageDetail")
}

func humanizeEventType(eventType string) string {
	if eventType == "" {
		return "System event"
	}
	label := strings.ToLower(eventTypeWordBoundary.ReplaceAllString(eventType, "$1 $2"))
	return strings.ToUpper(label[:1]) + label[1:]
}

func formatISODuration(duration string) string {
	matches := isoDurationPattern.FindStringSubmatch(duration)
	if matches == nil {
		return duration
	}
	suffixes := []string{"d", "h", "m", "s"}
	parts := make([]string, 0, len(suffixes))
	for index, suffix := range suffixes {
		if matches[index+1] != "" {
			parts = append(parts, matches[index+1]+suffix)
		}
	}
	if len(parts) == 0 {
		return duration
	}
	return strings.Join(parts, " ")
}

func eventCallKind(detail *EventMessageDetail) string {
	if detail == nil {
		return "Call"
	}
	switch detail.CallEventType {
	case "meeting":
		return "Meeting"
	case "screenShare":
		return "Screen share"
	default:
		return "Call"
	}
}

func eventMemberNames(detail *EventMessageDetail) []string {
	if detail == nil {
		return nil
	}
	names := make([]string, 0, len(detail.Members))
	seen := make(map[string]bool)
	for _, member := range detail.Members {
		if member.DisplayName == nil {
			continue
		}
		name := strings.TrimSpace(*member.DisplayName)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// SystemEventSummary converts Graph eventDetail metadata into readable text.
func (msg *Message) SystemEventSummary() string {
	if msg == nil {
		return "System event"
	}
	detail := msg.EventDetail
	eventType := detail.shortType()
	callKind := eventCallKind(detail)
	base := ""
	switch eventType {
	case "callStarted":
		base = callKind + " started"
	case "callEnded":
		base = callKind + " ended"
	case "callRecording":
		base = callKind + " recording available"
	case "callTranscript":
		base = callKind + " transcript available"
	case "chatRenamed":
		base = "Chat renamed"
	case "channelRenamed":
		base = "Channel renamed"
	case "teamRenamed":
		base = "Team renamed"
	case "membersAdded":
		base = "Members added"
	case "membersDeleted":
		base = "Members removed"
	case "membersJoined":
		base = "Members joined"
	case "membersLeft":
		base = "Members left"
	case "messagePinned":
		base = "Message pinned"
	case "messageUnpinned":
		base = "Message unpinned"
	default:
		base = humanizeEventType(eventType)
	}

	if names := eventMemberNames(detail); len(names) > 0 {
		switch eventType {
		case "membersAdded", "membersDeleted", "membersJoined", "membersLeft":
			base += ": " + strings.Join(names, ", ")
		}
	}

	if detail != nil {
		initiator := identityDisplayName(detail.Initiator)
		if initiator != "" {
			switch eventType {
			case "callStarted", "chatRenamed", "channelRenamed", "teamRenamed":
				base += " by " + initiator
			}
		}
		if eventType == "callEnded" && detail.CallDuration != "" {
			base += " (" + formatISODuration(detail.CallDuration) + ")"
		}
		switch eventType {
		case "chatRenamed":
			if detail.ChatDisplayName != "" {
				base += ": " + detail.ChatDisplayName
			}
		case "channelRenamed":
			if detail.ChannelDisplayName != "" {
				base += ": " + detail.ChannelDisplayName
			}
		case "teamRenamed":
			if detail.TeamDisplayName != "" {
				base += ": " + detail.TeamDisplayName
			}
		}
	}

	if eventType == "" {
		for _, fallback := range []string{msg.Subject, msg.Summary} {
			if fallback = strings.TrimSpace(fallback); fallback != "" {
				return fallback
			}
		}
	}
	return base
}
