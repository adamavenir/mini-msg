package main

import "C"

import "github.com/adamavenir/fray/internal/db"

func FrayGetReadTo(handle C.ulonglong, agentID, home *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	agentIDStr := cStringToGo(agentID)
	homeStr := cStringToGo(home)

	readTo, err := db.GetReadTo(entry.db, agentIDStr, homeStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(readTo))
}

//export FraySetReadTo
func FraySetReadTo(handle C.ulonglong, agentID, home, msgID *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	agentIDStr := cStringToGo(agentID)
	homeStr := cStringToGo(home)
	msgIDStr := cStringToGo(msgID)

	msg, err := db.GetMessage(entry.db, msgIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if msg == nil {
		return returnJSON(errorResponse("message not found"))
	}

	if err := db.SetReadTo(entry.db, agentIDStr, homeStr, msgIDStr, msg.TS); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(map[string]bool{"set": true}))
}

//export FrayRegisterAgent
