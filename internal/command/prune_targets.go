package command

import (
	"database/sql"
	"strings"
)

// resolvePruneTarget resolves a prune target to a home value and optional thread name.
// Returns (home, threadName, error).
func resolvePruneTarget(dbConn *sql.DB, target string) (string, string, error) {
	target = strings.TrimSpace(strings.ToLower(target))

	// Main room aliases
	if target == "" || target == "main" || target == "room" {
		return "room", "", nil
	}

	// Try to resolve as thread
	thread, err := resolveThreadRef(dbConn, target)
	if err != nil {
		return "", "", err
	}

	return thread.GUID, thread.Name, nil
}

type pruneProtectionOpts struct {
	// ProtectReplies: if true (default), messages with replies are protected
	ProtectReplies bool
	// ProtectFaves: if true (default), faved messages are protected
	ProtectFaves bool
	// ProtectReacts: if true (default), messages with reactions are protected
	ProtectReacts bool
	// RequireReplies: if true, only prune messages that have replies
	RequireReplies bool
	// RequireFaves: if true, only prune messages that have faves
	RequireFaves bool
	// RequireReacts: if true, only prune messages that have reactions
	RequireReacts bool
}

// parsePruneProtectionOpts parses --with and --without flags into protection options.
// --with removes protections (e.g., --with faves allows pruning faved messages)
// --without filters to only prune items lacking those attributes
func parsePruneProtectionOpts(withFlags, withoutFlags []string) pruneProtectionOpts {
	opts := pruneProtectionOpts{
		ProtectReplies: true,
		ProtectFaves:   true,
		ProtectReacts:  true,
	}

	// --with removes protections
	for _, flag := range withFlags {
		for _, item := range strings.Split(flag, ",") {
			switch strings.TrimSpace(strings.ToLower(item)) {
			case "replies":
				opts.ProtectReplies = false
			case "faves":
				opts.ProtectFaves = false
			case "reacts", "reactions":
				opts.ProtectReacts = false
			}
		}
	}

	// --without adds requirements (only prune items lacking these)
	for _, flag := range withoutFlags {
		for _, item := range strings.Split(flag, ",") {
			switch strings.TrimSpace(strings.ToLower(item)) {
			case "replies":
				opts.RequireReplies = true
			case "faves":
				opts.RequireFaves = true
			case "reacts", "reactions":
				opts.RequireReacts = true
			}
		}
	}

	return opts
}
