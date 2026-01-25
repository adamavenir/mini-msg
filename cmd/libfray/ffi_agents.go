package main

import "C"

import (
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/adamavenir/fray/internal/usage"
)

func FrayGetAgents(handle C.ulonglong, managedOnly C.int) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	var agents []types.Agent
	var err error

	if managedOnly != 0 {
		agents, err = db.GetManagedAgents(entry.db)
	} else {
		agents, err = db.GetAgents(entry.db)
	}
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(agents))
}

//export FrayGetAgent
func FrayGetAgent(handle C.ulonglong, agentID *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	agentIDStr := cStringToGo(agentID)
	agent, err := db.GetAgent(entry.db, agentIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if agent == nil {
		return returnJSON(errorResponse("agent not found"))
	}

	return returnJSON(successResponse(agent))
}

func FrayRegisterAgent(handle C.ulonglong, agentID *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	agentIDStr := cStringToGo(agentID)
	if agentIDStr == "" {
		return returnJSON(errorResponse("agent ID required"))
	}

	if !core.IsValidAgentID(agentIDStr) {
		return returnJSON(errorResponse("invalid agent ID: must start with lowercase letter and contain only lowercase letters, numbers, hyphens, and dots"))
	}

	existing, err := db.GetAgent(entry.db, agentIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if existing != nil {
		return returnJSON(successResponse(existing))
	}

	agentGUID, err := core.GenerateGUID("usr")
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	usedAvatars := make(map[string]struct{})
	existingAgents, _ := db.GetAgents(entry.db)
	for _, a := range existingAgents {
		if a.Avatar != nil && *a.Avatar != "" {
			usedAvatars[*a.Avatar] = struct{}{}
		}
	}
	avatar := core.AssignAvatar(agentIDStr, usedAvatars)

	now := time.Now().Unix()
	agent := types.Agent{
		GUID:         agentGUID,
		AgentID:      agentIDStr,
		Avatar:       &avatar,
		RegisteredAt: now,
		LastSeen:     now,
		Presence:     types.PresenceOffline,
	}

	if err := db.CreateAgent(entry.db, agent); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	frayDir := filepath.Dir(entry.project.DBPath)
	if err := db.AppendAgent(frayDir, agent); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	created, err := db.GetAgent(entry.db, agentIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(created))
}

//export FrayGetConfig

func FrayGetAgentUsage(handle C.ulonglong, agentID *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	agentIDStr := cStringToGo(agentID)
	if agentIDStr == "" {
		return returnJSON(errorResponse("agent_id required"))
	}

	// Get agent to find their session_id and driver
	agent, err := db.GetAgent(entry.db, agentIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if agent == nil {
		return returnJSON(errorResponse("agent not found"))
	}

	// If no session, return zeroes
	if agent.LastSessionID == nil || *agent.LastSessionID == "" {
		return returnJSON(successResponse(usage.SessionUsage{
			SessionID:    "",
			ContextLimit: 200000,
		}))
	}

	sessionID := *agent.LastSessionID

	// Determine driver from agent's invoke config
	driver := ""
	if agent.Invoke != nil {
		driver = agent.Invoke.Driver
	}

	// Use the internal usage package to get session usage
	var sessionUsage *usage.SessionUsage
	if driver != "" {
		sessionUsage, err = usage.GetSessionUsageByDriver(sessionID, driver)
	} else {
		sessionUsage, err = usage.GetSessionUsage(sessionID)
	}

	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	if sessionUsage == nil {
		sessionUsage = &usage.SessionUsage{
			SessionID:    sessionID,
			ContextLimit: 200000,
		}
	}

	return returnJSON(successResponse(sessionUsage))
}

//export FrayFreeString
