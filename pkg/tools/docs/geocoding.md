# Geocoding Tools Guide

## Overview

The geocoding tools provide functionality to convert between addresses and geographic coordinates. These tools are essential for tasks involving location data, but they can be sensitive to input formatting.

## Available Tools

### 1. `geocode_address`

Converts a textual address or place name into geographic coordinates.

**Usage:**
```go
// Register the tool in your server configuration
geocodeAddressTool := tools.GeocodeAddressTool()
server.AddTool(geocodeAddressTool, tools.HandleGeocodeAddress)
```

**Input Parameters:**
- `address` (string, required): The address or place name to geocode

**Output:**
- A JSON object containing the geocoded place information, including coordinates and formatted address

**Error Codes:**
- `EMPTY_ADDRESS`: The address parameter was empty or not provided
- `NO_RESULTS`: No results were found for the provided address
- `SERVICE_ERROR`: Failed to communicate with the geocoding service
- `PARSE_ERROR`: Failed to parse the geocoding response
- `INTERNAL_ERROR`: An internal server error occurred

### 2. `reverse_geocode`

Converts geographic coordinates into a human-readable address.

**Usage:**
```go
// Register the tool in your server configuration
reverseGeocodeTool := tools.ReverseGeocodeTool()
server.AddTool(reverseGeocodeTool, tools.HandleReverseGeocode)
```

**Input Parameters:**
- `latitude` (float64, required): The latitude coordinate (-90 to 90)
- `longitude` (float64, required): The longitude coordinate (-180 to 180)

**Output:**
- A JSON object containing the place information, including formatted address and address components

**Error Codes:**
- `INVALID_LATITUDE`: The latitude is outside the valid range (-90 to 90)
- `INVALID_LONGITUDE`: The longitude is outside the valid range (-180 to 180)
- `SERVICE_ERROR`: Failed to communicate with the geocoding service
- `PARSE_ERROR`: Failed to parse the geocoding response
- `INTERNAL_ERROR`: An internal server error occurred

## Best Practices for AI Assistants

When using these geocoding tools, follow these guidelines to increase success rates:

### Address Formatting Guidelines

1. **Simplify complex queries**: 
   - Remove parentheses, special characters, and unnecessary details
   - BAD: "Blue Temple (Wat Rong Suea Ten) in Chiang Rai"
   - GOOD: "Blue Temple Chiang Rai Thailand"

2. **Add geographic context**: 
   - Always include city, region, country names for international locations
   - BAD: "Eiffel Tower"
   - GOOD: "Eiffel Tower, Paris, France"

3. **Be specific with landmarks**: 
   - Use official names and locations for tourist sites
   - BAD: "Main cathedral in Barcelona"
   - GOOD: "Sagrada Familia, Barcelona, Spain"

4. **Handle errors gracefully**: 
   - If geocoding fails, try removing parenthetical information
   - Try alternative name formats before giving up
   - Check the error code and suggestions returned in the error response

5. **Progressive refinement**: 
   - Start with the most specific form, then try broader forms
   - If "123 Main St, Small Town" fails, try "Main St, Small Town"

### Example Queries That Work Well

```
1600 Amphitheatre Parkway, Mountain View, CA
Eiffel Tower, Paris, France
Sydney Opera House, Australia
Blue Temple Chiang Rai Thailand
Times Square New York
```

### Coordinate Format Guidelines

1. **Use decimal format**: 
   - Coordinates should be in decimal degrees
   - Example: 37.7749, -122.4194

2. **Respect coordinate boundaries**:
   - Latitude must be between -90 and 90 degrees
   - Longitude must be between -180 and 180 degrees

3. **Use sufficient precision**: 
   - For high precision, use at least 4 decimal places when known
   - Example: 37.7749 instead of 37.77

4. **Try offset coordinates**:
   - If coordinates don't return a meaningful address, try slightly offset points
   - Shift by 0.0001 degrees in any direction

### Error Handling

When errors occur, the tool will return a structured error response containing:

- `code`: A string code indicating the type of error
- `message`: A human-readable error message
- `query`: The original query that caused the error
- `suggestions`: An array of suggestions for fixing the issue

AI assistants should:

1. Parse this structured error
2. Follow the suggestions in the response
3. Try alternative formulations based on the error code
4. Provide clear feedback to the user about what went wrong
5. Suggest alternatives when appropriate

## Integrating with MCP Prompts

See [AI Prompts for Geocoding](ai_prompts.md) for examples of system prompts that can be used to guide AI systems in using these tools effectively.

By following these guidelines, AI assistants can significantly improve the success rate of geocoding operations and provide better location-based assistance. 