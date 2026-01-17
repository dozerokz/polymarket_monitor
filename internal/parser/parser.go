package parser

import (
	"encoding/json"
	"fmt"
	"github.com/dozerokz/logger"
	"io"
	"net/http"
	"polymarket_monitor/internal/notifier"
	"slices"
	"time"
)

const (
	activityEndpoint  = "https://data-api.polymarket.com/activity?limit=20&sortBy=TIMESTAMP&sortDirection=DESC&user=%s"
	eventURL          = "https://polymarket.com/event/"
	profileURL        = "https://polymarket.com/@"
	monitorSleepDelay = 5 * time.Second
	monitorErrorDelay = 15 * time.Second
)

// cache used to store wallets activity
var cache = map[string][]string{}

// getActivity gets last 25 activity events for wallet address
func getActivity(wallet string) ([]activityResponse, error) {
	var userActivity []activityResponse

	resp, err := http.Get(fmt.Sprintf(activityEndpoint, wallet))
	if err != nil {
		return userActivity, fmt.Errorf("failed to make request for user '%s' activity: %w", wallet, err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return userActivity, fmt.Errorf("failed to read response body: %w", err)
	}

	err = json.Unmarshal(body, &userActivity)
	if err != nil {
		return userActivity, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return userActivity, nil
}

// initMonitor saves wallets activity to map to avoid notifications for old events
func initMonitor(wallets []string, log *logger.Logger) error {
	for _, wallet := range wallets {
		activity, err := getActivity(wallet)
		if err != nil {
			log.Error("Failed to get wallet %s activity: %v", wallet, err)
			continue
		}
		for _, tx := range activity {
			cache[wallet] = append(cache[wallet], tx.TransactionHash)
		}
	}

	if len(cache) == 0 {
		return fmt.Errorf("failed to initialize monitor. all wallets have no activity")
	}

	return nil
}

// Monitor is main monitoring function.
// Tracking wallets activity, comparing to previously saved, sending notification to telegram if new activity detected
func Monitor(wallets []string, tgNotifier *notifier.TgNotifier, log *logger.Logger) {
	initErr := initMonitor(wallets, log)
	if initErr != nil {
		log.Error("%v", initErr)
		return
	}

	log.Info("Initialized successfully %d wallets", len(cache))

	for {
		for _, wallet := range wallets {
			activity, err := getActivity(wallet)
			if err != nil {
				log.Error("Error while getting wallet %s activity: %v | sleeping %v",
					wallet, err, monitorErrorDelay)
				time.Sleep(monitorErrorDelay)
				continue
			}

			for _, event := range activity {
				if !slices.Contains(cache[wallet], event.TransactionHash) {
					message := buildNotifierMessage(event)
					if message != "" {
						err = tgNotifier.Notify(message)
						if err != nil {
							log.Error("Error while sending message to telegram: %v", err)
						}
						log.Debug("cache: %v | activity resp: %v", cache[wallet], activity)
						log.Info("Sent notification successfully")
					}
					cache[wallet] = addToCache(cache[wallet], event.TransactionHash)
				} else {
					continue
				}
			}
		}
	}
}

// buildNotifierMessage creating formatted message for telegram
func buildNotifierMessage(event activityResponse) string {
	var message string

	if event.Type == "REWARD" {
		return message
	}

	if event.Type == "TRADE" && event.Side == "BUY" {
		message = fmt.Sprintf(
			"<b>New Polymarket Prediction By <a href=\"%s%s\">@%s</a></b>\n\n"+
				"<b>%s</b>\n\n"+
				"<b>Bought</b> %.1f of <b>%s</b> \n"+
				"Price: %.0f¢ \n"+
				"Total: $%.2f\n\n"+
				"[<a href=\"%s/%s/%s\">View on Polymarket</a>]",
			profileURL, event.Name, event.Name, event.Title, event.Size, event.Outcome, event.Price*100, event.UsdcSize,
			eventURL, event.EventSlug, event.Slug)
	}
	if event.Type == "TRADE" && event.Side == "SELL" {
		message = fmt.Sprintf(
			"<b>New Polymarket Prediction By <a href=\"%s%s\">@%s</a></b>\n\n"+
				"<b>%s</b>\n\n"+
				"<b>Sold</b> %.1f of <b>%s</b> \n"+
				"Price: %.0f¢ \n"+
				"Total: $%.2f\n\n"+
				"[<a href=\"%s/%s/%s\">View on Polymarket</a>]",
			profileURL, event.Name, event.Name, event.Title, event.Size, event.Outcome, event.Price*100, event.UsdcSize,
			eventURL, event.EventSlug, event.Slug)
	}
	return message
}

// addToCache removes the last element of cache[wallet] and prepends a new value,
// keeping the cache size constant.
func addToCache[T any](s []T, v T) []T {
	if len(s) == 0 {
		return s
	}
	s = s[:len(s)-1]         // delete last
	s = append([]T{v}, s...) // add to beginning
	return s
}
