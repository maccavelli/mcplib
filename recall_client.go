// Package mcplib provides shared infrastructure for MCP server implementations.
package mcplib

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RecallClient wraps the official go-sdk Streamable HTTP transport for connecting
// to mcp-server-recall. Provides circuit-breaker-protected search, save, get,
// list, and batch operations with sharded async telemetry.
//
// This is the canonical shared implementation; individual servers should use
// a type alias: type MCPClient = mcplib.RecallClient
type RecallClient struct {
	URL             string
	serverName      string
	mu              sync.Mutex
	session         *mcp.ClientSession
	recallEnabled   bool
	logger          *slog.Logger
	telemetryShards [8]chan telemetryEvent
	wg              sync.WaitGroup
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	closed          atomic.Bool
	closeOnce       sync.Once
}

type telemetryEvent struct {
	sessionID string
	projectID string
	namespace string
	payload   any
}

// NewRecallClient initializes a new Streamable HTTP MCP client for recall.
// serverName identifies this client in logs and telemetry (e.g., "go-refactor").
func NewRecallClient(url, serverName string) *RecallClient {
	ctx, cancel := context.WithCancel(context.Background())
	client := &RecallClient{
		URL:             url,
		serverName:      serverName,
		logger:          slog.Default(),
		lifecycleCtx:    ctx,
		lifecycleCancel: cancel,
	}
	for i := range 8 {
		client.telemetryShards[i] = make(chan telemetryEvent, 2048)
		client.wg.Add(1)
		go client.workerFlusher(i)
	}
	return client
}

// RecallEnabled safely returns true if the connection to Recall is fully established.
func (c *RecallClient) RecallEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recallEnabled
}

func (c *RecallClient) setRecallEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recallEnabled = enabled
}

// Start connects to the remote MCP server using Streamable HTTP transport.
// It runs initialization (handshake + capability discovery) and sets the
// circuit breaker to enabled on success. Auto-reconnects on crash detection.
func (c *RecallClient) Start(parent context.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("recall Start goroutine panic recovered", "server", c.serverName, "panic", r)
		}
	}()
	// Tie the reconnect loop to the lifecycle context so Close() stops it.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	context.AfterFunc(c.lifecycleCtx, cancel)

	for {
		if ctx.Err() != nil {
			return
		}

		connected := false
		backoff := 100 * time.Millisecond
		maxBackoff := 5 * time.Second

		// Rapid circuit breaker without arbitrary loop ceilings
		for {
			if ctx.Err() != nil {
				return
			}

			client := mcp.NewClient(
				&mcp.Implementation{
					Name:    c.serverName,
					Version: "1.0.0",
				},
				nil,
			)

			transport := &mcp.StreamableClientTransport{
				Endpoint: c.URL,
			}

			session, err := client.Connect(ctx, transport, nil)
			if err != nil {
				c.logger.Debug("[RECALL] connection attempt failed",
					"server", c.serverName,
					"url", c.URL,
					"error", err,
					"retry_in", backoff,
				)
				c.setRecallEnabled(false)

				backoffTimer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					backoffTimer.Stop()
					return
				case <-backoffTimer.C:
				}
				backoff = min(backoff*2, maxBackoff)
				continue
			}

			c.mu.Lock()
			c.session = session
			c.mu.Unlock()

			c.setRecallEnabled(true)
			connected = true
			c.logger.Info("Streamable HTTP connection established", "url", c.URL, "session_id", session.ID())

			// Setup channel to wait on session closure
			errCh := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						c.logger.Error("recall session.Wait goroutine panic recovered", "panic", r)
					}
				}()
				errCh <- session.Wait()
			}()

			// Block until connection closes or context is cancelled
			select {
			case <-ctx.Done():
				c.logger.Info("Context cancelled, closing Streamable HTTP session", "url", c.URL)
				session.Close()
				c.setRecallEnabled(false)
				return
			case err := <-errCh:
				if err != nil {
					c.logger.Warn("Streamable HTTP session closed unexpectedly (crash detected)",
						"server", c.serverName, "url", c.URL, "error", err)
				}
				c.setRecallEnabled(false)
				connected = false
			}
			break // break attempt loop to trigger reconnect
		}

		if !connected {
			c.setRecallEnabled(false)
			c.logger.Warn("Recall connection dropped, attempting to reconnect in 5s...")
			reconnTimer := time.NewTimer(5 * time.Second)
			select {
			case <-ctx.Done():
				reconnTimer.Stop()
				return
			case <-reconnTimer.C:
			}
		}
	}
}

