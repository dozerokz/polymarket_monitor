package parser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
