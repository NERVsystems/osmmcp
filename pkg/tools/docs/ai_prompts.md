# AI Prompts for Geocoding Tools

This document contains prompts to help AI assistants properly use the geocoding tools. These prompts can be integrated into your MCP configuration to improve geocoding success rates.

## System Prompts

### General Geocoding System Prompt

```
You have access to geocoding tools that convert between addresses and coordinates. When using these:

1. Format addresses clearly without parentheses, e.g., "Merlion Park Singapore" instead of "Merlion Park (Singapore)"
2. Always include city and country for international locations 
3. If geocoding fails, check the error message for suggestions
4. Try progressive simplification when address lookups fail
5. For reverse geocoding, ensure coordinates are in decimal form within valid ranges
```

### Geocode Address Tool Prompt

```
When using the geocode_address tool, follow these guidelines:

1. SIMPLIFY COMPLEX QUERIES
   - Remove parentheses and special characters 
   - Example: "Merlion Park Singapore" NOT "Merlion Park (Singapore)"

2. ADD GEOGRAPHIC CONTEXT
   - Always include city, region, country for international locations
   - Example: "Eiffel Tower, Paris, France" NOT just "Eiffel Tower"

3. ERROR HANDLING
   - If you receive a NO_RESULTS error, follow the suggestions in the response
   - Try removing parenthetical information or simplifying the query
   - Try adding geographic context (city/country names)

4. PROGRESSIVE REFINEMENT
   - Start specific, then try broader forms if needed
   - If detailed address fails, try just the landmark name with location
```

### Reverse Geocode Tool Prompt

```
When using the reverse_geocode tool, follow these guidelines:

1. FORMAT COORDINATES PROPERLY
   - Use decimal degrees (e.g., 37.7749, -122.4194)
   - Do NOT use degrees/minutes/seconds format

2. VALIDATE COORDINATE RANGES
   - Latitude must be between -90 and 90
   - Longitude must be between -180 and 180

3. PRECISION MATTERS
   - Use at least 4 decimal places when available
   - More precise coordinates yield better results

4. HANDLE EMPTY RESULTS
   - If no meaningful address is returned, try coordinates slightly offset from original
   - Shift by 0.0001 degrees in any direction and try again
```

## Example Integration with MCP-Go

Here's how to integrate these prompts into your MCP server configuration using the latest mcp-go v0.28.0+ API:

```go
package main

import (
	"context"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/NERVsystems/osmmcp/pkg/tools"
)

func main() {
	// Create an MCP server
	s := server.NewMCPServer(
		"Geocoding Example",
		"1.0",
		server.WithToolCapabilities(true),
	)

	// Register geocoding tools with improved descriptions
	geocodeAddressTool := tools.GeocodeAddressTool()
	reverseGeocodeTool := tools.ReverseGeocodeTool()

	// Add tools to server
	s.AddTool(geocodeAddressTool, tools.HandleGeocodeAddress)
	s.AddTool(reverseGeocodeTool, tools.HandleReverseGeocode)

	// Create a prompt with instructions for the AI
	systemPrompt := `You have access to geocoding tools that convert between addresses and coordinates. 
When using these tools:

1. Format addresses clearly without parentheses, e.g., "Merlion Park Singapore" instead of "Merlion Park (Singapore)"
2. Always include city and country for international locations 
3. If geocoding fails, check the error message for suggestions
4. Try progressive simplification when address lookups fail
5. For reverse geocoding, ensure coordinates are in decimal form within valid ranges`

	// Set up the prompt using the new API
	geocodingPrompt := mcp.NewPrompt("geocoding_system",
		mcp.WithPromptDescription("System prompt with geocoding instructions"),
	)
	
	// Add the prompt with its handler function
	s.AddPrompt(geocodingPrompt, func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult(
			"Geocoding System Instructions",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleSystem, 
					mcp.NewTextContent(systemPrompt),
				),
			},
		), nil
	})

	// Start the server
	// ...
}
```

## Example Client Interaction

When integrating with an LLM client, ensure the client is instructed to follow these patterns:

```json
{
  "prompt": {
    "messages": [
      {
        "role": "system",
        "content": {
          "type": "text",
          "text": "You have access to geocoding tools. Format addresses clearly without parentheses. Always include city and country for international locations."
        }
      },
      {
        "role": "user",
        "content": {
          "type": "text",
          "text": "Can you find the coordinates of the Merlion Park in Singapore?"
        }
      }
    ]
  }
}
```

The AI should then use the `geocode_address` tool with a properly formatted query:

```json
{
  "name": "geocode_address",
  "arguments": {
    "address": "Merlion Park Singapore"
  }
}
```

Rather than:

```json
{
  "name": "geocode_address",
  "arguments": {
    "address": "Merlion Park (Singapore)"
  }
}
```

Which would likely fail to return results. 