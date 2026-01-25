package main

import "C"

import (
	"strconv"
	"strings"

	"github.com/adamavenir/fray/internal/types"
)

// Helper functions
func goStringToC(s string) *C.char {
	return C.CString(s)
}

func cStringToGo(cs *C.char) string {
	if cs == nil {
		return ""
	}
	return C.GoString(cs)
}

func returnJSON(data []byte) *C.char {
	return goStringToC(string(data))
}

func parseCursor(cursorStr string) (*types.MessageCursor, error) {
	if cursorStr == "" {
		return nil, nil
	}
	parts := strings.SplitN(cursorStr, ":", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, nil
	}
	return &types.MessageCursor{GUID: parts[0], TS: ts}, nil
}

func messagesToInterface(messages []types.Message) []interface{} {
	result := make([]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = msg
	}
	return result
}
