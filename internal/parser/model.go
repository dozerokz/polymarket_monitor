package parser

// activityResponse is response structure from polymarket activity endpoint
type activityResponse struct {
	Timestamp       int64   `json:"timestamp"`
	Type            string  `json:"type"`
	Size            float64 `json:"size"`
	UsdcSize        float64 `json:"usdcSize"`
	Price           float64 `json:"price"`
	Side            string  `json:"side"`
	Title           string  `json:"title"`
	Slug            string  `json:"slug"`
	EventSlug       string  `json:"eventSlug"`
	Outcome         string  `json:"outcome"`
	Name            string  `json:"name"`
	Pseudonym       string  `json:"pseudonym"`
	ProxyWallet     string  `json:"proxyWallet"`
	TransactionHash string  `json:"transactionHash"`
}

// eventDetailsResponse is the subset of gamma-api event payload used for tag-based filtering.
type eventDetailsResponse struct {
	Slug  string             `json:"slug"`
	Title string             `json:"title"`
	Tags  []eventTagResponse `json:"tags"`
}

type eventTagResponse struct {
	Label string `json:"label"`
	Slug  string `json:"slug"`
}
