package main

import "C"

import (
	"strings"

	"github.com/adamavenir/fray/internal/db"
)

func FrayFaveItem(handle C.ulonglong, itemGUID, agentID *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	itemGUIDStr := cStringToGo(itemGUID)
	agentIDStr := cStringToGo(agentID)

	itemType := "message"
	if strings.HasPrefix(itemGUIDStr, "thrd-") {
		itemType = "thread"
	}

	favedAt, err := db.AddFave(entry.db, agentIDStr, itemType, itemGUIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(map[string]interface{}{
		"faved":    true,
		"faved_at": favedAt,
	}))
}

func FrayGetFaves(handle C.ulonglong, agentID *C.char, itemType *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	agentIDStr := cStringToGo(agentID)
	typeStr := cStringToGo(itemType)

	faves, err := db.GetFaves(entry.db, agentIDStr, typeStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(faves))
}

//export FrayUnfaveItem
func FrayUnfaveItem(handle C.ulonglong, itemGUID, agentID *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	itemGUIDStr := cStringToGo(itemGUID)
	agentIDStr := cStringToGo(agentID)

	// Infer item type from GUID prefix
	itemType := "message"
	if strings.HasPrefix(itemGUIDStr, "thrd-") {
		itemType = "thread"
	}

	if err := db.RemoveFave(entry.db, agentIDStr, itemType, itemGUIDStr); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(map[string]bool{"unfaved": true}))
}

//export FrayGetAgentUsage
