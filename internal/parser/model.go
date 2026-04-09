package parser

// activityResponse is response structure from polymarket activity endpoint
type activityResponse struct {
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
	TransactionHash string  `json:"transactionHash"`
}
