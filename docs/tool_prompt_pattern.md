# Tool Prompt Registration Pattern

This repository demonstrates a pattern for providing AI assistants with guidance on how to use each tool. The key steps are:

1. **Define prompt handlers**

   Each prompt ID has a handler function that returns `mcp.GetPromptResult`. The handler assembles `PromptMessage` objects describing how the tool should be used.

   ```go
   func GeocodeAddressExamplesHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
       text := `EXAMPLES OF EFFECTIVE GEOCODE_ADDRESS USAGE:\n...`
       return mcp.NewGetPromptResult(
           "Geocode Address Examples",
           []mcp.PromptMessage{
               mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(text)),
           },
       ), nil
   }
   ```

2. **Register prompts with the MCP server**

   During server initialization the registry adds each prompt with `AddPrompt` and provides a short description. This allows the server to fetch prompt text by ID when requested.

   ```go
   s.AddPrompt(mcp.NewPrompt("geocoding",
       mcp.WithPromptDescription("Instructions for properly using geocoding tools"),
   ), GeocodingPromptHandler)
   ```

3. **Create prompt templates**

   Prompt templates let the server proactively deliver instructions without requiring the agent to call a tool first. A template is composed of one or more `PromptMessage` objects and can be attached to the server at startup.

   ```go
   template := mcp.NewPromptTemplate("geocoding",
       []mcp.PromptMessage{
           mcp.NewPromptMessage(mcp.RoleSystem, mcp.NewTextContent(prompts.GeocodingSystemPrompt())),
       },
   )
   srv.AddPromptTemplate(template)
   ```

4. **Use the registry to register tools and prompts together**

   A central registry simplifies adding all tools and associated prompts to the server.

   ```go
   registry := tools.NewRegistry(logger)
   registry.RegisterAll(srv)
   ```

With this approach each tool can have dedicated prompts explaining its usage and best practices. Other MCP servers can adopt the same pattern: write prompt handlers for each tool, register them with `AddPrompt`, and optionally provide a prompt template so the AI receives instructions at session start.
