package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSystemEventSummariesFromGraphPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name: "meeting started",
			payload: `{
                "messageType": "systemEventMessage",
                "body": {"content": "<systemEventMessage/>"},
                "eventDetail": {
                    "@odata.type": "#microsoft.graph.callStartedEventMessageDetail",
                    "callEventType": "meeting",
                    "initiator": {"user": {"displayName": "Ada Lovelace"}}
                }
            }`,
			want: "Meeting started by Ada Lovelace",
		},
		{
			name: "meeting ended with duration",
			payload: `{
                "messageType": "systemEventMessage",
                "eventDetail": {
                    "@odata.type": "#microsoft.graph.callEndedEventMessageDetail",
                    "callEventType": "meeting",
                    "callDuration": "PT23M15S"
                }
            }`,
			want: "Meeting ended (23m 15s)",
		},
		{
			name: "recording available",
			payload: `{
                "messageType": "systemEventMessage",
                "eventDetail": {
                    "@odata.type": "#microsoft.graph.callRecordingEventMessageDetail",
                    "callRecordingStatus": "success"
                }
            }`,
			want: "Call recording available",
		},
		{
			name: "transcript available",
			payload: `{
                "messageType": "systemEventMessage",
                "eventDetail": {
                    "@odata.type": "#microsoft.graph.callTranscriptEventMessageDetail"
                }
            }`,
			want: "Call transcript available",
		},
		{
			name: "members added",
			payload: `{
                "messageType": "systemEventMessage",
                "eventDetail": {
                    "@odata.type": "#microsoft.graph.membersAddedEventMessageDetail",
                    "members": [
                        {"displayName": "Grace Hopper"},
                        {"displayName": "Alan Turing"}
                    ]
                }
            }`,
			want: "Members added: Grace Hopper, Alan Turing",
		},
		{
			name: "unknown future event",
			payload: `{
                "messageType": "systemEventMessage",
                "eventDetail": {
                    "@odata.type": "#microsoft.graph.teamDescriptionUpdatedEventMessageDetail"
                }
            }`,
			want: "Team description updated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var message Message
			if err := json.Unmarshal([]byte(test.payload), &message); err != nil {
				t.Fatalf("unmarshal Graph message: %v", err)
			}
			if got := message.GetPlainText(); got != test.want {
				t.Fatalf("GetPlainText() = %q, want %q", got, test.want)
			}
			if sender := message.SenderName(); sender != "Teams" {
				t.Fatalf("SenderName() = %q, want Teams", sender)
			}
		})
	}
}

func TestSystemEventFallbackUsesGraphSummary(t *testing.T) {
	message := Message{MessageType: "systemEventMessage", Summary: "A Teams policy changed"}
	if got := message.GetPlainText(); got != "A Teams policy changed" {
		t.Fatalf("GetPlainText() = %q, want Graph summary", got)
	}
}

func TestSystemEventRendersWithTeamsHeaderAndConcreteText(t *testing.T) {
	app := NewApp()
	app.Messages = []Message{{
		ID:              "event-1",
		CreatedDateTime: "2026-08-02T12:00:00Z",
		MessageType:     "systemEventMessage",
		EventDetail: &EventMessageDetail{
			ODataType:     "#microsoft.graph.callStartedEventMessageDetail",
			CallEventType: "meeting",
		},
	}}
	model := NewModel(app, "client", "user")
	rendered := stripANSI(model.renderMessages(80, 20))
	for _, want := range []string{"Teams", "Meeting started"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered system event missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "[system event]") {
		t.Fatalf("render retained generic system-event marker:\n%s", rendered)
	}
}

func TestMessagesEqualDetectsEventDetailUpgrade(t *testing.T) {
	body := "<systemEventMessage/>"
	before := []Message{{
		ID:          "event-1",
		MessageType: "systemEventMessage",
		Body:        &MessageBody{Content: &body},
	}}
	after := []Message{{
		ID:          "event-1",
		MessageType: "systemEventMessage",
		Body:        &MessageBody{Content: &body},
		EventDetail: &EventMessageDetail{ODataType: "#microsoft.graph.callStartedEventMessageDetail"},
	}}
	if messagesEqual(before, after) {
		t.Fatal("messagesEqual ignored newly available eventDetail metadata")
	}
}
