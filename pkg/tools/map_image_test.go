package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleGetMapImage(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments,omitempty"`
			Meta      *mcp.Meta      `json:"_meta,omitempty"`
		}{
			Name: "get_map_image",
			Arguments: map[string]any{
				"latitude":  37.7749,
				"longitude": -122.4194,
				"zoom":      1,
				"format":    "json",
			},
		},
	}

	result, err := HandleGetMapImage(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil || len(result.Content) == 0 {
		t.Fatalf("unexpected empty result")
	}
}