// CallDatabaseTool generically executes remote database tools and marshals their
// structured data results.
//
// CONTRACT: it returns "" on BOTH an empty result AND when recall is unavailable
// (circuit breaker open / not connected). Errors are logged, not propagated.
// Callers that must distinguish "no results" from "recall down" should check
// RecallEnabled() or use the error-returning CallDatabaseToolE.
func (c *RecallClient) CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]any) string {
	result, err := c.CallDatabaseToolE(ctx, toolName, arguments)
	if err != nil {
		c.logger.Warn("recall_call_failed", "tool", toolName, "error", err)
	}
	return result
}

// CallDatabaseToolE is the error-returning variant of CallDatabaseTool.
// CLI tools and other callers that need explicit error propagation should use this method.
func (c *RecallClient) CallDatabaseToolE(ctx context.Context, toolName string, arguments map[string]any) (string, error) {
	if !c.RecallEnabled() {
		return "", fmt.Errorf("recall unavailable: circuit breaker active")
	}

	c.mu.Lock()
	session := c.session
	c.mu.Unlock()

	if session == nil {
		return "", fmt.Errorf("recall unavailable: session not initialized")
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		return "", fmt.Errorf("recall RPC failed: %w", err)
	}

	if result == nil {
		return "", fmt.Errorf("recall returned nil result")
	}

	// Surface server-side tool errors (IsError=true).
	if result.IsError {
		errText := "unknown error"
		for _, ct := range result.Content {
			if tc, ok := ct.(*mcp.TextContent); ok && tc.Text != "" {
				errText = tc.Text
				break
			}
		}
		return "", fmt.Errorf("recall tool error: %s", errText)
	}

	// Prefer structured content for machine-to-machine consumption (e.g., recall)
	if result.StructuredContent != nil {
		if sc, ok := result.StructuredContent.(map[string]any); ok {
			if data, exists := sc["data"]; exists {
				j, jErr := json.Marshal(data)
				if jErr != nil {
					return "", fmt.Errorf("recall structured data marshal failed: %w", jErr)
				}
				return string(j), nil
			}
			j, jErr := json.Marshal(sc)
			if jErr != nil {
				return "", fmt.Errorf("recall structured content marshal failed: %w", jErr)
			}
			return string(j), nil
		}
	}

	// Fallback: extract text content if structured content is missing or invalid
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok && tc.Text != "" {
			c.logger.Info("API command text content fallback executed", "target", "recall", "tool", toolName)
			return tc.Text, nil
		}
	}

	return "", fmt.Errorf("recall returned no parseable content (Content=%d, StructuredContent=%v)", len(result.Content), result.StructuredContent != nil)
}

// ---------------------------------------------------------------------------
// Save / Telemetry
// ---------------------------------------------------------------------------

// SaveToRecall persists context state into the remote recall server.
// projectID is the project-scoped key (e.g., project root path) used for recall's compound key.
func (c *RecallClient) SaveToRecall(ctx context.Context, sessionID, projectID string, payload any) error {
	return c.SaveToRecallWithNamespace(ctx, sessionID, projectID, "", payload)
}

// SaveToRecallWithNamespace persists context state to a specific recall namespace.
// namespace controls domain routing ("sessions", "server_status", "dialectic_history", etc.). Empty defaults to "sessions".
func (c *RecallClient) SaveToRecallWithNamespace(ctx context.Context, sessionID, projectID, namespace string, payload any) error {
	if c.closed.Load() {
		return fmt.Errorf("recall client closed")
	}

	shardIdx := 0
	if len(sessionID) > 0 {
		shardIdx = int(sessionID[0]) % 8
	}

	select {
	case c.telemetryShards[shardIdx] <- telemetryEvent{sessionID: sessionID, projectID: projectID, namespace: namespace, payload: payload}:
		return nil
	default:
		c.logger.Warn("Telemetry queue full, shedding payload", "session_id", sessionID, "project_id", projectID, "shard", shardIdx)
		return fmt.Errorf("telemetry queue full")
	}
}

