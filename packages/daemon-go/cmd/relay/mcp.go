// mcp.go — Model Context Protocol server.
//
// Exposes Relay's HTTP API as MCP tools so any MCP-aware LLM client
// (Claude Desktop, Cursor, Cline, Continue, etc.) can drive the orchestrator
// without using the CLI or desktop app.
//
// Protocol: JSON-RPC 2.0 over stdio.
// Spec:     https://modelcontextprotocol.io
//
// Usage from a client, e.g. Claude Desktop's claude_desktop_config.json:
//
//   {
//     "mcpServers": {
//       "relay": {
//         "command": "relay",
//         "args": ["mcp"]
//       }
//     }
//   }
//
// Tools exposed:
//   relay_status        — get current session info
//   relay_providers     — list providers + states
//   relay_run_task      — start a new task
//   relay_handoff       — trigger immediate handoff
//   relay_retrieve      — search the code/graph for snippets
//   relay_diff          — current session worktree diff
//   relay_cost          — live spend
//   relay_send_reply    — send a stdin reply to the active adapter
//   relay_list_profiles — list profiles
//   relay_pause         — pause / resume the session

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ─── JSON-RPC types ──────────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP tool descriptor (subset used by tools/list).
type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ─── Tool table ──────────────────────────────────────────────────────────────

func tools() []mcpTool {
	str := func() map[string]interface{} { return map[string]interface{}{"type": "string"} }
	intT := func() map[string]interface{} { return map[string]interface{}{"type": "integer"} }
	obj := func(props map[string]interface{}, required ...string) map[string]interface{} {
		return map[string]interface{}{"type": "object", "properties": props, "required": required}
	}
	return []mcpTool{
		{
			Name:        "relay_status",
			Description: "Get current Relay session status: active provider, task, FSM state, tokens, HFS score.",
			InputSchema: obj(map[string]interface{}{}),
		},
		{
			Name:        "relay_providers",
			Description: "List Relay-managed providers with their probe status, quota usage, and next-in-chain flag.",
			InputSchema: obj(map[string]interface{}{}),
		},
		{
			Name:        "relay_run_task",
			Description: "Start a new Relay task. The orchestrator will route it via profile matcher and rotate across providers as quotas hit.",
			InputSchema: obj(map[string]interface{}{
				"task":      str(),
				"threshold": map[string]interface{}{"type": "number", "default": 0.85},
			}, "task"),
		},
		{
			Name:        "relay_handoff",
			Description: "Trigger an immediate handoff from the active provider to the next one in the chain.",
			InputSchema: obj(map[string]interface{}{}),
		},
		{
			Name:        "relay_retrieve",
			Description: "Search the indexed code chunks for relevant snippets. Returns top-K matches by FTS5 rank.",
			InputSchema: obj(map[string]interface{}{
				"query": str(),
				"limit": map[string]interface{}{"type": "integer", "default": 20},
			}, "query"),
		},
		{
			Name:        "relay_diff",
			Description: "Returns the current session's git diff against the user's main branch (worktree-isolated).",
			InputSchema: obj(map[string]interface{}{}),
		},
		{
			Name:        "relay_cost",
			Description: "Live cost for the current session: USD spend, tokens in/out, active provider.",
			InputSchema: obj(map[string]interface{}{}),
		},
		{
			Name:        "relay_send_reply",
			Description: "Send a stdin reply to the active adapter. Use when the agent is waiting on user input.",
			InputSchema: obj(map[string]interface{}{"reply": str()}, "reply"),
		},
		{
			Name:        "relay_list_profiles",
			Description: "List routing profiles with their chains, kinds, and skills.",
			InputSchema: obj(map[string]interface{}{}),
		},
		{
			Name:        "relay_pause",
			Description: "Pause or resume agent execution. Agents halt at the next event boundary.",
			InputSchema: obj(map[string]interface{}{
				"pause": map[string]interface{}{"type": "boolean"},
			}, "pause"),
		},
		{
			Name:        "relay_events",
			Description: "Get recent events from the session log. Use sinceId to paginate.",
			InputSchema: obj(map[string]interface{}{
				"sinceId": intT(),
			}),
		},
	}
}

// ─── HTTP bridge ─────────────────────────────────────────────────────────────

const mcpDaemonBase = "http://127.0.0.1:4748"

var mcpClient = &http.Client{Timeout: 8 * time.Second}

