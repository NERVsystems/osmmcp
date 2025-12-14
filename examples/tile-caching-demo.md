# Tile Caching Feature Demo

This document demonstrates the new tile caching functionality implemented in the OSM MCP server.

## Overview

The tile caching feature provides:

1. **OSM Compliance**: Caches tiles locally to reduce load on OpenStreetMap servers, as recommended by OSM usage guidelines
2. **Performance**: Faster tile retrieval for repeated requests
3. **MCP Integration**: Exposes cached tiles through the MCP protocol for AI analysis
4. **Resource Management**: Automatic cache management with TTL and size limits

## Architecture

### Components

- **TileResourceManager** (`pkg/cache/tile_resource.go`): Manages tile resources with metadata
- **Enhanced FetchMapTile** (`pkg/core/tiles.go`): Integrates resource management with tile fetching
- **Tile Cache Tool** (`pkg/tools/tile_cache.go`): MCP tool for accessing cached tiles
- **URI Scheme**: `osm://tile/{zoom}/{x}/{y}` for tile resource identification

### Cache Configuration

- **TTL**: 24 hours (configurable)
- **Max Items**: 1000 tiles (configurable)
- **Cleanup**: Every minute
- **Storage**: In-memory with automatic eviction

## Usage Examples

### 1. Fetch a Map Tile (Automatic Caching)

```json
{
  "method": "tools/call",
  "params": {
    "name": "get_map_image",
    "arguments": {
      "latitude": 37.7749,
      "longitude": -122.4194,
      "zoom": 14
    }
  }
}
```

This automatically caches the tile and creates a resource with URI: `osm://tile/14/2621/6333`

### 2. List Cached Tiles

```json
{
  "method": "tools/call",
  "params": {
    "name": "tile_cache",
    "arguments": {
      "action": "list"
    }
  }
}
```

Response:
```json
{
  "cached_tiles": [
    {
      "uri": "osm://tile/14/2621/6333",
      "name": "Map Tile 14/2621/6333",
      "description": "OpenStreetMap tile at zoom 14, coordinates (2621, 6333)",
      "zoom": 14,
      "x": 2621,
      "y": 6333
    }
  ],
  "count": 1
}
```

### 3. Get Specific Cached Tile

```json
{
  "method": "tools/call",
  "params": {
    "name": "tile_cache",
    "arguments": {
      "action": "get",
      "x": 2621,
      "y": 6333,
      "zoom": 14
    }
  }
}
```

Returns both metadata and base64-encoded image data.

### 4. Cache Statistics

```json
{
  "method": "tools/call",
  "params": {
    "name": "tile_cache",
    "arguments": {
      "action": "stats"
    }
  }
}
```

Response:
```json
{
  "cached_tiles": 15,
  "max_tiles": 1000,
  "ttl_hours": 24
}
```

## Resource Format

Each cached tile becomes an MCP resource with:

### Metadata (JSON)
```json
{
  "zoom": 14,
  "x": 2621,
  "y": 6333,
  "centerLat": 37.7749,
  "centerLon": -122.4194,
  "northLat": 37.8131,
  "southLat": 37.7367,
  "eastLon": -122.3926,
  "westLon": -122.4462,
  "pixelSizeMeters": 38.21,
  "mapScale": "1:147115"
}
```

### Image Data (Base64 PNG)
The actual tile image as base64-encoded PNG data.

## Benefits for AI Analysis

1. **Persistent Context**: AI can reference previously viewed tiles
2. **Metadata Rich**: Each tile includes geographic bounds and scale information
3. **Efficient Access**: No need to re-fetch tiles for analysis
4. **Structured Format**: Consistent URI scheme for tile identification

## Cache Management

- **Automatic Expiration**: Tiles expire after 24 hours
- **LRU Eviction**: Oldest tiles removed when cache is full
- **Memory Efficient**: Only keeps active tiles in memory
- **Background Cleanup**: Expired tiles removed automatically

## OSM Compliance

This implementation follows OpenStreetMap's tile usage guidelines:

- ✅ Local caching to reduce server load
- ✅ Reasonable cache TTL (24 hours)
- ✅ Proper User-Agent headers
- ✅ Rate limiting through cache hits
- ✅ No bulk downloading without permission

## Future Enhancements

Potential improvements:
- Disk-based cache persistence
- Cache warming for common areas
- Tile pre-fetching for routes
- Integration with other map providers
- Resource-based tile serving to AI clients 