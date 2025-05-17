// Package osm provides utilities for interacting with OpenStreetMap APIs.
package osm

// OverpassElement represents an element returned from the Overpass API
type OverpassElement struct {
	ID     int     `json:"id"`
	Type   string  `json:"type"`
	Lat    float64 `json:"lat,omitempty"`
	Lon    float64 `json:"lon,omitempty"`
	Center *struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center,omitempty"`
	Tags    map[string]string `json:"tags,omitempty"`
	Nodes   []int64           `json:"nodes,omitempty"` // For ways, list of node IDs
	Members []struct {
		Type string `json:"type"`
		Ref  int64  `json:"ref"`
		Role string `json:"role"`
	} `json:"members,omitempty"` // For relations
}
