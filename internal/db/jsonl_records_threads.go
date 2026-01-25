package db

// ThreadJSONLRecord represents a thread entry in JSONL.
type ThreadJSONLRecord struct {
	Type              string   `json:"type"`
	GUID              string   `json:"guid"`
	Name              string   `json:"name"`
	ParentThread      *string  `json:"parent_thread,omitempty"`
	Subscribed        []string `json:"subscribed,omitempty"`
	Status            string   `json:"status"`
	ThreadType        string   `json:"thread_type,omitempty"`
	CreatedAt         int64    `json:"created_at"`
	AnchorMessageGUID *string  `json:"anchor_message_guid,omitempty"`
	AnchorHidden      bool     `json:"anchor_hidden,omitempty"`
	LastActivityAt    *int64   `json:"last_activity_at,omitempty"`
}

// ThreadUpdateJSONLRecord represents a thread update entry in JSONL.
type ThreadUpdateJSONLRecord struct {
	Type              string  `json:"type"`
	GUID              string  `json:"guid"`
	Name              *string `json:"name,omitempty"`
	Status            *string `json:"status,omitempty"`
	ThreadType        *string `json:"thread_type,omitempty"`
	ParentThread      *string `json:"parent_thread,omitempty"`
	AnchorMessageGUID *string `json:"anchor_message_guid,omitempty"`
	AnchorHidden      *bool   `json:"anchor_hidden,omitempty"`
	LastActivityAt    *int64  `json:"last_activity_at,omitempty"`
}

// ThreadDeleteJSONLRecord represents a thread deletion tombstone.
type ThreadDeleteJSONLRecord struct {
	Type     string `json:"type"` // "thread_delete"
	ThreadID string `json:"thread_id"`
	Seq      int64  `json:"seq,omitempty"`
	TS       int64  `json:"ts"`
}

// ThreadSubscribeJSONLRecord represents a subscription event.
type ThreadSubscribeJSONLRecord struct {
	Type         string `json:"type"`
	ThreadGUID   string `json:"thread_guid"`
	AgentID      string `json:"agent_id"`
	SubscribedAt int64  `json:"subscribed_at"`
}

// ThreadUnsubscribeJSONLRecord represents an unsubscribe event.
type ThreadUnsubscribeJSONLRecord struct {
	Type           string `json:"type"`
	ThreadGUID     string `json:"thread_guid"`
	AgentID        string `json:"agent_id"`
	UnsubscribedAt int64  `json:"unsubscribed_at"`
}

// ThreadMessageJSONLRecord represents a thread membership event.
type ThreadMessageJSONLRecord struct {
	Type        string `json:"type"`
	ThreadGUID  string `json:"thread_guid"`
	MessageGUID string `json:"message_guid"`
	AddedBy     string `json:"added_by"`
	AddedAt     int64  `json:"added_at"`
}

// ThreadMessageRemoveJSONLRecord represents a removal event.
type ThreadMessageRemoveJSONLRecord struct {
	Type        string `json:"type"`
	ThreadGUID  string `json:"thread_guid"`
	MessageGUID string `json:"message_guid"`
	RemovedBy   string `json:"removed_by"`
	RemovedAt   int64  `json:"removed_at"`
}

// MessagePinJSONLRecord represents a message pin event.
type MessagePinJSONLRecord struct {
	Type        string `json:"type"`
	MessageGUID string `json:"message_guid"`
	ThreadGUID  string `json:"thread_guid"`
	PinnedBy    string `json:"pinned_by"`
	PinnedAt    int64  `json:"pinned_at"`
}

// MessageUnpinJSONLRecord represents a message unpin event.
type MessageUnpinJSONLRecord struct {
	Type        string `json:"type"`
	MessageGUID string `json:"message_guid"`
	ThreadGUID  string `json:"thread_guid"`
	UnpinnedBy  string `json:"unpinned_by"`
	UnpinnedAt  int64  `json:"unpinned_at"`
}

// MessageMoveJSONLRecord represents a message move event.
type MessageMoveJSONLRecord struct {
	Type        string `json:"type"`
	MessageGUID string `json:"message_guid"`
	OldHome     string `json:"old_home"`
	NewHome     string `json:"new_home"`
	MovedBy     string `json:"moved_by"`
	MovedAt     int64  `json:"moved_at"`
}

// ThreadPinJSONLRecord represents a thread pin event.
type ThreadPinJSONLRecord struct {
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	PinnedBy   string `json:"pinned_by"`
	PinnedAt   int64  `json:"pinned_at"`
}

// ThreadUnpinJSONLRecord represents a thread unpin event.
type ThreadUnpinJSONLRecord struct {
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	UnpinnedBy string `json:"unpinned_by"`
	UnpinnedAt int64  `json:"unpinned_at"`
}

// ThreadMuteJSONLRecord represents a thread mute event.
type ThreadMuteJSONLRecord struct {
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	AgentID    string `json:"agent_id"`
	MutedAt    int64  `json:"muted_at"`
	ExpiresAt  *int64 `json:"expires_at,omitempty"`
}

// ThreadUnmuteJSONLRecord represents a thread unmute event.
type ThreadUnmuteJSONLRecord struct {
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	AgentID    string `json:"agent_id"`
	UnmutedAt  int64  `json:"unmuted_at"`
}
