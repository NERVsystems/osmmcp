// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

// Location represents a geographic coordinate (latitude and longitude)
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Address represents a structured address
type Address struct {
	Street      string `json:"street,omitempty"`
	HouseNumber string `json:"house_number,omitempty"`
	City        string `json:"city,omitempty"`
	State       string `json:"state,omitempty"`
	Country     string `json:"country,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	Formatted   string `json:"formatted,omitempty"`
}

// Place represents a named location with coordinates and optional address
type Place struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Location   Location `json:"location"`
	Address    Address  `json:"address,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Rating     float64  `json:"rating,omitempty"`
	Distance   float64  `json:"distance,omitempty"`   // in meters
	Importance float64  `json:"importance,omitempty"` // Nominatim importance score
}

// Route represents a path between two locations
type Route struct {
	Distance     float64    `json:"distance"` // in meters
	Duration     float64    `json:"duration"` // in seconds
	StartPoint   Location   `json:"start_point"`
	EndPoint     Location   `json:"end_point"`
	Instructions []string   `json:"instructions"`
	Polyline     []Location `json:"polyline,omitempty"`
}

// TransportMode represents different transportation methods
type TransportMode string

const (
	TransportModeCar     TransportMode = "car"
	TransportModeBicycle TransportMode = "bicycle"
	TransportModeWalking TransportMode = "walking"
	TransportModeTransit TransportMode = "transit"
)
