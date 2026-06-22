package parser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFindBlacklistedEventTags(t *testing.T) {
	originalTags := append([]string(nil), blacklistedEventTags...)
	defer func() {
		blacklistedEventTags = originalTags
	}()

	SetBlacklistedEventTags([]string{"sports", "esports"})

	tags := []eventTagResponse{
		{Label: "Esports", Slug: "esports"},
		{Label: "Politics", Slug: "politics"},
		{Label: "Sports"},
		{Slug: "sports"},
	}

	matchedTags := findBlacklistedEventTags(tags)

	expected := []string{"esports", "sports"}
	if len(matchedTags) != len(expected) {
		t.Fatalf("expected %d matched tags, got %d (%v)", len(expected), len(matchedTags), matchedTags)
	}

	for i := range expected {
		if matchedTags[i] != expected[i] {
			t.Fatalf("expected matchedTags[%d] to be %q, got %q", i, expected[i], matchedTags[i])
		}
	}
}

func TestGetEventDetailsUsesCache(t *testing.T) {
	originalEndpoint := gammaEventEndpoint
	originalCache := eventDetailsCache
	defer func() {
		gammaEventEndpoint = originalEndpoint
		eventDetailsCache = originalCache
	}()

	eventDetailsCache = map[string]eventDetailsResponse{}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		fmt.Fprint(w, `{"slug":"test-event","title":"Test event","tags":[{"label":"Sports","slug":"sports"}]}`)
	}))
	defer server.Close()

	gammaEventEndpoint = server.URL + "/events/slug/%s"

	firstResponse, err := getEventDetails("test-event")
	if err != nil {
		t.Fatalf("first getEventDetails returned error: %v", err)
	}

	secondResponse, err := getEventDetails("test-event")
	if err != nil {
		t.Fatalf("second getEventDetails returned error: %v", err)
	}

	if requestCount.Load() != 1 {
		t.Fatalf("expected 1 upstream request, got %d", requestCount.Load())
	}

	if firstResponse.Slug != "test-event" || secondResponse.Slug != "test-event" {
		t.Fatalf("unexpected cached event slug values: first=%q second=%q", firstResponse.Slug, secondResponse.Slug)
	}
}

func TestBuildActivityKeyDistinguishesSameTransactionHash(t *testing.T) {
	firstEvent := activityResponse{
		Timestamp:       100,
		Type:            "TRADE",
		Side:            "BUY",
		Slug:            "market-slug",
		Outcome:         "Yes",
		Size:            10,
		UsdcSize:        5,
		TransactionHash: "0xabc",
	}
	secondEvent := firstEvent
	secondEvent.Size = 12

	if buildActivityKey(firstEvent) == buildActivityKey(secondEvent) {
		t.Fatal("expected activity keys to differ for fills that share tx hash but differ by amount")
	}
}

func TestIsStaleActivity(t *testing.T) {
	watermark := time.Unix(200, 0).UTC()

	tests := []struct {
		name      string
		timestamp int64
		want      bool
	}{
		{name: "missing timestamp", timestamp: 0, want: true},
		{name: "older event", timestamp: 199, want: true},
		{name: "same second", timestamp: 200, want: false},
		{name: "newer event", timestamp: 201, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStaleActivity(activityResponse{Timestamp: tt.timestamp}, watermark); got != tt.want {
				t.Fatalf("isStaleActivity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildEventPageURLFallsBackToMarketSlug(t *testing.T) {
	event := activityResponse{
		Slug: "fifwc-jor-alg-2026-06-22-jor",
	}

	got := buildEventPageURL(event)
	want := "https://polymarket.com/event/fifwc-jor-alg-2026-06-22-jor"
	if got != want {
		t.Fatalf("buildEventPageURL() = %q, want %q", got, want)
	}
}

func TestBuildNotifierMessageIncludesTrackedWallet(t *testing.T) {
	message := buildNotifierMessage("0x1234", activityResponse{
		Type:      "TRADE",
		Side:      "BUY",
		Name:      "Allezpapa",
		Title:     "Will Jordan win on 2026-06-22?",
		Outcome:   "Yes",
		Size:      769.6,
		Price:     0.15,
		UsdcSize:  115.44,
		Slug:      "fifwc-jor-alg-2026-06-22-jor",
		EventSlug: "",
	})

	if !strings.Contains(message, "<code>0x1234</code>") {
		t.Fatalf("expected tracked wallet to be present in message, got %q", message)
	}
	if strings.Contains(message, "event//") {
		t.Fatalf("expected message link to avoid double slash, got %q", message)
	}
}