func (c *RecallClient) workerFlusher(shard int) {
	defer c.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("recall flusher goroutine panic recovered", "shard", shard, "server", c.serverName, "panic", r)
		}
	}()
	// Terminate via the lifecycle context (Close cancels it). The shard channel
	// is never closed, so concurrent senders can never panic; on shutdown we
	// best-effort drain whatever is already queued.
	for {
		select {
		case <-c.lifecycleCtx.Done():
			for {
				select {
				case event := <-c.telemetryShards[shard]:
					c.flushEvent(shard, event)
				default:
					return
				}
			}
		case event := <-c.telemetryShards[shard]:
			c.flushEvent(shard, event)
		}
	}
}

// flushEvent pushes a single telemetry event to recall with bounded retries.
func (c *RecallClient) flushEvent(shard int, event telemetryEvent) {
	{
		var payloadBytes []byte
		if event.payload != nil {
			payloadBytes, _ = json.Marshal(event.payload)
		}
		if len(payloadBytes) == 0 || string(payloadBytes) == "null" {
			payloadBytes = []byte("{}")
		}
		sID := event.sessionID
		if sID == "" {
			sID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
		}
		pID := event.projectID
		if pID == "" {
			pID = "global"
		}
		args := map[string]any{
			"server_id":     c.serverName,
			"project_id":    pID,
			"outcome":       "saved",
			"session_id":    sID,
			"model":         "native",
			"token_spend":   0,
			"trace_context": "async_push",
			"state_data":    string(payloadBytes),
		}
		if event.namespace != "" {
			args["namespace"] = event.namespace
		}

		retries := 15
		backoff := 100 * time.Millisecond

		for i := range retries {
			// Diagnostic: capture state BEFORE the call.
			enabled := c.RecallEnabled()
			c.mu.Lock()
			hasSession := c.session != nil
			c.mu.Unlock()

			if !enabled || !hasSession {
				c.logger.Warn("flusher_pre_call_state",
					"shard", shard,
					"attempt", i+1,
					"recall_enabled", enabled,
					"has_session", hasSession,
					"server", c.serverName,
				)
			}

			callCtx, callCancel := context.WithTimeout(c.lifecycleCtx, 30*time.Second)
			res, callErr := c.CallDatabaseToolE(callCtx, "save_to_recall", args)
			callCancel()
			if callErr == nil && res != "" {
				if i > 0 {
					c.logger.Info("flusher_recovered",
						"shard", shard,
						"attempt", i+1,
						"server", c.serverName,
					)
				}
				break
			}

			// Diagnostic: structured error trace on every failure.
			errMsg := ""
			if callErr != nil {
				errMsg = callErr.Error()
			}
			c.logger.Warn("flusher_call_failed",
				"shard", shard,
				"attempt", i+1,
				"max_attempts", retries,
				"error", errMsg,
				"result_empty", res == "",
				"recall_enabled", enabled,
				"has_session", hasSession,
				"server", c.serverName,
				"session_id", sID,
				"namespace", event.namespace,
			)

			if i == retries-1 {
				c.logger.Error("flusher_exhausted",
					"shard", shard,
					"attempts", retries,
					"server", c.serverName,
					"session_id", sID,
					"last_error", errMsg,
					"args_keys", fmt.Sprintf("%v", mapsKeys(args)),
				)
			}
			select {
			case <-c.lifecycleCtx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 2*time.Second {
				backoff = 2 * time.Second
			}
		}
	}
}

// mapsKeys returns sorted keys from a map for diagnostic logging.
func mapsKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// Session Retrieval
// ---------------------------------------------------------------------------

// GetSession retrieves context state from the remote recall server.
// The sessionID corresponds to the CSSA pipeline correlation ID (typically the project root).
func (c *RecallClient) GetSession(ctx context.Context, sessionID string) (map[string]any, error) {
	args := map[string]any{
		"session_id": sessionID,
		"namespace":  "sessions",
	}
	res := c.CallDatabaseTool(ctx, "get", args)
	if res == "" {
		if !c.RecallEnabled() {
			return nil, fmt.Errorf("recall_unreachable: recall circuit breaker is open, cannot retrieve session '%s'", sessionID)
		}
		return nil, fmt.Errorf("recall_key_not_found: no session data found for key '%s' — verify the session_id matches an existing recall entry", sessionID)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(res), &data); err != nil {
		return nil, fmt.Errorf("recall_data_corrupt: session '%s' returned unparseable data: %w", sessionID, err)
	}
	return data, nil
}

// AggregateSessionFromRecall retrieves ALL session records for a given server_id
// and project_id from recall's universal list endpoint, then merges their state_data
// into a unified map. This is the correct aggregation path for generate_final_report
// which needs to combine data from all pipeline stages.
//
// Limits results to the 20 most recent entries and truncates per-stage content
// at 32KB to prevent orchestrator payload overflow (25MB limit).
func (c *RecallClient) AggregateSessionFromRecall(ctx context.Context, serverID, projectID string) (map[string]any, error) {
	const maxEntries = 20       // Most recent N entries per server_id
	const maxContentLen = 32768 // 32KB per stage content cap

	if !c.RecallEnabled() {
		return nil, fmt.Errorf("recall_unreachable: recall circuit breaker is open, cannot aggregate sessions")
	}

	args := map[string]any{
		"server_id":        serverID,
		"project_id":       projectID,
		"limit":            maxEntries,
		"truncate_content": true,
		"namespace":        "sessions",
	}
	res := c.CallDatabaseTool(ctx, "list", args)
	if res == "" {
		return nil, fmt.Errorf("recall_key_not_found: no sessions found for server_id='%s' project_id='%s'", serverID, projectID)
	}

	// Parse the list response.
	// CallDatabaseTool may return either:
	//   (a) Unwrapped: {"count":N, "entries":[...]} — when StructuredContent has "data" key
	//   (b) Wrapped:   {"data":{"count":N, "entries":[...]}} — when TextContent fallback is used
	var envelope map[string]any
	if err := json.Unmarshal([]byte(res), &envelope); err != nil {
		return nil, fmt.Errorf("recall_data_corrupt: list returned unparseable data: %w", err)
	}

	// Handle both unwrapped (entries at top level) and wrapped (entries inside "data").
	var entries []any
	if e, ok := envelope["entries"].([]any); ok {
		// Case (a): CallDatabaseTool already unwrapped the "data" field
		entries = e
	} else if data, ok := envelope["data"].(map[string]any); ok {
		// Case (b): full response envelope with "data" wrapper
		entries, _ = data["entries"].([]any)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("recall_key_not_found: no session entries for server_id='%s' project_id='%s'", serverID, projectID)
	}

	// Limit to most recent N entries (recall returns chronologically ordered).
	totalEntries := len(entries)
	if totalEntries > maxEntries {
		entries = entries[totalEntries-maxEntries:]
	}

	// Merge all stage state_data into a unified result map.
	merged := make(map[string]any)
	merged["_total_entries"] = totalEntries
	merged["_stage_count"] = len(entries)

	stages := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		key, _ := e["key"].(string)
		record, _ := e["record"].(map[string]any)
		if record == nil {
			continue
		}
		content, _ := record["content"].(string)
		if content == "" {
			continue
		}

		// Parse the state_data JSON stored in the content field.
		var stageData map[string]any
		if json.Unmarshal([]byte(content), &stageData) == nil {
			if len(content) > maxContentLen {
				delete(stageData, "files")
				delete(stageData, "diff")
				delete(stageData, "stdout")
				delete(stageData, "stderr")
				stageData["_pruned"] = true
			}
			stages = append(stages, stageData)
			// Merge top-level fields from each stage into the unified map.
			maps.Copy(merged, stageData)
		}

		c.logger.Debug("AggregateSessionFromRecall: merged stage", "key", key)
	}

	merged["_stages"] = stages
	c.logger.Info("AggregateSessionFromRecall success",
		"server_id", serverID,
		"project_id", projectID,
		"total_avail", totalEntries,
		"stages_merged", len(stages),
	)

	return merged, nil
}

