package aap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TrustLevel indicates the verification status of an identity.
type TrustLevel string

const (
	TrustLevelFull      TrustLevel = "full"      // Has verified keypair
	TrustLevelUntrusted TrustLevel = "untrusted" // Legacy, no keys
)

// InvokeConfig holds driver-specific configuration for spawning agents.
// Matches fray's existing types.InvokeConfig.
type InvokeConfig struct {
	Driver         string         `json:"driver,omitempty"`
	Model          string         `json:"model,omitempty"`
	Trust          []string       `json:"trust,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
	PromptDelivery string         `json:"prompt_delivery,omitempty"`
	SpawnTimeoutMs int64          `json:"spawn_timeout_ms,omitempty"`
	IdleAfterMs    int64          `json:"idle_after_ms,omitempty"`
	MinCheckinMs   int64          `json:"min_checkin_ms,omitempty"`
	MaxRuntimeMs   int64          `json:"max_runtime_ms,omitempty"`
}

// VariantContext holds project/role-specific configuration.
type VariantContext struct {
	Variant        string   `json:"variant"`
	ContextURI     string   `json:"context_uri,omitempty"`
	ContextRef     string   `json:"context_ref,omitempty"`
	ContextSHA     string   `json:"context_sha,omitempty"`
	TrustOverrides []string `json:"trust_overrides,omitempty"`
}

// Resolution is the result of resolving an address.
type Resolution struct {
	Identity      *Identity
	TrustLevel    TrustLevel
	Invoke        *InvokeConfig
	Variants      map[string]*VariantContext
	RegistryHosts []string
	Source        string // "aap", "fray", "remote"
}

// Resolver resolves addresses to identities.
type Resolver interface {
	// Resolve looks up an address and returns the full resolution.
	Resolve(addr string) (*Resolution, error)

	// ResolveAddress is like Resolve but takes a parsed Address.
	ResolveAddress(addr Address) (*Resolution, error)
}

// ResolverOpts configures resolver behavior.
type ResolverOpts struct {
	// GlobalRegistry is the path to ~/.config/aap/
	GlobalRegistry string

	// ProjectRegistry is the path to .aap/ in the project
	ProjectRegistry string

	// FrayCompat enables fallback to .fray/agents.jsonl
	FrayCompat bool

	// FrayPath is the path to .fray/ directory
	FrayPath string
}

// DefaultResolver implements Resolver with local and project resolution.
type DefaultResolver struct {
	opts           ResolverOpts
	globalReg      *FileRegistry
	projectReg     *FileRegistry
	frayAgents     map[string]*FrayAgent // cached fray agents
	frayAgentsPath string                // path to agents.jsonl
}

// NewResolver creates a new resolver.
func NewResolver(opts ResolverOpts) (Resolver, error) {
	r := &DefaultResolver{
		opts: opts,
	}

	// Initialize global registry if path provided
	if opts.GlobalRegistry != "" {
		agentsDir := filepath.Join(opts.GlobalRegistry, "agents")
		if _, err := os.Stat(agentsDir); err == nil {
			reg, err := NewFileRegistry(agentsDir)
			if err != nil {
				return nil, fmt.Errorf("init global registry: %w", err)
			}
			r.globalReg = reg
		}
	}

	// Initialize project registry if path provided
	if opts.ProjectRegistry != "" {
		agentsDir := filepath.Join(opts.ProjectRegistry, "agents")
		if _, err := os.Stat(agentsDir); err == nil {
			reg, err := NewFileRegistry(agentsDir)
			if err != nil {
				return nil, fmt.Errorf("init project registry: %w", err)
			}
			r.projectReg = reg
		}
	}

	// Load fray agents if compat enabled
	if opts.FrayCompat && opts.FrayPath != "" {
		r.frayAgentsPath = filepath.Join(opts.FrayPath, "agents.jsonl")
		if _, err := os.Stat(r.frayAgentsPath); err == nil {
			agents, err := LoadFrayAgents(r.frayAgentsPath)
			if err != nil {
				return nil, fmt.Errorf("load fray agents: %w", err)
			}
			r.frayAgents = agents
		}
	}

	return r, nil
}

// Resolve looks up an address string and returns the full resolution.
func (r *DefaultResolver) Resolve(addr string) (*Resolution, error) {
	parsed, err := Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}
	return r.ResolveAddress(parsed)
}

// ResolveAddress resolves a parsed address to an identity.
func (r *DefaultResolver) ResolveAddress(addr Address) (*Resolution, error) {
	agent := addr.Agent

	// Handle @host case
	if addr.Host != "" {
		return r.resolveWithHost(addr)
	}

	// Local resolution order (per AAP-SPEC section 3.1):
	// 1. Global registry
	// 2. Project registry
	// 3. Fray compat (if enabled)

	// Try global registry
	if r.globalReg != nil {
		identity, err := r.globalReg.Get(agent)
		if err == nil {
			res := r.buildResolution(identity, addr, "aap")
			return res, nil
		}
	}

	// Try project registry
	if r.projectReg != nil {
		identity, err := r.projectReg.Get(agent)
		if err == nil {
			res := r.buildResolution(identity, addr, "aap")
			return res, nil
		}
	}

	// Try fray compat
	if r.opts.FrayCompat && r.frayAgents != nil {
		if frayAgent, ok := r.frayAgents[agent]; ok {
			identity := FrayAgentToIdentity(frayAgent)
			res := &Resolution{
				Identity:   identity,
				TrustLevel: TrustLevelUntrusted,
				Source:     "fray",
			}
			if frayAgent.Invoke != nil {
				res.Invoke = &InvokeConfig{
					Driver:         frayAgent.Invoke.Driver,
					Model:          frayAgent.Invoke.Model,
					Trust:          frayAgent.Invoke.Trust,
					Config:         frayAgent.Invoke.Config,
					PromptDelivery: frayAgent.Invoke.PromptDelivery,
					SpawnTimeoutMs: frayAgent.Invoke.SpawnTimeoutMs,
					IdleAfterMs:    frayAgent.Invoke.IdleAfterMs,
					MinCheckinMs:   frayAgent.Invoke.MinCheckinMs,
					MaxRuntimeMs:   frayAgent.Invoke.MaxRuntimeMs,
				}
			}
			return res, nil
		}
	}

	return nil, fmt.Errorf("agent %q not found", agent)
}

// resolveWithHost handles addresses with @host component.
func (r *DefaultResolver) resolveWithHost(addr Address) (*Resolution, error) {
	agent := addr.Agent
	host := addr.Host

	// Try global registry first
	if r.globalReg != nil {
		identity, err := r.globalReg.Get(agent)
		if err == nil {
			// Check if host is in registry_hosts
			res := r.buildResolution(identity, addr, "aap")
			if res.RegistryHosts != nil && containsHost(res.RegistryHosts, host) {
				return res, nil
			}
			// Host not in registry - this is an error (remote resolution not implemented)
		}
	}

	// Remote resolution not implemented (Phase 6)
	return nil, fmt.Errorf("agent %q@%s not found (remote resolution not implemented)", agent, host)
}

// buildResolution creates a Resolution from an identity.
func (r *DefaultResolver) buildResolution(identity *Identity, addr Address, source string) *Resolution {
	res := &Resolution{
		Identity: identity,
		Source:   source,
	}

	if identity.HasKey {
		res.TrustLevel = TrustLevelFull
	} else {
		res.TrustLevel = TrustLevelUntrusted
	}

	// Load invoke config if available
	invoke := r.loadInvokeConfig(addr.Agent)
	if invoke != nil {
		res.Invoke = invoke
	}

	// Load variants if requested
	if len(addr.Variants) > 0 {
		res.Variants = r.loadVariants(addr.Agent, addr.Variants)
	}

	// Load registry hosts
	res.RegistryHosts = r.loadRegistryHosts(addr.Agent)

	return res
}

// loadInvokeConfig loads invoke configuration for an agent.
func (r *DefaultResolver) loadInvokeConfig(agent string) *InvokeConfig {
	// Check global registry
	if r.opts.GlobalRegistry != "" {
		invokePath := filepath.Join(r.opts.GlobalRegistry, "agents", agent, "invoke.json")
		if data, err := os.ReadFile(invokePath); err == nil {
			var invoke InvokeConfig
			if err := json.Unmarshal(data, &invoke); err == nil {
				return &invoke
			}
		}
	}

	// Check project registry
	if r.opts.ProjectRegistry != "" {
		invokePath := filepath.Join(r.opts.ProjectRegistry, "agents", agent, "invoke.json")
		if data, err := os.ReadFile(invokePath); err == nil {
			var invoke InvokeConfig
			if err := json.Unmarshal(data, &invoke); err == nil {
				return &invoke
			}
		}
	}

	return nil
}

// loadVariants loads variant contexts for an agent.
func (r *DefaultResolver) loadVariants(agent string, variants []string) map[string]*VariantContext {
	result := make(map[string]*VariantContext)

	for _, variant := range variants {
		// Check global registry
		if r.opts.GlobalRegistry != "" {
			contextPath := filepath.Join(r.opts.GlobalRegistry, "variants", agent, variant, "context.json")
			if data, err := os.ReadFile(contextPath); err == nil {
				var vc VariantContext
				if err := json.Unmarshal(data, &vc); err == nil {
					result[variant] = &vc
					continue
				}
			}
		}

		// Check project registry
		if r.opts.ProjectRegistry != "" {
			contextPath := filepath.Join(r.opts.ProjectRegistry, "variants", agent, variant, "context.json")
			if data, err := os.ReadFile(contextPath); err == nil {
				var vc VariantContext
				if err := json.Unmarshal(data, &vc); err == nil {
					result[variant] = &vc
				}
			}
		}
	}

	return result
}

// loadRegistryHosts loads the registry_hosts list for an agent.
func (r *DefaultResolver) loadRegistryHosts(agent string) []string {
	// Check global registry for hosts.json
	if r.opts.GlobalRegistry != "" {
		hostsPath := filepath.Join(r.opts.GlobalRegistry, "agents", agent, "hosts.json")
		if data, err := os.ReadFile(hostsPath); err == nil {
			var hosts []string
			if err := json.Unmarshal(data, &hosts); err == nil {
				return hosts
			}
		}
	}
	return nil
}

// containsHost checks if a host is in the registry hosts list.
func containsHost(hosts []string, host string) bool {
	normalized := "@" + host
	for _, h := range hosts {
		if h == normalized || h == host {
			return true
		}
	}
	return false
}
