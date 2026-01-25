package db

import "github.com/adamavenir/fray/internal/types"

// AppendJobCreate appends a job creation record to JSONL.
func AppendJobCreate(projectPath string, job types.Job) error {
	record := JobCreateJSONLRecord{
		Type:       "job_create",
		GUID:       job.GUID,
		Name:       job.Name,
		Context:    job.Context,
		OwnerAgent: job.OwnerAgent,
		Status:     string(job.Status),
		ThreadGUID: job.ThreadGUID,
		CreatedAt:  job.CreatedAt,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendJobUpdate appends a job update record to JSONL.
func AppendJobUpdate(projectPath string, record JobUpdateJSONLRecord) error {
	record.Type = "job_update"
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendJobWorkerJoin appends a worker join record to JSONL.
func AppendJobWorkerJoin(projectPath string, record JobWorkerJoinJSONLRecord) error {
	record.Type = "job_worker_join"
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendJobWorkerLeave appends a worker leave record to JSONL.
func AppendJobWorkerLeave(projectPath string, record JobWorkerLeaveJSONLRecord) error {
	record.Type = "job_worker_leave"
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
