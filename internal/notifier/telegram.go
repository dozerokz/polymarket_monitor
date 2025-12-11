package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	SendMessageEndpoint = "https://api.telegram.org/bot%s/sendMessage"
	clientTimeOut       = 30 * time.Second
)

// TgNotifier is structure used for telegram notifications
type TgNotifier struct {
	token  string
	chatID string
	client *http.Client
}

// NewTgNotifier creates TgNotifier
func NewTgNotifier(token string, chatID string) *TgNotifier {

	return &TgNotifier{
		token:  token,
		chatID: chatID,
		client: &http.Client{
			Timeout: 30 * time.Second},
	}
}

// Notify used for sending message string to telegram chat
func (n *TgNotifier) Notify(message string) error {
	payload := map[string]string{
		"text":       message,
		"chat_id":    n.chatID,
		"parse_mode": "HTML",
	}

	jsonValue, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request payload: %v", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf(SendMessageEndpoint, n.token), bytes.NewBuffer(jsonValue))
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
