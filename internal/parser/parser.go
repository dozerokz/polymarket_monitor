package parser

import (
	"encoding/json"
	"fmt"
	"github.com/dozerokz/logger"
	"io"
	"net/http"
	"net/url"
	"polymarket_monitor/internal/activitycache"
	"polymarket_monitor/internal/notifier"
	"slices"
	"strconv"
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

// eventDetailsCache keeps gamma-api lookups by event slug to avoid repeating tag requests.
var eventDetailsCache = map[string]eventDetailsResponse{}

// initialized marks wallets that were warmed up (to avoid spamming old activity after transient init errors).
var initialized = map[string]bool{}

func normalizeTransactionHash(hash string) string {
	return strings.ToLower(strings.TrimSpace(hash))
}

func normalizeWalletAddress(wallet string) string {
	return strings.ToLower(strings.TrimSpace(wallet))
}

func buildActivityKey(event activityResponse) string {
	return strings.Join([]string{
		strconv.FormatInt(event.Timestamp, 10),
		normalizeTransactionHash(event.TransactionHash),
		strings.ToUpper(strings.TrimSpace(event.Type)),
		strings.ToUpper(strings.TrimSpace(event.Side)),
		strings.TrimSpace(event.EventSlug),
		strings.TrimSpace(event.Slug),
		strings.TrimSpace(event.Outcome),
		strconv.FormatFloat(event.Size, 'f', -1, 64),
		strconv.FormatFloat(event.UsdcSize, 'f', -1, 64),
	}, "|")
}

func seenEventFromActivity(event activityResponse) activitycache.SeenEvent {
	return activitycache.SeenEvent{
		Key:             buildActivityKey(event),
		TransactionHash: normalizeTransactionHash(event.TransactionHash),
		SeenAt:          time.Unix(event.Timestamp, 0).UTC(),
	}
}

func isStaleActivity(event activityResponse, watermark time.Time) bool {
	if event.Timestamp <= 0 {
		return true
	}

	return time.Unix(event.Timestamp, 0).UTC().Before(watermark)
}

func belongsToTrackedWallet(trackedWallet string, event activityResponse) bool {
	proxyWallet := normalizeWalletAddress(event.ProxyWallet)
	if proxyWallet == "" {
		return true
	}

	return proxyWallet == normalizeWalletAddress(trackedWallet)
}

func displayHandle(event activityResponse) string {
	if handle := strings.TrimSpace(event.Name); handle != "" {
		return handle
	}

	return strings.TrimSpace(event.Pseudonym)
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
	if len(blacklistedEventTags) == 0 {
		return false, nil, nil
	}

	eventDetails, err := getEventDetails(eventLookupSlug(event))
	if err != nil {
		return false, nil, err
	}

	matchedTags := findBlacklistedEventTags(eventDetails.Tags)
	return len(matchedTags) > 0, matchedTags, nil
}

func seedWalletActivity(wallet string, activity []activityResponse, store *activitycache.Store, log *logger.Logger) error {
	seenEvents := make([]activitycache.SeenEvent, 0, len(activity))
	for i := len(activity) - 1; i >= 0; i-- {
		event := activity[i]
		if !belongsToTrackedWallet(wallet, event) {
			log.Error("Skipped seeding mismatched activity for requested wallet %s: proxyWallet=%s tx=%s",
				wallet, event.ProxyWallet, normalizeTransactionHash(event.TransactionHash))
			continue
		}

		seenEvent := seenEventFromActivity(event)
		if seenEvent.Key == "" {
			continue
		}
		seenEvents = append(seenEvents, seenEvent)
	}

	if err := store.SeedWallet(wallet, seenEvents, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to seed wallet %s activity cache: %w", wallet, err)
	}

	return nil
}

// initMonitor seeds wallet activity in the persistent cache to avoid notifications for old events.
// If a wallet fails to init (network/API error), it will be warmed up later in the monitor loop.
func initMonitor(wallets []string, store *activitycache.Store, log *logger.Logger) (int, error) {
	if len(wallets) == 0 {
		return 0, fmt.Errorf("wallets list is empty")
	}

	initializedCount := 0
	for _, wallet := range wallets {
		isInitialized, err := store.IsWalletInitialized(wallet)
		if err != nil {
			return initializedCount, err
		}
		initialized[wallet] = isInitialized
		if initialized[wallet] {
			initializedCount++
			continue
		}

		activity, err := getActivity(wallet)
		if err != nil {
			log.Error("Failed to get wallet %s activity during init: %v", wallet, err)
			continue
		}

		if err = seedWalletActivity(wallet, activity, store, log); err != nil {
			log.Error("Failed to seed wallet %s activity cache during init: %v", wallet, err)
			continue
		}

		initialized[wallet] = true
		initializedCount++
	}

	return initializedCount, nil
}

// Monitor is main monitoring function.
// Tracking wallets activity, comparing to previously saved, sending notification to telegram if new activity detected
func Monitor(wallets []string, tgNotifier *notifier.TgNotifier, store *activitycache.Store, log *logger.Logger) {
	initializedCount, initErr := initMonitor(wallets, store, log)
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
				if err = seedWalletActivity(wallet, activity, store, log); err != nil {
					log.Error("Failed to seed wallet %s activity cache: %v | next attempt in %v",
						wallet, err, monitorErrorDelay)
					nextFetchAt[wallet] = time.Now().Add(monitorErrorDelay)
					continue
				}
				initialized[wallet] = true
				log.Info("Initialized wallet %s", wallet)
				continue
			}

			activityWatermark, err := store.ActivityWatermark(wallet)
			if err != nil {
				log.Error("Failed to read activity watermark for wallet %s: %v", wallet, err)
				continue
			}

			// Process from oldest to newest to keep cache eviction consistent and notifications chronological.
			for i := len(activity) - 1; i >= 0; i-- {
				event := activity[i]
				seenEvent := seenEventFromActivity(event)
				if seenEvent.Key == "" {
					continue
				}
				if !belongsToTrackedWallet(wallet, event) {
					log.Error("Skipped mismatched activity for requested wallet %s: proxyWallet=%s tx=%s",
						wallet, event.ProxyWallet, seenEvent.TransactionHash)
					continue
				}

				seen, err := store.HasSeenEvent(wallet, seenEvent.Key)
				if err != nil {
					log.Error("Failed to read activity cache for wallet %s, tx %s: %v",
						wallet, seenEvent.TransactionHash, err)
					continue
				}
				if seen {
					continue
				}

				eventTime := time.Unix(event.Timestamp, 0).UTC()
				if isStaleActivity(event, activityWatermark) {
					log.Info("Skipped stale activity for wallet %s, tx %s: event_time=%s watermark=%s",
						wallet, seenEvent.TransactionHash, eventTime.Format(time.RFC3339),
						activityWatermark.Format(time.RFC3339Nano))
					if err = store.RememberEvent(wallet, seenEvent); err != nil {
						log.Error("Failed to update activity cache for wallet %s, tx %s: %v",
							wallet, seenEvent.TransactionHash, err)
					}
					continue
				}

				message := buildNotifierMessage(wallet, event)
				if message == "" {
					if err = store.RememberEvent(wallet, seenEvent); err != nil {
						log.Error("Failed to update activity cache for wallet %s, tx %s: %v",
							wallet, seenEvent.TransactionHash, err)
					}
					if eventTime.After(activityWatermark) {
						activityWatermark = eventTime
					}
					continue
				}

				shouldSkip, matchedTags, err := shouldSkipNotificationByTags(event)
				if err != nil {
					log.Error("Failed to resolve tags for event %s (tx %s): %v | notification will be sent anyway",
						eventLookupSlug(event), seenEvent.TransactionHash, err)
				} else if shouldSkip {
					log.Info("Skipped notification for wallet %s, tx %s, event %s due to blacklisted tags: %s",
						wallet, seenEvent.TransactionHash, eventLookupSlug(event), strings.Join(matchedTags, ", "))
					if err = store.RememberEvent(wallet, seenEvent); err != nil {
						log.Error("Failed to update activity cache for wallet %s, tx %s: %v",
							wallet, seenEvent.TransactionHash, err)
					}
					if eventTime.After(activityWatermark) {
						activityWatermark = eventTime
					}
					continue
				}

				err = tgNotifier.Notify(message)
				if err != nil {
					log.Error("Error while sending message to telegram: %v", err)
				} else {
					log.Info("Sent notification successfully for wallet %s, tx %s", wallet, seenEvent.TransactionHash)
				}

				if err = store.RememberEvent(wallet, seenEvent); err != nil {
					log.Error("Failed to update activity cache for wallet %s, tx %s: %v",
						wallet, seenEvent.TransactionHash, err)
				}
				if eventTime.After(activityWatermark) {
					activityWatermark = eventTime
				}
			}
		}
		time.Sleep(monitorSleepDelay)
	}
}

