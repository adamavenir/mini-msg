package main

import "C"

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func FrayGetThreads(handle C.ulonglong, parentThread *C.char, includeArchived C.int) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	var parentPtr *string
	if parentThread != nil {
		parentStr := cStringToGo(parentThread)
		if parentStr != "" {
			parentPtr = &parentStr
		}
	}

	opts := &types.ThreadQueryOptions{
		ParentThread:    parentPtr,
		IncludeArchived: includeArchived != 0,
	}

	threads, err := db.GetThreads(entry.db, opts)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(threads))
}

//export FrayGetThread
func FrayGetThread(handle C.ulonglong, threadRef *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	threadRefStr := cStringToGo(threadRef)
	if threadRefStr == "" {
		return returnJSON(errorResponse("thread reference required"))
	}

	// Try by GUID first
	if strings.HasPrefix(threadRefStr, "thrd-") {
		thread, err := db.GetThread(entry.db, threadRefStr)
		if err != nil {
			return returnJSON(errorResponse(err.Error()))
		}
		if thread != nil {
			return returnJSON(successResponse(thread))
		}
	}

	// Try by prefix
	thread, err := db.GetThreadByPrefix(entry.db, threadRefStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if thread != nil {
		return returnJSON(successResponse(thread))
	}

	// Try by name
	thread, err = db.GetThreadByNameAny(entry.db, threadRefStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if thread == nil {
		return returnJSON(errorResponse("thread not found"))
	}

	return returnJSON(successResponse(thread))
}

//export FrayGetThreadMessages
func FrayGetThreadMessages(handle C.ulonglong, threadGUID *C.char, limit C.int, sinceCursor *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	threadGUIDStr := cStringToGo(threadGUID)
	cursorStr := cStringToGo(sinceCursor)

	cursor, _ := parseCursor(cursorStr)

	messages, err := db.GetThreadMessages(entry.db, threadGUIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	// Apply cursor filtering in memory
	if cursor != nil {
		filtered := make([]types.Message, 0, len(messages))
		for _, msg := range messages {
			if msg.TS > cursor.TS || (msg.TS == cursor.TS && msg.ID > cursor.GUID) {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	// Apply limit
	if limit > 0 && int(limit) < len(messages) {
		messages = messages[:int(limit)]
	}

	var nextCursor *CursorResponse
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		nextCursor = &CursorResponse{GUID: last.ID, TS: last.TS}
	}

	return returnJSON(successResponse(MessagePageResponse{
		Messages: messagesToInterface(messages),
		Cursor:   nextCursor,
	}))
}

//export FraySubscribeToThread
func FraySubscribeToThread(handle C.ulonglong, threadGUID, agentID *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	threadGUIDStr := cStringToGo(threadGUID)
	agentIDStr := cStringToGo(agentID)

	if err := db.SubscribeThread(entry.db, threadGUIDStr, agentIDStr, 0); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	frayDir := filepath.Dir(entry.project.DBPath)
	_ = db.AppendThreadSubscribe(frayDir, db.ThreadSubscribeJSONLRecord{
		ThreadGUID:   threadGUIDStr,
		AgentID:      agentIDStr,
		SubscribedAt: time.Now().Unix(),
	})

	return returnJSON(successResponse(map[string]bool{"subscribed": true}))
}

//export FrayFaveItem
