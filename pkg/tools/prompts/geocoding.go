// Package prompts provides prompt templates for use with the MCP server.
package prompts

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// geocodingSystemPromptText contains instructions for using the geocoding tools.
const geocodingSystemPromptText = `You have access to geocoding tools that convert between addresses and coordinates.
When using these tools:

1. Format addresses clearly without parentheses, e.g., "Blue Temple Chiang Rai Thailand" instead of "Blue Temple (Wat Rong Suea Ten)"
2. Always include city and country for international locations
3. If geocoding fails, check the error message for suggestions and try with the suggested improvements
4. Try progressive simplification when address lookups fail
5. For reverse geocoding, ensure coordinates are in decimal form within valid ranges

IMPORTANT ADDRESS FORMATTING EXAMPLES:
✅ GOOD: "Blue Temple Chiang Rai Thailand"
❌ BAD: "Blue Temple (Wat Rong Suea Ten)"

✅ GOOD: "Eiffel Tower, Paris, France"
❌ BAD: "Eiffel Tower"

✅ GOOD: "Sydney Opera House, Sydney, Australia"
❌ BAD: "The Opera House"

ERROR HANDLING GUIDELINES:
When you receive error responses from the geocoding tools:
1. Parse the error message for the error code and suggestions
2. Try the suggestions provided in the error
3. If an address with parentheses fails, remove the parenthetical content
4. If a landmark name fails, add city and country information
5. Use the most specific, clear address format possible`

// GeocodingSystemPrompt returns the system prompt text used for the geocoding
// prompt template and GetPrompt handler.
func GeocodingSystemPrompt() string {
	return geocodingSystemPromptText
}

// RegisterGeocodingPrompts registers all geocoding-related prompts with the MCP server
func RegisterGeocodingPrompts(s *server.MCPServer) {
	// Register the main geocoding prompt
	s.AddPrompt(mcp.NewPrompt("geocoding",
		mcp.WithPromptDescription("Instructions for properly using geocoding tools"),
	), GeocodingPromptHandler)

	// Register examples for geocode_address
	s.AddPrompt(mcp.NewPrompt("geocode_address_examples",
		mcp.WithPromptDescription("Examples of properly formatted address geocoding queries"),
	), GeocodeAddressExamplesHandler)

	// Register examples for reverse_geocode
	s.AddPrompt(mcp.NewPrompt("reverse_geocode_examples",
		mcp.WithPromptDescription("Examples of properly formatted reverse geocoding queries"),
	), ReverseGeocodeExamplesHandler)
}

// GeocodingPromptHandler returns the main prompt for geocoding tools
func GeocodingPromptHandler(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := GeocodingSystemPrompt()

	return mcp.NewGetPromptResult(
		"Geocoding Tool Usage Guidelines",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewTextContent(systemPrompt),
			),
		},
	), nil
}

// GeocodeAddressExamplesHandler returns examples for geocode_address
func GeocodeAddressExamplesHandler(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	examplesPrompt := `EXAMPLES OF EFFECTIVE GEOCODE_ADDRESS USAGE:

User: "Can you find the coordinates for the Blue Temple in Thailand?"
AI: *uses geocode_address with "Blue Temple Chiang Rai Thailand"*

User: "What are the coordinates of the Eiffel Tower?"
AI: *uses geocode_address with "Eiffel Tower Paris France"*

User: "Where is the Sydney Opera House located?"
AI: *uses geocode_address with "Sydney Opera House Sydney Australia"*

ERROR CORRECTION PATTERN:
1. If you get a NO_RESULTS error when looking up "Blue Temple (Wat Rong Suea Ten)"
2. Check the suggestions in the error response
3. Retry with "Blue Temple Chiang Rai Thailand" as suggested
4. Return the successfully geocoded coordinates`

	return mcp.NewGetPromptResult(
		"Geocode Address Examples",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewTextContent(examplesPrompt),
			),
		},
	), nil
}

// ReverseGeocodeExamplesHandler returns examples for reverse_geocode
func ReverseGeocodeExamplesHandler(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	examplesPrompt := `EXAMPLES OF EFFECTIVE REVERSE_GEOCODE USAGE:

User: "What's at these coordinates: 37.7749, -122.4194?"
AI: *uses reverse_geocode with latitude: 37.7749, longitude: -122.4194*

User: "Can you tell me the address for 19.9584 N, 99.8787 E?"
AI: *converts to decimal first, then uses reverse_geocode with latitude: 19.9584, longitude: 99.8787*

User: "What's located at the following position: 40°41'40.2"N 74°07'00.0"W?"
AI: *converts from DMS to decimal first (40.69450, -74.11667), then uses reverse_geocode*

ERROR CORRECTION PATTERN:
1. If coordinates are in DMS format (degrees, minutes, seconds), convert to decimal
2. Ensure latitude is between -90 and 90
3. Ensure longitude is between -180 and 180
4. Use at least 4 decimal places for precision
5. If results are unclear, try slightly offset coordinates to find nearby locations`

	return mcp.NewGetPromptResult(
		"Reverse Geocode Examples",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewTextContent(examplesPrompt),
			),
		},
	), nil
}
