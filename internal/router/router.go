package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	mlld "github.com/mlld-lang/mlld/sdk/go"
)

// ResponseMode indicates how an agent should respond to a mention.
type ResponseMode string

const (
	ModeDeepWork     ResponseMode = "deep-work"     // Full session with comprehensive context
	ModeWeighIn      ResponseMode = "weigh-in"      // Opinion/review with thread context
	ModeQuickAnswer  ResponseMode = "quick-answer"  // Short answer with minimal context
	ModeAcknowledge  ResponseMode = "acknowledge"   // FYI - no spawn needed
)

// RouterResult is the output from the mlld router.
type RouterResult struct {
	Mode        ResponseMode `json:"mode"`
	ShouldSpawn bool         `json:"shouldSpawn"`
	Confidence  float64      `json:"confidence"`
}

// RouterPayload is the input to the mlld router.
type RouterPayload struct {
	Message string  `json:"message"` // The mention message body
	From    string  `json:"from"`    // Who sent the message
	Agent   string  `json:"agent"`   // The mentioned agent
	Thread  *string `json:"thread"`  // Thread context (nil if room)
}

// Router wraps the mlld router for daemon command dispatch.
type Router struct {
	client     *mlld.Client
	routerPath string
	available  bool
}

// New creates a new Router for the given fray project.
// Returns a Router that gracefully degrades if mlld is unavailable.
func New(frayDir string) *Router {
	routerPath := filepath.Join(frayDir, "llm", "router.mld")

	// Check if router file exists
	if _, err := os.Stat(routerPath); os.IsNotExist(err) {
		return &Router{available: false}
	}

	client := mlld.New()
	client.Timeout = 10 * time.Second
	client.WorkingDir = frayDir

	return &Router{
		client:     client,
		routerPath: routerPath,
		available:  true,
	}
}

// Available returns true if the router is available for use.
func (r *Router) Available() bool {
	return r.available
}

// Route determines the response mode for a given mention.
// Returns a default result if routing fails (graceful degradation).
func (r *Router) Route(payload RouterPayload) RouterResult {
	// Default: deep-work with medium confidence
	defaultResult := RouterResult{
		Mode:        ModeDeepWork,
		ShouldSpawn: true,
		Confidence:  0.5,
	}

	if !r.available {
		return defaultResult
	}

	// Execute the router with payload
	result, err := r.client.Execute(r.routerPath, payload, nil)
	if err != nil {
		// Log error but don't fail - return default
		fmt.Fprintf(os.Stderr, "[router] execute error: %v\n", err)
		return defaultResult
	}

	// Parse JSON output
	var routerResult RouterResult
	if err := json.Unmarshal([]byte(result.Output), &routerResult); err != nil {
		fmt.Fprintf(os.Stderr, "[router] parse error: %v (output: %s)\n", err, result.Output)
		return defaultResult
	}

	return routerResult
}