// ---------------------------------------------------------------------------
// Search (Optimized)
// ---------------------------------------------------------------------------

// SearchOpts encapsulates optional parameters for domain-scoped searches.
type SearchOpts struct {
	Query        string
	Namespace    string
	Package      string
	Category     string
	SymbolType   string
	Interface    string
	Receiver     string
	Domain       string
	Tag          string
	Limit        int
	KeyPrefix    string
	KeySuffix    string
	Tags         []string
	TagMatchMode string
	MetadataOnly bool
}

// SearchScoped queries recall with structured search parameters.
// Consolidates all namespace-aware search operations into a single method.
// Returns "" on both empty results and recall unavailability (see CallDatabaseTool).
func (c *RecallClient) SearchScoped(ctx context.Context, opts SearchOpts) string {
	args := map[string]any{"query": opts.Query}
	if opts.Namespace != "" {
		args["namespace"] = opts.Namespace
	}
	if opts.Tag != "" {
		args["tag"] = opts.Tag
	}
	if opts.SymbolType != "" {
		args["symbol_type"] = opts.SymbolType
	}
	if opts.Category != "" {
		args["category"] = opts.Category
	}
	if opts.Package != "" {
		args["package"] = opts.Package
	}
	if opts.Interface != "" {
		args["interface"] = opts.Interface
	}
	if opts.Receiver != "" {
		args["receiver"] = opts.Receiver
	}
	if opts.Domain != "" {
		args["domain"] = opts.Domain
	}
	if opts.Tag != "" {
		args["tag"] = opts.Tag
	}
	if opts.KeyPrefix != "" {
		args["key_prefix"] = opts.KeyPrefix
	}
	if opts.KeySuffix != "" {
		args["key_suffix"] = opts.KeySuffix
	}
	if len(opts.Tags) > 0 {
		args["tags"] = opts.Tags
	}
	if opts.TagMatchMode != "" {
		args["tag_match_mode"] = opts.TagMatchMode
	}
	if opts.MetadataOnly {
		args["metadata_only"] = true
	}
	if opts.Limit == 0 {
		args["limit"] = 10000 // 0 means unlimited
	} else {
		args["limit"] = opts.Limit
	}
	return c.CallDatabaseTool(ctx, "search", args)
}

