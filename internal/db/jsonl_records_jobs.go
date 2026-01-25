package db

import "github.com/adamavenir/fray/internal/types"

// JobCreateJSONLRecord represents a job creation event in JSONL.
type JobCreateJSONLRecord struct {
	Type       string            `json:"type"`
	GUID       string            `json:"guid"`
	Name       string            `json:"name"`
	Context    *types.JobContext `json:"context,omitempty"`
	OwnerAgent string            `json:"owner_agent"`
	Status     string            `json:"status"`
	ThreadGUID string            `json:"thread_guid"`
	CreatedAt  int64             `json:"created_at"`
}

// JobUpdateJSONLRecord represents a job status update event in JSONL.
type JobUpdateJSONLRecord struct {
	Type        string `json:"type"`
	GUID        string `json:"guid"`
	Status      string `json:"status"`
	CompletedAt *int64 `json:"completed_at,omitempty"`
}

// JobWorkerJoinJSONLRecord represents a worker joining a job.
type JobWorkerJoinJSONLRecord struct {
	Type     string `json:"type"`
	JobGUID  string `json:"job_guid"`
	AgentID  string `json:"agent_id"`
	WorkerID string `json:"worker_id"`
	JobIdx   int    `json:"job_idx"`
	JoinedAt int64  `json:"joined_at"`
}

// JobWorkerLeaveJSONLRecord represents a worker leaving a job.
type JobWorkerLeaveJSONLRecord struct {
	Type     string `json:"type"`
	JobGUID  string `json:"job_guid"`
	AgentID  string `json:"agent_id"`
	WorkerID string `json:"worker_id"`
	LeftAt   int64  `json:"left_at"`
}
