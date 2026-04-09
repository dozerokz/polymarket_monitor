package parser

import (
	"encoding/json"
	"fmt"
	"github.com/dozerokz/logger"
	"io"
	"net/http"
	"net/url"
	"polymarket_monitor/internal/notifier"
	"slices"
	"strings"
	"time"
)

const (
	activityLimit     = 20
	activityEndpoint  = "https://data-api.polymarket.com/activity?limit=%d&sortBy=TIMESTAMP&sortDirection=DESC&user=%s"
	eventURL          = "https://polymarket.com/event/"
	profileURL        = "https://polymarket.com/@"
	monitorSleepDelay = 5 * time.Second
	monitorErrorDelay = 15 * time.Second
	httpTimeout       = 30 * time.Second
)

var httpClient = &http.Client{Timeout: httpTimeout}

// cache used to store wallets activity
var cache = map[string][]string{}

// initialized marks wallets that were warmed up (to avoid spamming old activity after transient init errors).
var initialized = map[string]bool{}

func normalizeTransactionHash(hash string) string {
	return strings.ToLower(strings.TrimSpace(hash))
}

// getActivity gets last activity events for wallet address
func getActivity(wallet string) ([]activityResponse, error) {
	var userActivity []activityResponse

	resp, err := httpClient.Get(fmt.Sprintf(activityEndpoint, activityLimit, url.QueryEscape(wallet)))
	if err != nil {
		return userActivity, fmt.Errorf("failed to make request for user '%s' activity: %w", wallet, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return userActivity, fmt.Errorf("activity request for user '%s' failed: %s | %s", wallet, resp.Status, string(body))
	}

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

// initMonitor saves wallets activity to cache to avoid notifications for old events.
// If a wallet fails to init (network/API error), it will be warmed up later in the monitor loop.
func initMonitor(wallets []string, log *logger.Logger) (int, error) {
	if len(wallets) == 0 {
		return 0, fmt.Errorf("wallets list is empty")
	}

	initializedCount := 0
	for _, wallet := range wallets {
		cache[wallet] = make([]string, 0, activityLimit)
		initialized[wallet] = false

		activity, err := getActivity(wallet)
		if err != nil {
			log.Error("Failed to get wallet %s activity during init: %v", wallet, err)
			continue
		}
		// Iterate from oldest to newest so that the newest transaction ends up at the front of the cache.
		for i := len(activity) - 1; i >= 0; i-- {
			tx := activity[i]
			hash := normalizeTransactionHash(tx.TransactionHash)
			if hash == "" {
				continue
			}
			cache[wallet] = addToCache(cache[wallet], hash, activityLimit)
		}
		initialized[wallet] = true
		initializedCount++
	}

	return initializedCount, nil
}

// Monitor is main monitoring function.
// Tracking wallets activity, comparing to previously saved, sending notification to telegram if new activity detected
func Monitor(wallets []string, tgNotifier *notifier.TgNotifier, log *logger.Logger) {
	initializedCount, initErr := initMonitor(wallets, log)
	if initErr != nil {
		log.Error("%v", initErr)
		return
	}

	log.Info("Initialized successfully %d/%d wallets", initializedCount, len(wallets))

	nextFetchAt := make(map[string]time.Time, len(wallets))

	for {
		now := time.Now()

		for _, wallet := range wallets {
			if t := nextFetchAt[wallet]; !t.IsZero() && now.Before(t) {
				continue
			}

			activity, err := getActivity(wallet)
			if err != nil {
				log.Error("Error while getting wallet %s activity: %v | next attempt in %v",
					wallet, err, monitorErrorDelay)
				nextFetchAt[wallet] = time.Now().Add(monitorErrorDelay)
				continue
			}

			nextFetchAt[wallet] = time.Time{}

			// Warm up wallets that failed init to prevent spamming old activity.
			if !initialized[wallet] {
				cache[wallet] = cache[wallet][:0]
				// Iterate from oldest to newest so that the newest transaction ends up at the front of the cache.
				for i := len(activity) - 1; i >= 0; i-- {
					tx := activity[i]
					hash := normalizeTransactionHash(tx.TransactionHash)
					if hash == "" {
						continue
					}
					cache[wallet] = addToCache(cache[wallet], hash, activityLimit)
				}
				initialized[wallet] = true
				log.Info("Initialized wallet %s", wallet)
				continue
			}

			// Process from oldest to newest to keep cache eviction consistent and notifications chronological.
			for i := len(activity) - 1; i >= 0; i-- {
				event := activity[i]
				hash := normalizeTransactionHash(event.TransactionHash)
				if hash == "" {
					continue
				}
				if !slices.Contains(cache[wallet], hash) {
					message := buildNotifierMessage(event)
					if message != "" {
						err = tgNotifier.Notify(message)
						if err != nil {
							log.Error("Error while sending message to telegram: %v", err)
						}
						log.Debug("cache: %v | activity resp: %v", cache[wallet], activity)
						log.Info("Sent notification successfully")
					}
					cache[wallet] = addToCache(cache[wallet], hash, activityLimit)
				} else {
					continue
				}
			}
		}
		time.Sleep(monitorSleepDelay)
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

// addToCache prepends a new value and caps cache size to maxSize.
// If the value already exists, it is moved to the front.
func addToCache[T comparable](s []T, v T, maxSize int) []T {
	if maxSize <= 0 {
		return s
	}

	if idx := slices.Index(s, v); idx != -1 {
		s = append(s[:idx], s[idx+1:]...)
	}

	s = append([]T{v}, s...)
	if len(s) > maxSize {
		s = s[:maxSize]
	}
	return s
}
