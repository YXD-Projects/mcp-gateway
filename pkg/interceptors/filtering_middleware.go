package interceptors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/docker/mcp-gateway/pkg/query"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	sessionID     string
	sessionIDLock sync.RWMutex
)

type IndexRequest struct {
	Tools     []*mcp.Tool `json:"tools"` // Changed to []*mcp.Tool
	SessionID *string     `json:"session_id,omitempty"`
}

type IndexResponse struct {
	SessionID string `json:"session_id"`
}

type FilterRequest struct {
	SessionID string `json:"session_id"`
	Query     string `json:"query"`
}

func FilterToolsMiddleware(filterPort int) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// Only intercept tools/list
			if method == "tools/list" {
				logf(">>> tools/list intercepted - indexing and filtering\n")

				// Get the real tools first
				result, err := next(ctx, method, req)
				if err != nil {
					return result, err
				}

				// Type-assert to ListToolsResult
				if toolsResult, ok := result.(*mcp.ListToolsResult); ok {
					originalCount := len(toolsResult.Tools)
					logf(">>> Got %d tools\n", originalCount)

					// Step 1: Index the tools
					indexURL := fmt.Sprintf("http://localhost:%d/index_tools", filterPort)
					filterURL := fmt.Sprintf("http://localhost:%d/filter-tools", filterPort)
					sid, err := indexTools(toolsResult, indexURL)
					if err != nil {
						logf(">>> Error indexing tools: %v, returning original\n", err)
						return toolsResult, nil
					}
					logf(">>> Tools indexed with session_id: %s\n", sid)

					// Step 2: Filter the tools using the query
					filteredResult, err := filterTools(sid, query.GetLatestQuery(), filterURL)
					if err != nil {
						logf(">>> Error filtering tools: %v, returning original\n", err)
						return toolsResult, nil
					}

					logf(">>> Filtered tools returned: %d (originally %d)\n", len(filteredResult.Tools), originalCount)
					return filteredResult, nil
				}

				return result, err
			}

			// For all other methods, pass through
			return next(ctx, method, req)
		}
	}
}

func indexTools(toolsResult *mcp.ListToolsResult, indexEndpoint string) (string, error) {
	// Check if we already have a session ID for this set of tools
	sessionIDLock.RLock()
	existingSID := sessionID
	sessionIDLock.RUnlock()

	// Create index request
	indexReq := IndexRequest{
		Tools: toolsResult.Tools, // Now matches []*mcp.Tool
	}

	// Reuse existing session ID if available
	if existingSID != "" {
		indexReq.SessionID = &existingSID
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(indexReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal index request: %w", err)
	}

	// Send to index endpoint
	resp, err := http.Post(indexEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to send index request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("index endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read and unmarshal response
	var indexResp IndexResponse
	if err := json.NewDecoder(resp.Body).Decode(&indexResp); err != nil {
		return "", fmt.Errorf("failed to decode index response: %w", err)
	}

	// Store session ID for reuse
	sessionIDLock.Lock()
	sessionID = indexResp.SessionID
	sessionIDLock.Unlock()

	return indexResp.SessionID, nil
}

func filterTools(sid, query, filterEndpoint string) (*mcp.ListToolsResult, error) {
	// Create filter request
	filterReq := FilterRequest{
		SessionID: sid,
		Query:     query,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(filterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal filter request: %w", err)
	}

	// Send to filter endpoint
	resp, err := http.Post(filterEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send filter request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("filter endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read and unmarshal response
	var filteredResult mcp.ListToolsResult
	if err := json.NewDecoder(resp.Body).Decode(&filteredResult); err != nil {
		return nil, fmt.Errorf("failed to decode filter response: %w", err)
	}

	return &filteredResult, nil
}
