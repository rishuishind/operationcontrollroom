package db

// CommunityArea is static reference data (Chicago community area id, name,
// and an approximate centroid used to query weather for that area).
type CommunityArea struct {
	ID   int
	Name string
	Lat  float64
	Lon  float64
}

// Areas mirrors cmd/genseed's area list -- kept in sync manually since it's
// small, static reference data rather than something derived from the trips
// extract.
var Areas = []CommunityArea{
	{32, "Loop", 41.8786, -87.6298},
	{8, "Near North Side", 41.9035, -87.6316},
	{76, "O'Hare", 41.9786, -87.9047},
	{25, "Austin", 41.8956, -87.7654},
	{41, "Hyde Park", 41.7943, -87.5907},
	{6, "Lake View", 41.9440, -87.6538},
	{33, "Near South Side", 41.8514, -87.6236},
	{22, "Logan Square", 41.9289, -87.7079},
}
