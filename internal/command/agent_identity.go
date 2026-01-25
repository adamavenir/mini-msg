package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adamavenir/fray/internal/aap"
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

func NewAgentAvatarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "avatar <name> <avatar>",
		Short: "Set agent avatar character",
		Long: `Set the avatar character for an agent. The avatar is displayed in chat bylines.

Examples:
  fray agent avatar opus 🅾
  fray agent avatar designer 🅳
  fray agent avatar helper ✿`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			avatar := args[1]

			agent, err := db.GetAgent(cmdCtx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			// Validate avatar
			if !core.IsValidAvatar(avatar) {
				return writeCommandError(cmd, fmt.Errorf("invalid avatar: %s (use a single character or emoji)", avatar))
			}

			// Update in database
			updates := db.AgentUpdates{
				Avatar: types.OptionalString{Set: true, Value: &avatar},
			}
			if err := db.UpdateAgent(cmdCtx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			// Append to JSONL
			if err := db.AppendAgentUpdate(cmdCtx.Project.DBPath, db.AgentUpdateJSONLRecord{
				AgentID: agentID,
				Avatar:  &avatar,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"avatar":   avatar,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Updated @%s avatar to %s\n", agentID, avatar)
			return nil
		},
	}

	return cmd
}

func init() {
	_ = os.Getenv("FRAY_AGENT_ID")
}

// NewAgentResolveCmd resolves an address using AAP resolution.
func NewAgentResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <address>",
		Short: "Resolve an agent address using AAP",
		Long: `Resolve an agent address to show identity, trust level, and invoke config.

Examples:
  fray agent resolve @dev           # Basic resolution
  fray agent resolve @dev.frontend  # With variant
  fray agent resolve @dev@server    # With host`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			addr := args[0]

			aapDir, err := core.AAPConfigDir()
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("get AAP config dir: %w", err))
			}

			projectAAPDir := filepath.Join(cmdCtx.Project.Root, ".aap")
			frayDir := filepath.Dir(cmdCtx.Project.DBPath)

			resolver, err := aap.NewResolver(aap.ResolverOpts{
				GlobalRegistry:  aapDir,
				ProjectRegistry: projectAAPDir,
				FrayCompat:      true,
				FrayPath:        frayDir,
			})
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("create resolver: %w", err))
			}

			res, err := resolver.Resolve(addr)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("resolve: %w", err))
			}

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"address":     addr,
					"agent":       res.Identity.Record.Agent,
					"guid":        res.Identity.Record.GUID,
					"trust_level": res.TrustLevel,
					"has_key":     res.Identity.HasKey,
					"source":      res.Source,
					"invoke":      res.Invoke,
				})
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Address: %s\n", addr)
			fmt.Fprintf(out, "Agent: %s\n", res.Identity.Record.Agent)
			fmt.Fprintf(out, "GUID: %s\n", res.Identity.Record.GUID)
			fmt.Fprintf(out, "Trust Level: %s\n", res.TrustLevel)
			fmt.Fprintf(out, "Has Key: %v\n", res.Identity.HasKey)
			fmt.Fprintf(out, "Source: %s\n", res.Source)
			if res.Invoke != nil && res.Invoke.Driver != "" {
				fmt.Fprintf(out, "Driver: %s\n", res.Invoke.Driver)
			}
			if res.Identity.HasKey {
				keyID := aap.KeyFingerprint(res.Identity.PublicKey)
				fmt.Fprintf(out, "Key ID: %s\n", keyID)
			}

			return nil
		},
	}

	return cmd
}

// NewAgentIdentityCmd shows AAP identity for an agent.
func NewAgentIdentityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identity <name>",
		Short: "Show AAP identity and public key fingerprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])

			aapDir, err := core.AAPConfigDir()
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("get AAP config dir: %w", err))
			}

			registry, err := aap.NewFileRegistry(filepath.Join(aapDir, "agents"))
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("create registry: %w", err))
			}

			identity, err := registry.Get(agentID)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found in AAP registry: %w", err))
			}

			if cmdCtx.JSONMode {
				output := map[string]any{
					"agent":      identity.Record.Agent,
					"guid":       identity.Record.GUID,
					"address":    identity.Record.Address,
					"has_key":    identity.HasKey,
					"created_at": identity.Record.CreatedAt,
				}
				if identity.HasKey {
					output["key_id"] = aap.KeyFingerprint(identity.PublicKey)
				}
				if identity.Record.Metadata != nil {
					output["metadata"] = identity.Record.Metadata
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Agent: %s\n", identity.Record.Agent)
			fmt.Fprintf(out, "GUID: %s\n", identity.Record.GUID)
			fmt.Fprintf(out, "Address: %s\n", identity.Record.Address)
			fmt.Fprintf(out, "Created: %s\n", identity.Record.CreatedAt)
			if identity.HasKey {
				keyID := aap.KeyFingerprint(identity.PublicKey)
				fmt.Fprintf(out, "Key ID: %s\n", keyID)
			} else {
				fmt.Fprintln(out, "Key: none (use 'fray agent keygen' to generate)")
			}

			return nil
		},
	}

	return cmd
}

// NewAgentKeygenCmd generates a keypair for an existing agent.
func NewAgentKeygenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keygen <name>",
		Short: "Generate keypair for an existing agent",
		Long: `Generate an Ed25519 keypair for an existing AAP identity.
The agent must already have an AAP identity (created via 'fray new').

You will be prompted for a passphrase to encrypt the private key.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])

			aapDir, err := core.AAPConfigDir()
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("get AAP config dir: %w", err))
			}

			registry, err := aap.NewFileRegistry(filepath.Join(aapDir, "agents"))
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("create registry: %w", err))
			}

			// Check if agent exists
			existing, err := registry.Get(agentID)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found in AAP registry - run 'fray new %s' first", agentID))
			}

			if existing.HasKey {
				return writeCommandError(cmd, fmt.Errorf("agent @%s already has a keypair", agentID))
			}

			// Prompt for passphrase
			passphrase, err := promptPassphrase("Enter passphrase for new key: ")
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("read passphrase: %w", err))
			}

			// Delete and re-register with key (simplest approach given current API)
			if err := registry.Delete(agentID); err != nil {
				return writeCommandError(cmd, fmt.Errorf("prepare for keygen: %w", err))
			}

			identity, err := registry.Register(agentID, aap.RegisterOpts{
				GenerateKey: true,
				Passphrase:  passphrase,
				Metadata:    existing.Record.Metadata,
			})
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("generate key: %w", err))
			}

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent":  agentID,
					"guid":   identity.Record.GUID,
					"key_id": aap.KeyFingerprint(identity.PublicKey),
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Generated keypair for @%s\n", agentID)
			fmt.Fprintf(cmd.OutOrStdout(), "  Key ID: %s\n", aap.KeyFingerprint(identity.PublicKey))

			return nil
		},
	}

	return cmd
}