// SearchStandards queries the recall standards namespace with multi-dimensional
// BM25 search. Filters are optional — pass empty string to skip.
// Returns "" on both empty results and recall unavailability (see CallDatabaseTool).
func (c *RecallClient) SearchStandards(ctx context.Context, query, pkg, symbolType string, limit int) string {
	return c.SearchScoped(ctx, SearchOpts{
		Query:      query,
		Namespace:  "standards",
		Package:    pkg,
		SymbolType: symbolType,
		Limit:      limit,
	})
}

// SearchSessions queries the recall sessions namespace with multi-dimensional
// BM25/Jaccard search. Filters are optional — pass empty string to skip.
// Returns "" on both empty results and recall unavailability (see CallDatabaseTool).
func (c *RecallClient) SearchSessions(ctx context.Context, query, projectID, serverID, outcome, traceContext string, limit int) string {
	args := map[string]any{"query": query, "namespace": "sessions"}
	if limit > 0 {
		args["limit"] = limit
	}
	if projectID != "" {
		args["project_id"] = projectID
	}
	if serverID != "" {
		args["server_id"] = serverID
	}
	if outcome != "" {
		args["outcome"] = outcome
	}
	if traceContext != "" {
		args["trace_context"] = traceContext
	}
	return c.CallDatabaseTool(ctx, "search", args)
}

