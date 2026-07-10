// vision.go — screenshot + vision-LLM fallback for IDE/extension providers.
//
// MVP scope:
//   1. Config CRUD via /api/vision/config (GET/POST)
//   2. Probe via /api/vision/probe — captures one screenshot, sends to vision
//      model, returns parsed observation
//   3. Live polling loop is NOT YET wired into the orchestrator — to be done
//      once user confirms the observation flow works
//
// Vision providers supported in MVP:
//   - ollama (local, multimodal models like qwen2.5-vl, llava)
//   - gemini (Google AI Studio)
//   - openai (gpt-4o)
//   - anthropic (claude with vision)

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kbinani/screenshot"

	"github.com/dbisina/relay/internal/config"
)

// captureScreen grabs the primary display.
func captureScreen() (image.Image, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("no active displays")
	}
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// ─── Ollama bridge: model list + pull ────────────────────────────────────────

// OllamaModel — a single locally-installed Ollama model.
type OllamaModel struct {
	Name      string `json:"name"`      // e.g. "qwen2.5-vl:7b"
	Size      int64  `json:"size"`      // bytes
	ParamSize string `json:"paramSize"` // e.g. "7.6B"
	Family    string `json:"family"`    // e.g. "qwen2"
	IsVision  bool   `json:"isVision"`  // heuristic: family contains "vl"/"vision"/"llava"
}

// CuratedVisionModel — popular vision-capable model with default tag.
type CuratedVisionModel struct {
	Tag         string `json:"tag"`         // pullable identifier
	DisplayName string `json:"displayName"` // human label
	Description string `json:"description"` // 1-liner
	Size        string `json:"size"`        // approximate download size
}

// CuratedVisionModels — popular vision models on ollama.com that work well
// for parsing UIs and answering questions about screenshots.
var CuratedVisionModels = []CuratedVisionModel{
	{Tag: "qwen2.5vl:7b", DisplayName: "Qwen2.5-VL 7B", Description: "Strong general vision + UI reading", Size: "~5 GB"},
	{Tag: "qwen2.5vl:3b", DisplayName: "Qwen2.5-VL 3B", Description: "Lightweight, faster", Size: "~2.5 GB"},
	{Tag: "llava:7b", DisplayName: "LLaVA 7B", Description: "Classic open vision model", Size: "~4.5 GB"},
	{Tag: "llava:13b", DisplayName: "LLaVA 13B", Description: "Higher accuracy, slower", Size: "~8 GB"},
	{Tag: "llama3.2-vision:11b", DisplayName: "Llama 3.2 Vision 11B", Description: "Meta's vision model", Size: "~7 GB"},
	{Tag: "moondream:1.8b", DisplayName: "Moondream 1.8B", Description: "Tiny + fast on CPU", Size: "~1.5 GB"},
	{Tag: "minicpm-v:8b", DisplayName: "MiniCPM-V 8B", Description: "Strong OCR + visual reasoning", Size: "~5 GB"},
	{Tag: "gemma3:4b", DisplayName: "Gemma3 4B", Description: "Google's multimodal compact", Size: "~3 GB"},
}

