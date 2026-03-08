package mcp

// Server is a placeholder for the MCP (Model Context Protocol) server implementation.
// It will expose tools for AI coding agents to store and retrieve context chunks.
//
// Planned tools:
//   - ctx_store  — store a context chunk with embedding
//   - ctx_fetch  — fetch context by query_key or semantic similarity
//   - ctx_review — submit a usefulness/correctness review
//   - ctx_list   — list context chunks for a project
//   - ctx_delete — delete a context chunk

// Server holds dependencies for the MCP server.
type Server struct {
	// db    *pgxpool.Pool
	// embed *embeddings.Client
}

// NewServer creates a new MCP server instance.
func NewServer() *Server {
	return &Server{}
}