func mcpGet(path string) (interface{}, error) {
	resp, err := mcpClient.Get(mcpDaemonBase + path)
	if err != nil {
		return nil, fmt.Errorf("daemon not reachable on %s — start it with `relay daemon`", mcpDaemonBase)
	}
	defer resp.Body.Close()
	var out interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func mcpPostJSON(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	resp, err := mcpClient.Post(mcpDaemonBase+path, "application/json", strings.NewReader(string(b)))
	if err != nil {
		return nil, fmt.Errorf("daemon not reachable on %s", mcpDaemonBase)
	}
	defer resp.Body.Close()
	var out interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// ─── Tool dispatch ───────────────────────────────────────────────────────────

func dispatchTool(name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	case "relay_status":
		return mcpGet("/api/status")
	case "relay_providers":
		return mcpGet("/api/providers")
	case "relay_diff":
		return mcpGet("/api/session/diff")
	case "relay_cost":
		return mcpGet("/api/session/cost")
	case "relay_list_profiles":
		return mcpGet("/api/profiles")

	case "relay_retrieve":
		q, _ := args["query"].(string)
		if q == "" {
			return nil, fmt.Errorf("query required")
		}
		limit := 20
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		return mcpGet(fmt.Sprintf("/api/retrieval?q=%s&limit=%d",
			url.QueryEscape(q), limit))

	case "relay_events":
		since := int64(0)
		if v, ok := args["sinceId"].(float64); ok {
			since = int64(v)
		}
		return mcpGet(fmt.Sprintf("/api/events?since=%d", since))

	case "relay_run_task":
		task, _ := args["task"].(string)
		if task == "" {
			return nil, fmt.Errorf("task required")
		}
		threshold := 0.85
		if v, ok := args["threshold"].(float64); ok {
			threshold = v
		}
		return mcpPostJSON("/api/run", map[string]interface{}{
			"task": task, "threshold": threshold,
		})

	case "relay_handoff":
		return mcpPostJSON("/api/handoff", map[string]string{})

	case "relay_send_reply":
		reply, _ := args["reply"].(string)
		if reply == "" {
			return nil, fmt.Errorf("reply required")
		}
		return mcpPostJSON("/api/session/reply", map[string]string{"reply": reply})

	case "relay_pause":
		pause, _ := args["pause"].(bool)
		return mcpPostJSON("/api/session/pause", map[string]bool{"pause": pause})
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

// ─── Server loop ─────────────────────────────────────────────────────────────

// runMCPServer reads JSON-RPC frames from stdin, writes responses to stdout.
// One frame per line. No length-prefix framing — sufficient for most clients.
func runMCPServer() error {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	writeResp := func(r rpcResponse) {
		r.JSONRPC = "2.0"
		b, _ := json.Marshal(r)
		out.Write(b)        //nolint:errcheck
		out.WriteByte('\n') //nolint:errcheck
		out.Flush()         //nolint:errcheck
	}
	writeErr := func(id json.RawMessage, code int, msg string) {
		writeResp(rpcResponse{ID: id, Error: &rpcError{Code: code, Message: msg}})
	}

	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = []byte(strings.TrimSpace(string(line)))
		// Strip UTF-8 BOM if a sloppy client wrote one
		if len(line) >= 3 && line[0] == 0xEF && line[1] == 0xBB && line[2] == 0xBF {
			line = line[3:]
		}
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeErr(nil, -32700, "parse error: "+err.Error())
			continue
		}

		switch req.Method {
		case "initialize":
			writeResp(rpcResponse{ID: req.ID, Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]string{
					"name":    "relay-mcp",
					"version": "0.3.0",
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{"listChanged": false},
				},
			}})

		case "tools/list":
			writeResp(rpcResponse{ID: req.ID, Result: map[string]interface{}{
				"tools": tools(),
			}})

		case "tools/call":
			var params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeErr(req.ID, -32602, "invalid params: "+err.Error())
				continue
			}
			res, derr := dispatchTool(params.Name, params.Arguments)
			if derr != nil {
				writeResp(rpcResponse{ID: req.ID, Result: map[string]interface{}{
					"isError": true,
					"content": []map[string]string{
						{"type": "text", "text": derr.Error()},
					},
				}})
				continue
			}
			payload, _ := json.MarshalIndent(res, "", "  ")
			writeResp(rpcResponse{ID: req.ID, Result: map[string]interface{}{
				"content": []map[string]string{
					{"type": "text", "text": string(payload)},
				},
			}})

		case "ping":
			writeResp(rpcResponse{ID: req.ID, Result: map[string]string{}})

		case "notifications/initialized":
			// Client signalling it has finished init. No response needed.

		default:
			// Notifications have no ID. Don't respond.
			if len(req.ID) == 0 {
				continue
			}
			writeErr(req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

// ─── Cobra command ──────────────────────────────────────────────────────────

func cmdMCP() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run Relay as an MCP server (stdio JSON-RPC)",
		Long: `Exposes Relay's HTTP API as MCP tools over stdio.

Use from Claude Desktop / Cursor / Cline / any MCP client by adding:

  {
    "mcpServers": {
      "relay": {
        "command": "relay",
        "args": ["mcp"]
      }
    }
  }

Then your LLM can call relay_handoff, relay_run_task, relay_retrieve, etc.

Requires the Relay daemon to be running on :4748. Start it separately with
` + "`relay daemon`" + ` or open the desktop app.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPServer()
		},
	}
}
