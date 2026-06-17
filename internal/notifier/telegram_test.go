package notifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifyOmitsMessageThreadIDWhenTopicIsNotConfigured(t *testing.T) {
	originalEndpoint := sendMessageEndpoint
	defer func() {
		sendMessageEndpoint = originalEndpoint
	}()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sendMessageEndpoint = server.URL + "/bot%s/sendMessage"

	notifier := NewTgNotifier("token", "chat-id", nil)
	if err := notifier.Notify("hello"); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if gotPayload["chat_id"] != "chat-id" {
		t.Fatalf("expected chat_id to be %q, got %v", "chat-id", gotPayload["chat_id"])
	}

	if _, ok := gotPayload["message_thread_id"]; ok {
		t.Fatalf("message_thread_id should be omitted when topic is not configured")
	}
}

func TestNotifyIncludesMessageThreadIDWhenTopicIsConfigured(t *testing.T) {
	originalEndpoint := sendMessageEndpoint
	defer func() {
		sendMessageEndpoint = originalEndpoint
	}()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sendMessageEndpoint = server.URL + "/bot%s/sendMessage"

	topicID := int64(42)
	notifier := NewTgNotifier("token", "chat-id", &topicID)
	if err := notifier.Notify("hello"); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if gotPayload["message_thread_id"] != float64(topicID) {
		t.Fatalf("expected message_thread_id to be %v, got %v", topicID, gotPayload["message_thread_id"])
	}
}
