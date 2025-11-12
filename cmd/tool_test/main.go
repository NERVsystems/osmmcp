package main

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	// Create an error result
	result := mcp.NewToolResultError("Test error message")

	// Marshal to JSON to see the structure
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling: %v\n", err)
		return
	}

	fmt.Println("Error Result JSON:")
	fmt.Println(string(jsonBytes))

	// Print content directly
	fmt.Println("\nContent:")
	for i, content := range result.Content {
		fmt.Printf("Content[%d]: %#v\n", i, content)
		if textContent, ok := content.(mcp.TextContent); ok {
			fmt.Printf("  Text: %q\n", textContent.Text)
			fmt.Printf("  Type: %q\n", textContent.Type)
		}
	}
}
