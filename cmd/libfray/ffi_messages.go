package main

import "C"

import (
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

//export FrayGetMessages
func FrayGetMessages(handle C.ulonglong, home *C.char, limit C.int, sinceCursor *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	homeStr := cStringToGo(home)
	cursorStr := cStringToGo(sinceCursor)

	cursor, _ := parseCursor(cursorStr)

	var homePtr *string
	if home != nil {
		homePtr = &homeStr
	}

	opts := &types.MessageQueryOptions{
		Limit: int(limit),
		Since: cursor,
		Home:  homePtr,
	}

	messages, err := db.GetMessages(entry.db, opts)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
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

//export FrayPostMessage
func FrayPostMessage(handle C.ulonglong, body, fromAgent, home, replyTo *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	bodyStr := cStringToGo(body)
	agentStr := cStringToGo(fromAgent)
	homeStr := cStringToGo(home)
	replyToStr := cStringToGo(replyTo)

	// Resolve thread name to GUID if needed
	if homeStr != "" && homeStr != "room" {
		thread, err := db.GetThreadByNameAny(entry.db, homeStr)
		if err == nil && thread != nil {
			homeStr = thread.GUID
		}
	}

	// Determine message type: user for humans, agent for managed agents
	msgType := types.MessageTypeUser
	agent, _ := db.GetAgent(entry.db, agentStr)
	if agent != nil && agent.Managed {
		msgType = types.MessageTypeAgent
	}

	mentions := core.ExtractMentions(bodyStr, nil)

	msg := types.Message{
		Body:      bodyStr,
		FromAgent: agentStr,
		Home:      homeStr,
		Mentions:  mentions,
		Type:      msgType,
	}

	if replyToStr != "" {
		msg.ReplyTo = &replyToStr
	}

	created, err := db.CreateMessage(entry.db, msg)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	frayDir := filepath.Dir(entry.project.DBPath)
	if err := db.AppendMessage(frayDir, created); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	now := time.Now().Unix()
	_ = db.UpdateAgent(entry.db, agentStr, db.AgentUpdates{
		LastSeen: types.OptionalInt64{Set: true, Value: &now},
	})

	return returnJSON(successResponse(created))
}

//export FrayEditMessage
func FrayEditMessage(handle C.ulonglong, msgID, newBody, reason *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	msgIDStr := cStringToGo(msgID)
	newBodyStr := cStringToGo(newBody)
	reasonStr := cStringToGo(reason)

	msg, err := db.GetMessage(entry.db, msgIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if msg == nil {
		return returnJSON(errorResponse("message not found"))
	}

	if err := db.EditMessage(entry.db, msgIDStr, newBodyStr, msg.FromAgent); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	frayDir := filepath.Dir(entry.project.DBPath)
	var reasonPtr *string
	if reasonStr != "" {
		reasonPtr = &reasonStr
	}
	_ = db.AppendMessageUpdate(frayDir, db.MessageUpdateJSONLRecord{
		ID:     msgIDStr,
		Body:   &newBodyStr,
		Reason: reasonPtr,
	})

	updatedMsg, err := db.GetMessage(entry.db, msgIDStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(updatedMsg))
}

//export FrayAddReaction
func FrayAddReaction(handle C.ulonglong, msgID, emoji, agent *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	msgIDStr := cStringToGo(msgID)
	emojiStr := cStringToGo(emoji)
	agentStr := cStringToGo(agent)

	msg, reactedAt, err := db.AddReaction(entry.db, msgIDStr, agentStr, emojiStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	frayDir := filepath.Dir(entry.project.DBPath)
	_ = db.AppendReaction(frayDir, msgIDStr, agentStr, emojiStr, reactedAt)

	return returnJSON(successResponse(msg))
}

//export FrayGetAgents
