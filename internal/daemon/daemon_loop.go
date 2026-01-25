package daemon

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// watchLoop is the main daemon loop.
func (d *Daemon) watchLoop(ctx context.Context) {
	defer d.wg.Done()

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

// poll checks for new mentions and updates process states.
func (d *Daemon) poll(ctx context.Context) {
	// Get managed agents
	agents, err := d.getManagedAgents()
	if err != nil {
		d.debugf("poll: error getting managed agents: %v", err)
		// Check for schema errors
		if isSchemaError(err) {
			fmt.Fprintf(os.Stderr, "Error: database schema mismatch. Run 'fray rebuild' to fix.\n")
			fmt.Fprintf(os.Stderr, "Details: %v\n", err)
			// Signal stop - can't continue with schema errors
			close(d.stopCh)
		}
		return
	}

	if len(agents) == 0 {
		d.debugf("poll: no managed agents found")
		return
	}

	d.debugf("poll: checking %d managed agents", len(agents))

	// Check for new mentions and reactions for each managed agent
	for _, agent := range agents {
		d.checkMentions(ctx, agent)
		d.checkReactions(ctx, agent)
	}

	// Check wake conditions (pattern, timer, on-mention)
	d.checkWakeConditions(ctx, agents)

	// Update presence for running processes
	d.updatePresence()
}

// getManagedAgents returns all agents with managed=true.
func (d *Daemon) getManagedAgents() ([]types.Agent, error) {
	allAgents, err := db.GetAllAgents(d.database)
	if err != nil {
		return nil, err
	}

	var managed []types.Agent
	for _, agent := range allAgents {
		if agent.Managed {
			managed = append(managed, agent)
		}
	}
	return managed, nil
}
