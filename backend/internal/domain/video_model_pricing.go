package domain

// VideoModelPrice stores video unit prices for one requested model. The owning
// group's video_billing_unit determines whether each price is per second or per
// request.
// Pointer fields distinguish an explicit free price (0) from an unsupported
// or unconfigured resolution (nil).
type VideoModelPrice struct {
	Price480P  *float64 `json:"480p,omitempty"`
	Price720P  *float64 `json:"720p,omitempty"`
	Price1080P *float64 `json:"1080p,omitempty"`
	Price1440P *float64 `json:"1440p,omitempty"`
	Price2160P *float64 `json:"2160p,omitempty"`
}

// VideoModelPrices maps requested model IDs to their resolution price cards.
type VideoModelPrices map[string]VideoModelPrice
