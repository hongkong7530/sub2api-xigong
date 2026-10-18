package service

import "time"

type SiteModelUsage struct {
	Model    string `json:"model"`
	Tokens   int64  `json:"tokens"`
	Requests int64  `json:"requests"`
}
type SiteUsageSnapshot struct {
	SampledAt         time.Time        `json:"sampled_at"`
	Day               string           `json:"day"`
	Requests          int64            `json:"requests"`
	TodayTokens       int64            `json:"today_tokens"`
	AveragePerMillion *float64         `json:"average_per_million"`
	PriceSamples      int64            `json:"price_samples"`
	Models            []SiteModelUsage `json:"models"`
}