func buildEventPageURL(event activityResponse) string {
	pathParts := make([]string, 0, 2)
	if slug := strings.Trim(event.EventSlug, "/"); slug != "" {
		pathParts = append(pathParts, slug)
	}
	if slug := strings.Trim(event.Slug, "/"); slug != "" {
		if len(pathParts) == 0 || pathParts[len(pathParts)-1] != slug {
			pathParts = append(pathParts, slug)
		}
	}
	if len(pathParts) == 0 {
		return ""
	}

	return strings.TrimRight(eventURL, "/") + "/" + strings.Join(pathParts, "/")
}

func buildTrackedWalletLabel(trackedWallet string, event activityResponse) string {
	handle := displayHandle(event)
	if handle == "" {
		return fmt.Sprintf("<code>%s</code>", trackedWallet)
	}

	return fmt.Sprintf("<a href=\"%s%s\">@%s</a> (<code>%s</code>)", profileURL, handle, handle, trackedWallet)
}

// buildNotifierMessage creates formatted message for telegram.
func buildNotifierMessage(trackedWallet string, event activityResponse) string {
	if event.Type == "REWARD" {
		return ""
	}

	action := ""
	switch {
	case event.Type == "TRADE" && event.Side == "BUY":
		action = "Bought"
	case event.Type == "TRADE" && event.Side == "SELL":
		action = "Sold"
	default:
		return ""
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<b>New Polymarket Prediction From %s</b>\n\n", buildTrackedWalletLabel(trackedWallet, event)))
	builder.WriteString(fmt.Sprintf("<b>%s</b>\n\n", event.Title))
	builder.WriteString(fmt.Sprintf("<b>%s</b> %.1f of <b>%s</b> \n", action, event.Size, event.Outcome))
	builder.WriteString(fmt.Sprintf("Price: %.0f¢ \n", event.Price*100))
	builder.WriteString(fmt.Sprintf("Total: $%.2f\n\n", event.UsdcSize))

	if eventPageURL := buildEventPageURL(event); eventPageURL != "" {
		builder.WriteString(fmt.Sprintf("[<a href=\"%s\">View on Polymarket</a>]", eventPageURL))
	}

	return builder.String()
}