// SearchAndFetch executes a SearchScoped query, extracts the document keys from the
// resulting JSON envelope, and executes a subsequent BatchGet to retrieve the full
// record payloads in a single atomic operation. Returns a map of DocumentKey -> Content.
func (c *RecallClient) SearchAndFetch(ctx context.Context, opts SearchOpts) (map[string]string, error) {
	if !c.RecallEnabled() {
		return nil, fmt.Errorf("recall_unreachable: circuit breaker is open")
	}

	searchRes := c.SearchScoped(ctx, opts)
	if searchRes == "" || (len(searchRes) > 5 && searchRes[:5] == "Error") {
		return nil, fmt.Errorf("search failed or returned empty: %s", searchRes)
	}

	var envelope struct {
		Count   int `json:"count"`
		Entries []struct {
			Key     string `json:"key"`
			Content string `json:"content"`
		} `json:"entries"`
	}

	if err := json.Unmarshal([]byte(searchRes), &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal search envelope: %w", err)
	}

	if len(envelope.Entries) == 0 {
		return map[string]string{}, nil
	}

	keys := make([]string, 0, len(envelope.Entries))
	result := make(map[string]string, len(envelope.Entries))

	for _, e := range envelope.Entries {
		if e.Content != "" {
			result[e.Key] = e.Content
		} else {
			keys = append(keys, e.Key)
		}
	}

	if len(keys) == 0 {
		return result, nil
	}

	fetched, err := c.FetchKeys(ctx, opts.Namespace, keys)
	if err != nil {
		return nil, err
	}

	maps.Copy(result, fetched)

	return result, nil
}

// FetchKeys retrieves a specific list of keys in chunks, returning a map of DocumentKey -> Content.
func (c *RecallClient) FetchKeys(ctx context.Context, namespace string, keys []string) (map[string]string, error) {
	if !c.RecallEnabled() {
		return nil, fmt.Errorf("recall_unreachable: circuit breaker is open")
	}

	result := make(map[string]string, len(keys))
	const chunkSize = 100

	for i := 0; i < len(keys); i += chunkSize {
		end := min(i+chunkSize, len(keys))
		chunk := keys[i:end]

		batchRes := c.BatchGet(ctx, namespace, chunk)
		if batchRes == "" || (len(batchRes) > 5 && batchRes[:5] == "Error") {
			return nil, fmt.Errorf("batch_get chunk failed or returned empty: %s", batchRes)
		}

		var batchData struct {
			Entries map[string]struct {
				Content string `json:"content"`
			} `json:"entries"`
		}

		if err := json.Unmarshal([]byte(batchRes), &batchData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal batch_get result: %w", err)
		}

		for k, v := range batchData.Entries {
			result[k] = v.Content
		}
	}

	return result, nil
}

