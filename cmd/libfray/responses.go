package main

import "encoding/json"

// JSON response types
type Response struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error *string     `json:"error,omitempty"`
}

type MessagePageResponse struct {
	Messages []interface{}   `json:"messages"`
	Cursor   *CursorResponse `json:"cursor,omitempty"`
}

type CursorResponse struct {
	GUID string `json:"guid"`
	TS   int64  `json:"ts"`
}

type ProjectResponse struct {
	Root   string `json:"root"`
	DBPath string `json:"db_path"`
}

func successResponse(data interface{}) []byte {
	resp := Response{OK: true, Data: data}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return errorResponse("failed to marshal response: " + err.Error())
	}
	return bytes
}

func errorResponse(errMsg string) []byte {
	resp := Response{OK: false, Error: &errMsg}
	bytes, _ := json.Marshal(resp)
	return bytes
}
