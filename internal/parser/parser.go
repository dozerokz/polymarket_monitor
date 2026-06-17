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

var gammaEventEndpoint = "https://gamma-api.polymarket.com/events/slug/%s"

var blacklistedEventTags []string

// cache used to store wallets activity
var cache = map[string][]string{}

// eventDetailsCache keeps gamma-api lookups by event slug to avoid repeating tag requests.
var eventDetailsCache = map[string]eventDetailsResponse{}

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

func getEventDetails(eventSlug string) (eventDetailsResponse, error) {
	eventSlug = strings.TrimSpace(eventSlug)
	if eventSlug == "" {
		return eventDetailsResponse{}, fmt.Errorf("event slug is empty")
	}

	if eventDetails, ok := eventDetailsCache[eventSlug]; ok {
		return eventDetails, nil
	}

	endpoint := fmt.Sprintf(gammaEventEndpoint, url.PathEscape(eventSlug))
	resp, err := httpClient.Get(endpoint)
	if err != nil {
		return eventDetailsResponse{}, fmt.Errorf("failed to make request for event '%s': %w", eventSlug, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return eventDetailsResponse{}, fmt.Errorf("event request for '%s' failed: %s | %s", eventSlug, resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return eventDetailsResponse{}, fmt.Errorf("failed to read event response body: %w", err)
	}

	var eventDetails eventDetailsResponse
	if err = json.Unmarshal(body, &eventDetails); err != nil {
		return eventDetailsResponse{}, fmt.Errorf("failed to unmarshal event response: %w", err)
	}

	eventDetailsCache[eventSlug] = eventDetails
	return eventDetails, nil
}

func normalizeTagIdentifier(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

func SetBlacklistedEventTags(tags []string) {
	blacklistedEventTags = append([]string(nil), tags...)
}

func findBlacklistedEventTags(tags []eventTagResponse) []string {
	if len(blacklistedEventTags) == 0 {
		return nil
	}

	var matchedTags []string
	seen := make(map[string]struct{}, len(blacklistedEventTags))

	for _, tag := range tags {
		for _, identifier := range []string{tag.Slug, tag.Label} {
			normalizedIdentifier := normalizeTagIdentifier(identifier)
			if normalizedIdentifier == "" || !slices.Contains(blacklistedEventTags, normalizedIdentifier) {
				continue
			}
			if _, ok := seen[normalizedIdentifier]; ok {
				continue
			}

			seen[normalizedIdentifier] = struct{}{}
			matchedTags = append(matchedTags, normalizedIdentifier)
		}
	}

	return matchedTags
}

func eventLookupSlug(event activityResponse) string {
	if strings.TrimSpace(event.EventSlug) != "" {
		return event.EventSlug
	}

	return event.Slug
}

func shouldSkipNotificationByTags(event activityResponse) (bool, []string, error) {
	eventDetails, err := getEventDetails(eventLookupSlug(event))
	if err != nil {
		return false, nil, err
	}

	matchedTags := findBlacklistedEventTags(eventDetails.Tags)
	return len(matchedTags) > 0, matchedTags, nil
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
					if message == "" {
						cache[wallet] = addToCache(cache[wallet], hash, activityLimit)
						continue
					}

					shouldSkip, matchedTags, err := shouldSkipNotificationByTags(event)
					if err != nil {
						log.Error("Failed to resolve tags for event %s (tx %s): %v | notification will be sent anyway",
							eventLookupSlug(event), hash, err)
					} else if shouldSkip {
						log.Info("Skipped notification for wallet %s, tx %s, event %s due to blacklisted tags: %s",
							wallet, hash, eventLookupSlug(event), strings.Join(matchedTags, ", "))
						cache[wallet] = addToCache(cache[wallet], hash, activityLimit)
						continue
					}

					err = tgNotifier.Notify(message)
					if err != nil {
						log.Error("Error while sending message to telegram: %v", err)
					}
					log.Debug("cache: %v | activity resp: %v", cache[wallet], activity)
					log.Info("Sent notification successfully")
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