// SearchMetadata retrieves only keys and hashes for a given query to minimize payload size over HTTP.
// Returns a map of DocumentKey -> Hash.
func (c *RecallClient) SearchMetadata(ctx context.Context, opts SearchOpts) (map[string]string, error) {
	if !c.RecallEnabled() {
		return nil, fmt.Errorf("recall_unreachable: circuit breaker is open")
	}

	opts.MetadataOnly = true
	searchRes := c.SearchScoped(ctx, opts)
	if searchRes == "" || (len(searchRes) > 5 && searchRes[:5] == "Error") {
		return nil, fmt.Errorf("search metadata failed: %s", searchRes)
	}

	var envelope struct {
		Entries []struct {
			Key  string `json:"key"`
			Hash string `json:"hash"`
		} `json:"entries"`
	}

	if err := json.Unmarshal([]byte(searchRes), &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal search metadata envelope: %w", err)
	}

	result := make(map[string]string, len(envelope.Entries))
	for _, e := range envelope.Entries {
		if e.Hash == "" {
			result[e.Key] = "unknown"
		} else {
			result[e.Key] = e.Hash
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Batch
// ---------------------------------------------------------------------------

// BatchGet retrieves multiple records by key from a non-memories namespace
// in a single atomic read. Returns the raw JSON response string.
// Returns "" on both empty results and recall unavailability (see CallDatabaseTool).
func (c *RecallClient) BatchGet(ctx context.Context, namespace string, keys []string) string {
	return c.CallDatabaseTool(ctx, "get", map[string]any{
		"namespace": namespace,
		"keys":      keys,
	})
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListSessionsByFilter retrieves sessions matching specified criteria with
// truncation enabled to prevent payload overflow. All filters are optional.
// Returns "" on both empty results and recall unavailability (see CallDatabaseTool).
func (c *RecallClient) ListSessionsByFilter(ctx context.Context, projectID, serverID, outcome string, limit int) string {
	args := map[string]any{"truncate_content": true, "namespace": "sessions"}
	if limit > 0 {
		args["limit"] = limit
	}
	if projectID != "" {
		args["project_id"] = projectID
	}
	if serverID != "" {
		args["server_id"] = serverID
	}
	if outcome != "" {
		args["outcome"] = outcome
	}
	return c.CallDatabaseTool(ctx, "list", args)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

// Close gracefully shuts down the async telemetry workers with a 2-second timeout,
// ensuring final payloads are flushed without blocking pipeline teardowns.
//
// SHUTDOWN ORDERING INVARIANT: This method MUST execute before the
// SetupStandardLogging cleanup closure. The telemetry workers call slog.Warn
// during their final drain, which writes to the AsyncWriter channel. If
// AsyncWriter.Close() runs first, those writes are lost. Go's LIFO defer stack
// naturally enforces this when SetupStandardLogging is deferred before
// RecallClient initialization in main().
func (c *RecallClient) Close() {
	c.closeOnce.Do(func() {
		c.closed.Store(true) // reject new telemetry sends
		c.lifecycleCancel()  // stop reconnect loop + unblock workers; workers drain via this signal
		c.logger.Info("mcpclient shutting down, flushing async telemetry queues...")

		// NOTE: the shard channels are intentionally NOT closed — that would race
		// concurrent SaveToRecallWithNamespace callers and panic. Workers exit on
		// lifecycleCtx cancellation (see workerFlusher) after a best-effort drain.

		done := make(chan struct{})
		go func() {
			c.wg.Wait()
			close(done)
		}()

		// Hard 2-second timeout to prevent zombie processes if Recall is hanging
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		select {
		case <-done:
			c.logger.Info("telemetry flushed successfully")
		case <-ctx.Done():
			c.logger.Warn("telemetry flush timeout exceeded, terminating forcibly")
		}

		c.mu.Lock()
		if c.session != nil {
			c.session.Close()
		}
		c.mu.Unlock()
	})
}

// ---------------------------------------------------------------------------
// Orchestrator Convenience API (CheckApprovalExists, GetOperationalProfile)
// ---------------------------------------------------------------------------

// ListStandardsCategories returns hierarchical package/symbol overview from the
// recall standards namespace. Optionally filter by package prefix.
// Returns "" on both empty results and recall unavailability (see CallDatabaseTool).
func (c *RecallClient) ListStandardsCategories(ctx context.Context, pkg string) string {
	args := map[string]any{}
	if pkg != "" {
		args["package"] = pkg
	}
	args["namespace"] = "standards_categories"
	return c.CallDatabaseTool(ctx, "list", args)
}

// CheckApprovalExists verifies that a brainstorm approval with a matching
// plan_hash exists in the recall sessions namespace for the given project.
func (c *RecallClient) CheckApprovalExists(ctx context.Context, projectID, planHash string) (bool, error) {
	if !c.RecallEnabled() {
		return false, fmt.Errorf("recall_unreachable: circuit breaker is open")
	}
	res := c.ListSessionsByFilter(ctx, projectID, "brainstorm", "approved", 10)
	if res == "" {
		return false, fmt.Errorf("recall_key_not_found: no approval records for project '%s'", projectID)
	}
	if !strings.Contains(res, planHash) {
		return false, nil
	}
	return true, nil
}

// OperationalProfile aggregates runtime metrics computed from historical session data.
type OperationalProfile struct {
	AvgTokenSpend      float64            `json:"avg_token_spend"`
	ServerFailureRates map[string]float64 `json:"server_failure_rates"`
	SessionCount       int                `json:"session_count"`
	AvgCacheMissRate   float64            `json:"avg_cache_miss_rate"`
	AvgMemoryMB        float64            `json:"avg_memory_mb"`
}

// GetOperationalProfile queries recall for recent session telemetry and computes
// an aggregated operational profile for runtime self-tuning.
func (c *RecallClient) GetOperationalProfile(ctx context.Context, limit int) *OperationalProfile {
	if !c.RecallEnabled() {
		return nil
	}

	if limit <= 0 {
		limit = 20
	}

	raw := c.CallDatabaseTool(ctx, "list", map[string]any{
		"namespace": "server_status",
		"server_id": "magictools",
		"limit":     limit,
	})
	if raw == "" {
		return nil
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil
	}

	var entries []any
	if e, ok := envelope["entries"].([]any); ok {
		entries = e
	} else if data, ok := envelope["data"].(map[string]any); ok {
		entries, _ = data["entries"].([]any)
	}
	if len(entries) == 0 {
		return nil
	}

	profile := &OperationalProfile{
		ServerFailureRates: make(map[string]float64),
		SessionCount:       len(entries),
	}

	var totalTokens float64
	var tokenEntries int
	serverTotal := make(map[string]int)
	serverFailed := make(map[string]int)

	for _, entryRaw := range entries {
		entry, ok := entryRaw.(map[string]any)
		if !ok {
			continue
		}
		record, _ := entry["record"].(map[string]any)
		if record == nil {
			continue
		}

		if ts, ok := record["token_spend"].(float64); ok && ts > 0 {
			totalTokens += ts
			tokenEntries++
		}

		serverID, _ := record["server_id"].(string)
		outcome, _ := record["outcome"].(string)
		if serverID != "" {
			serverTotal[serverID]++
			if outcome == "error" || outcome == "failed" || outcome == "timeout" {
				serverFailed[serverID]++
			}
		}
	}

	if tokenEntries > 0 {
		profile.AvgTokenSpend = totalTokens / float64(tokenEntries)
	}

	for server, total := range serverTotal {
		if total > 0 {
			profile.ServerFailureRates[server] = float64(serverFailed[server]) / float64(total)
		}
	}

	// Parse heartbeat state_data for cache miss rate and memory trends
	var totalMissRate float64
	var missRateEntries int
	var totalMemoryMB float64
	var memEntries int
	for _, entryRaw := range entries {
		entry, ok := entryRaw.(map[string]any)
		if !ok {
			continue
		}
		record, _ := entry["record"].(map[string]any)
		if record == nil {
			continue
		}
		content, _ := record["content"].(string)
		if content == "" {
			continue
		}
		var stateData map[string]any
		if json.Unmarshal([]byte(content), &stateData) != nil {
			continue
		}
		// Extract cache miss rate from heartbeat state_data
		if cacheMap, ok := stateData["cache"].(map[string]any); ok {
			if align, ok := cacheMap["align_cache"].(map[string]any); ok {
				hits, _ := align["hits"].(float64)
				misses, _ := align["misses"].(float64)
				if hits+misses > 0 {
					totalMissRate += misses / (hits + misses)
					missRateEntries++
				}
			}
		}
		// Extract memory RSS from heartbeat state_data
		if sysMap, ok := stateData["system"].(map[string]any); ok {
			if mem, ok := sysMap["memory_rss_mb"].(float64); ok && mem > 0 {
				totalMemoryMB += mem
				memEntries++
			}
		}
	}
	if missRateEntries > 0 {
		profile.AvgCacheMissRate = totalMissRate / float64(missRateEntries)
	}
	if memEntries > 0 {
		profile.AvgMemoryMB = totalMemoryMB / float64(memEntries)
	}

	return profile
}

// WaitForRecallServer blocks until the recall client connects or the timeout expires.
func WaitForRecallServer(ctx context.Context, client *RecallClient, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		if client.RecallEnabled() {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timeout waiting for recall server to become ready")
		case <-ticker.C:
		}
	}
}