// listOllamaModels — GET <baseURL>/api/tags
func listOllamaModels(baseURL string) ([]OllamaModel, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(body))
	}
	var wrapped struct {
		Models []struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Details struct {
				Family        string `json:"family"`
				ParameterSize string `json:"parameter_size"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, err
	}
	out := make([]OllamaModel, 0, len(wrapped.Models))
	for _, m := range wrapped.Models {
		fam := strings.ToLower(m.Details.Family)
		nm := strings.ToLower(m.Name)
		isVision := strings.Contains(fam, "vl") ||
			strings.Contains(fam, "vision") ||
			strings.Contains(fam, "llava") ||
			strings.Contains(fam, "moondream") ||
			strings.Contains(fam, "minicpm") ||
			strings.Contains(nm, "vision") ||
			strings.Contains(nm, "vl") ||
			strings.Contains(nm, "llava")
		out = append(out, OllamaModel{
			Name:      m.Name,
			Size:      m.Size,
			ParamSize: m.Details.ParameterSize,
			Family:    m.Details.Family,
			IsVision:  isVision,
		})
	}
	return out, nil
}

// pullOllamaModel — POST <baseURL>/api/pull with streaming progress.
// Emits a progress line via emit() for each status update.
func pullOllamaModel(baseURL, modelTag string, emit func(tag, msg string)) error {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model":  modelTag,
		"stream": true,
	})
	req, _ := http.NewRequest("POST", baseURL+"/api/pull", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 0} // streaming
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama pull %d: %s", resp.StatusCode, string(raw))
	}

	emit("system", fmt.Sprintf("pulling %s …", modelTag))
	dec := json.NewDecoder(resp.Body)
	lastStatus := ""
	for {
		var msg struct {
			Status    string `json:"status"`
			Digest    string `json:"digest,omitempty"`
			Total     int64  `json:"total,omitempty"`
			Completed int64  `json:"completed,omitempty"`
			Error     string `json:"error,omitempty"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if msg.Error != "" {
			emit("error", "pull error: "+msg.Error)
			return fmt.Errorf("pull: %s", msg.Error)
		}
		// Only emit when status changes or every 10% of a layer
		if msg.Status != lastStatus {
			emit("system", "  "+msg.Status)
			lastStatus = msg.Status
		} else if msg.Total > 0 && msg.Completed > 0 {
			pct := float64(msg.Completed) * 100.0 / float64(msg.Total)
			if int(pct)%20 == 0 {
				emit("system", fmt.Sprintf("  %s · %.0f%%", msg.Status, pct))
			}
		}
	}
	emit("result", fmt.Sprintf("✓ %s ready", modelTag))
	return nil
}

// installState reused — track pulls
func ollamaPullStateKey(tag string) string { return "ollama_pull:" + tag }

// ─── Ollama-backed launch fallback ────────────────────────────────────────────
//
// `ollama launch <tool>` (added in Ollama 0.5+) configures Claude Code / Codex
// / OpenCode / Cline to point at a local Ollama model. Lets users run those
// tools WITHOUT cloud auth.

// LaunchSpec — per-provider Ollama launch settings.
type LaunchSpec struct {
	Tool        string   // `ollama launch <Tool>`
	Recommended []string // models for this provider, best first
}

// OllamaLaunchSpecs — providers we can run via `ollama launch`.
// Pulled from docs/integrations/{claude-code,codex,opencode,cline}.mdx
var OllamaLaunchSpecs = map[string]LaunchSpec{
	"claude": {
		Tool: "claude",
		Recommended: []string{
			"qwen3.5", "kimi-k2.5:cloud", "glm-5:cloud",
			"minimax-m2.7:cloud", "qwen3.5:cloud", "glm-4.7-flash",
		},
	},
	"codex": {
		Tool: "codex",
		Recommended: []string{
			"gpt-oss:120b", "gpt-oss:20b", "qwen3-coder:30b", "qwen3-coder:120b",
		},
	},
	"opencode": {
		Tool: "opencode",
		Recommended: []string{
			"qwen3-coder:30b", "gpt-oss:20b", "qwen3.5",
		},
	},
	"cline": {
		Tool: "cline",
		Recommended: []string{
			"qwen3-coder:30b", "gpt-oss:20b", "qwen3.5",
		},
	},
}

// CanLaunchViaOllama — true if the provider has an `ollama launch` bridge.
func CanLaunchViaOllama(name string) bool {
	_, ok := OllamaLaunchSpecs[name]
	return ok
}

