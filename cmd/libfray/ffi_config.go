package main

import "C"

import (
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

func FrayGetConfig(handle C.ulonglong, key *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	keyStr := cStringToGo(key)
	if keyStr == "" {
		return returnJSON(errorResponse("key required"))
	}

	value, err := db.GetConfig(entry.db, keyStr)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(value))
}

//export FraySetConfig
func FraySetConfig(handle C.ulonglong, key, value *C.char) *C.char {
	entry, ok := getHandle(uint64(handle))
	if !ok {
		return returnJSON(errorResponse("invalid database handle"))
	}

	keyStr := cStringToGo(key)
	valueStr := cStringToGo(value)
	if keyStr == "" {
		return returnJSON(errorResponse("key required"))
	}

	if err := db.SetConfig(entry.db, keyStr, valueStr); err != nil {
		return returnJSON(errorResponse(err.Error()))
	}

	return returnJSON(successResponse(map[string]bool{"set": true}))
}

//export FrayListChannels
func FrayListChannels() *C.char {
	config, err := core.ReadGlobalConfig()
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	if config == nil {
		return returnJSON(successResponse([]core.GlobalChannelRef{}))
	}

	// Convert map to list with IDs
	type ChannelInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}
	channels := make([]ChannelInfo, 0, len(config.Channels))
	for id, ch := range config.Channels {
		channels = append(channels, ChannelInfo{
			ID:   id,
			Name: ch.Name,
			Path: ch.Path,
		})
	}
	return returnJSON(successResponse(channels))
}
