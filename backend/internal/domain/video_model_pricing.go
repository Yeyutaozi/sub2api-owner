package domain

// VideoModelPrice stores per-second video prices for one requested model.
// Pointer fields distinguish an explicit free price (0) from an unsupported
// or unconfigured resolution (nil).
type VideoModelPrice struct {
	Price480P  *float64 `json:"480p,omitempty"`
	Price720P  *float64 `json:"720p,omitempty"`
	Price1080P *float64 `json:"1080p,omitempty"`
}

// VideoModelPrices maps requested model IDs to their resolution price cards.
type VideoModelPrices map[string]VideoModelPrice