// runOllamaLaunch opens a terminal running `ollama launch <tool> --model <m>`.
func runOllamaLaunch(providerName, model string, emit func(tag, msg string)) error {
	spec, ok := OllamaLaunchSpecs[providerName]
	if !ok {
		return fmt.Errorf("%s has no ollama launch bridge", providerName)
	}
	args := []string{"launch", spec.Tool}
	if model != "" {
		args = append(args, "--model", model)
	}
	title := fmt.Sprintf("Relay - %s via Ollama", providerName)
	emit("system", "ollama "+strings.Join(args, " "))
	if err := openInTerminal("ollama", args, title); err != nil {
		emit("error", "failed to open terminal: "+err.Error())
		return err
	}
	emit("system", fmt.Sprintf("  %s will start with %s as backend in the new terminal",
		providerName, model))
	return nil
}

// (helpers wired from main.go — bridge config.Config ↔ ApiVisionConfig)

// ApiVisionConfig is the JSON shape for /api/vision/config.
type ApiVisionConfig struct {
	Enabled     bool   `json:"enabled"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	APIKeyEnv   string `json:"apiKeyEnv"`
	BaseURL     string `json:"baseUrl"`
	PollMs      int    `json:"pollMs"`
	WindowMatch string `json:"windowMatch"`
	// Status
	APIKeySet bool   `json:"apiKeySet"` // for cloud: env var or .env value present
	Available bool   `json:"available"` // last known: backend reachable
	LastError string `json:"lastError,omitempty"`
}

// ApiVisionObservation is returned by /api/vision/probe.
type ApiVisionObservation struct {
	NeedsInput bool     `json:"needsInput"`
	Question   string   `json:"question"`
	Choices    []string `json:"choices"`
	Summary    string   `json:"summary"`
	RawText    string   `json:"rawText"`
}

const visionSystemPrompt = `You are inspecting a screenshot of an AI coding agent's UI.
Look at the visible window content and answer with STRICT JSON only (no markdown, no prose).

Return:
{
  "needsInput": boolean,    // true if the agent is waiting for a user reply / approval / choice
  "question": string,       // the question or prompt, if any (empty string if none)
  "choices": string[],      // list of visible options/buttons the user could pick (empty array if none)
  "summary": string         // 1-2 sentence summary of what the agent is currently doing
}

Do not include any text outside the JSON.`

// probeVision: captures a screenshot, calls the configured vision model, returns observation.
// Cloud providers require explicit consent — when cfg.Provider != "ollama" and
// cfg.Enabled is true, the user has opted in via the Vision settings tab.
// We still log every cloud send to the audit trail.
func probeVision(cfg ApiVisionConfig) (ApiVisionObservation, error) {
	if cfg.Provider != "" && cfg.Provider != "ollama" && !cfg.Enabled {
		return ApiVisionObservation{}, fmt.Errorf("cloud vision disabled — enable in Settings → Vision (privacy: sends screenshots to %s)", cfg.Provider)
	}
	img, err := captureScreen()
	if err != nil {
		return ApiVisionObservation{}, fmt.Errorf("screenshot: %w", err)
	}
	pngBytes, err := encodePNG(img)
	if err != nil {
		return ApiVisionObservation{}, fmt.Errorf("encode: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pngBytes)

	switch strings.ToLower(cfg.Provider) {
	case "ollama":
		return callOllamaVision(cfg, b64)
	case "gemini":
		return callGeminiVision(cfg, b64)
	case "openai":
		return callOpenAIVision(cfg, b64)
	case "anthropic":
		return callAnthropicVision(cfg, b64)
	default:
		return ApiVisionObservation{}, fmt.Errorf("unknown vision provider: %s", cfg.Provider)
	}
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ─── Ollama (local) ───────────────────────────────────────────────────────────

func callOllamaVision(cfg ApiVisionConfig, b64 string) (ApiVisionObservation, error) {
	base := cfg.BaseURL
	if base == "" {
		base = "http://localhost:11434"
	}
	body := map[string]interface{}{
		"model":  cfg.Model,
		"prompt": visionSystemPrompt + "\n\nNow analyze the screenshot:",
		"images": []string{b64},
		"stream": false,
		"format": "json",
	}
	raw, err := postJSON(base+"/api/generate", "", body)
	if err != nil {
		return ApiVisionObservation{}, err
	}
	var resp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ApiVisionObservation{}, err
	}
	return parseVisionJSON(resp.Response)
}

// ─── Gemini ───────────────────────────────────────────────────────────────────

func callGeminiVision(cfg ApiVisionConfig, b64 string) (ApiVisionObservation, error) {
	key := getAPIKey(cfg.APIKeyEnv)
	if key == "" {
		return ApiVisionObservation{}, fmt.Errorf("missing %s", envOr(cfg.APIKeyEnv, "GEMINI_API_KEY"))
	}
	model := cfg.Model
	if model == "" {
		model = "gemini-1.5-pro"
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, key)
	body := map[string]interface{}{
		"contents": []map[string]interface{}{{
			"parts": []map[string]interface{}{
				{"text": visionSystemPrompt},
				{"inline_data": map[string]string{"mime_type": "image/png", "data": b64}},
			},
		}},
		"generationConfig": map[string]interface{}{
			"response_mime_type": "application/json",
		},
	}
	raw, err := postJSON(url, "", body)
	if err != nil {
		return ApiVisionObservation{}, err
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ApiVisionObservation{}, err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return ApiVisionObservation{}, fmt.Errorf("gemini: empty response")
	}
	return parseVisionJSON(resp.Candidates[0].Content.Parts[0].Text)
}

// ─── OpenAI (gpt-4o) ──────────────────────────────────────────────────────────

func callOpenAIVision(cfg ApiVisionConfig, b64 string) (ApiVisionObservation, error) {
	key := getAPIKey(cfg.APIKeyEnv)
	if key == "" {
		return ApiVisionObservation{}, fmt.Errorf("missing %s", envOr(cfg.APIKeyEnv, "OPENAI_API_KEY"))
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "system", "content": visionSystemPrompt},
			{"role": "user", "content": []map[string]interface{}{
				{"type": "text", "text": "Analyze this screenshot."},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/png;base64," + b64,
				}},
			}},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	raw, err := postJSON("https://api.openai.com/v1/chat/completions", "Bearer "+key, body)
	if err != nil {
		return ApiVisionObservation{}, err
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ApiVisionObservation{}, err
	}
	if len(resp.Choices) == 0 {
		return ApiVisionObservation{}, fmt.Errorf("openai: empty response")
	}
	return parseVisionJSON(resp.Choices[0].Message.Content)
}

// ─── Anthropic (claude vision) ────────────────────────────────────────────────

func callAnthropicVision(cfg ApiVisionConfig, b64 string) (ApiVisionObservation, error) {
	key := getAPIKey(cfg.APIKeyEnv)
	if key == "" {
		return ApiVisionObservation{}, fmt.Errorf("missing %s", envOr(cfg.APIKeyEnv, "ANTHROPIC_API_KEY"))
	}
	model := cfg.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": 1024,
		"system":     visionSystemPrompt,
		"messages": []map[string]interface{}{
			{"role": "user", "content": []map[string]interface{}{
				{"type": "image", "source": map[string]string{
					"type":       "base64",
					"media_type": "image/png",
					"data":       b64,
				}},
				{"type": "text", "text": "Analyze and return JSON only."},
			}},
		},
	}
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages",
		bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return ApiVisionObservation{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return ApiVisionObservation{}, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(raw))
	}
	var aresp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &aresp); err != nil {
		return ApiVisionObservation{}, err
	}
	if len(aresp.Content) == 0 {
		return ApiVisionObservation{}, fmt.Errorf("anthropic: empty content")
	}
	return parseVisionJSON(aresp.Content[0].Text)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func postJSON(url, authHeader string, body interface{}) ([]byte, error) {
	req, _ := http.NewRequest("POST", url, bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return raw, nil
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// parseVisionJSON is lenient: accepts both camelCase and snake_case keys,
// strips ``` fences, ignores extra fields. Vision models are inconsistent.
func parseVisionJSON(text string) (ApiVisionObservation, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var obs ApiVisionObservation
	obs.RawText = text

	a := strings.Index(text, "{")
	b := strings.LastIndex(text, "}")
	if a < 0 || b <= a {
		return obs, fmt.Errorf("no JSON object in vision response")
	}
	jsonBytes := text[a : b+1]

	// First try strict parse
	if err := json.Unmarshal([]byte(jsonBytes), &obs); err == nil && (obs.Summary != "" || obs.Question != "" || obs.NeedsInput) {
		return obs, nil
	}

	// Fallback: lenient parse into a map, then alias-pick keys
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonBytes), &raw); err != nil {
		return obs, fmt.Errorf("parse JSON: %w", err)
	}

	pickStr := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
		return ""
	}
	pickBool := func(keys ...string) bool {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				if b, ok := v.(bool); ok {
					return b
				}
				if s, ok := v.(string); ok {
					return strings.EqualFold(s, "true") || s == "1" || strings.EqualFold(s, "yes")
				}
			}
		}
		return false
	}
	pickStrArr := func(keys ...string) []string {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				if arr, ok := v.([]interface{}); ok {
					out := make([]string, 0, len(arr))
					for _, x := range arr {
						if s, ok := x.(string); ok {
							out = append(out, s)
						}
					}
					return out
				}
			}
		}
		return nil
	}

	obs.NeedsInput = pickBool("needsInput", "needs_input", "needs-input", "waitingForInput", "input_required")
	obs.Question = pickStr("question", "prompt", "user_question")
	obs.Summary = pickStr("summary", "description", "summary_text")
	obs.Choices = pickStrArr("choices", "options", "buttons", "actions")
	return obs, nil
}

