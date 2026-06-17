package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	clientTimeOut = 30 * time.Second
)

var sendMessageEndpoint = "https://api.telegram.org/bot%s/sendMessage"

// TgNotifier is structure used for telegram notifications
type TgNotifier struct {
	token   string
	chatID  string
	topicID *int64
	client  *http.Client
}

// NewTgNotifier creates TgNotifier
func NewTgNotifier(token string, chatID string, topicID *int64) *TgNotifier {

	return &TgNotifier{
		token:   token,
		chatID:  chatID,
		topicID: topicID,
		client: &http.Client{
			Timeout: clientTimeOut},
	}
}

type sendMessagePayload struct {
	Text            string `json:"text"`
	ChatID          string `json:"chat_id"`
	ParseMode       string `json:"parse_mode"`
	MessageThreadID *int64 `json:"message_thread_id,omitempty"`
}

// Notify used for sending message string to telegram chat
func (n *TgNotifier) Notify(message string) error {
	payload := sendMessagePayload{
		Text:            message,
		ChatID:          n.chatID,
		ParseMode:       "HTML",
		MessageThreadID: n.topicID,
	}

	jsonValue, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request payload: %v", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf(sendMessageEndpoint, n.token), bytes.NewBuffer(jsonValue))
	if err != nil {
		return fmt.Errorf("failed to create telegram post message request: %v", err)
	}

	req.Header.Add("accept", "application/json")
	req.Header.Add("content-type", "application/json")

	res, err := n.client.Do(req)

	if err != nil {
		return fmt.Errorf("post telegram message request failed: %v", err)
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("post telegram message response != 200 | %s", res.Status)
	}
	return nil
}