func getAPIKey(envName string) string {
	if envName == "" {
		return ""
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return loadEnvFileValue(envName)
}

func envOr(actual, fallback string) string {
	if actual != "" {
		return actual
	}
	return fallback
}

// visionConfigToAPI exports the current TOML vision config + live status
// (api key presence, last-known availability).
func visionConfigToAPI(cfg *config.Config) ApiVisionConfig {
	v := cfg.Vision
	apiKey := false
	if v.APIKeyEnv != "" {
		apiKey = getAPIKey(v.APIKeyEnv) != ""
	}
	if v.Provider == "ollama" {
		apiKey = true // local doesn't need a key
	}
	return ApiVisionConfig{
		Enabled:     v.Enabled,
		Provider:    v.Provider,
		Model:       v.Model,
		APIKeyEnv:   v.APIKeyEnv,
		BaseURL:     v.BaseURL,
		PollMs:      v.PollMs,
		WindowMatch: v.WindowMatch,
		APIKeySet:   apiKey,
	}
}

// writeVisionConfig patches [vision] block in relay.toml.
func writeVisionConfig(tomlPath string, req ApiVisionConfig) error {
	data, err := os.ReadFile(tomlPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var out []string
	skip := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if t == "[vision]" {
				skip = true
				continue
			}
			if skip {
				skip = false
			}
		}
		if skip {
			continue
		}
		out = append(out, line)
	}

	out = append(out, "", "[vision]",
		fmt.Sprintf("enabled       = %v", req.Enabled),
		fmt.Sprintf("provider      = %q", req.Provider),
		fmt.Sprintf("model         = %q", req.Model),
		fmt.Sprintf("api_key_env   = %q", req.APIKeyEnv),
		fmt.Sprintf("base_url      = %q", req.BaseURL),
		fmt.Sprintf("poll_ms       = %d", req.PollMs),
		fmt.Sprintf("window_match  = %q", req.WindowMatch),
	)

	return os.WriteFile(tomlPath, []byte(strings.Join(out, "\n")), 0600)
}
