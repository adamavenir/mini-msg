package command

import (
	"os"
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestJobCreateAndJoinFlow(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	// Create agent who will be job owner
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "owner", "starting workparty"); err != nil {
		t.Fatalf("new owner: %v", err)
	}

	// Create agent who will be worker
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "worker", "ready to work"); err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// Create a job
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "job", "create", "implement auth", "--as", "owner")
	if err != nil {
		t.Fatalf("job create: %v", err)
	}

	jobGUID := strings.TrimSpace(output)
	if !strings.HasPrefix(jobGUID, "job-") {
		t.Fatalf("expected job-xxx guid, got %q", jobGUID)
	}

	// Verify job was created in DB
	dbConn := openProjectDB(t, projectDir)
	job, err := db.GetJob(dbConn, jobGUID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job == nil {
		t.Fatal("expected job to exist")
	}
	if job.Name != "implement auth" {
		t.Fatalf("expected job name 'implement auth', got %q", job.Name)
	}
	if job.OwnerAgent != "owner" {
		t.Fatalf("expected owner_agent 'owner', got %q", job.OwnerAgent)
	}
	if job.Status != types.JobStatusRunning {
		t.Fatalf("expected status 'running', got %q", job.Status)
	}

	// Verify thread was created with job GUID as name
	thread, err := db.GetThreadByName(dbConn, jobGUID, nil)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread == nil {
		t.Fatal("expected thread to exist")
	}
	if thread.GUID != job.ThreadGUID {
		t.Fatalf("expected thread GUID %q, got %q", job.ThreadGUID, thread.GUID)
	}
	dbConn.Close()

	// Join the job as first worker (auto-index should be 0)
	cmd = NewRootCmd("test")
	output, err = executeCommand(cmd, "job", "join", jobGUID, "--as", "worker")
	if err != nil {
		t.Fatalf("job join: %v", err)
	}

	workerID := strings.TrimSpace(output)
	// Worker format: worker[<4-char-suffix>-<idx>]
	suffix := jobGUID[4:8]
	expectedWorkerID := "worker[" + suffix + "-0]"
	if workerID != expectedWorkerID {
		t.Fatalf("expected worker ID %q, got %q", expectedWorkerID, workerID)
	}

	// Verify worker agent was created
	dbConn = openProjectDB(t, projectDir)
	defer dbConn.Close()

	worker, err := db.GetAgent(dbConn, workerID)
	if err != nil {
		t.Fatalf("get worker agent: %v", err)
	}
	if worker == nil {
		t.Fatal("expected worker agent to exist")
	}
	if worker.JobID == nil || *worker.JobID != jobGUID {
		t.Fatalf("expected worker JobID %q, got %v", jobGUID, worker.JobID)
	}
	if worker.JobIdx == nil || *worker.JobIdx != 0 {
		t.Fatalf("expected worker JobIdx 0, got %v", worker.JobIdx)
	}
	if !worker.IsEphemeral {
		t.Fatal("expected worker to be ephemeral")
	}

	// Verify GetJobWorkers returns the worker
	workers, err := db.GetJobWorkers(dbConn, jobGUID)
	if err != nil {
		t.Fatalf("get job workers: %v", err)
	}
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
	if workers[0].AgentID != workerID {
		t.Fatalf("expected worker %q, got %q", workerID, workers[0].AgentID)
	}
}

func TestJobJoinAutoIndex(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "owner", "starting workparty"); err != nil {
		t.Fatalf("new owner: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "worker", "ready"); err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// Create job
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "job", "create", "test job", "--as", "owner")
	if err != nil {
		t.Fatalf("job create: %v", err)
	}
	jobGUID := strings.TrimSpace(output)
	suffix := jobGUID[4:8]

	// Join as worker three times - should get indices 0, 1, 2
	for i := 0; i < 3; i++ {
		cmd = NewRootCmd("test")
		output, err = executeCommand(cmd, "job", "join", jobGUID, "--as", "worker")
		if err != nil {
			t.Fatalf("job join %d: %v", i, err)
		}
		workerID := strings.TrimSpace(output)
		expectedWorkerID := "worker[" + suffix + "-" + string('0'+byte(i)) + "]"
		if workerID != expectedWorkerID {
			t.Fatalf("worker %d: expected %q, got %q", i, expectedWorkerID, workerID)
		}
	}

	// Verify all workers in DB
	dbConn := openProjectDB(t, projectDir)
	defer dbConn.Close()

	workers, err := db.GetJobWorkers(dbConn, jobGUID)
	if err != nil {
		t.Fatalf("get job workers: %v", err)
	}
	if len(workers) != 3 {
		t.Fatalf("expected 3 workers, got %d", len(workers))
	}
	for i, w := range workers {
		if w.JobIdx == nil || *w.JobIdx != i {
			t.Fatalf("worker %d: expected JobIdx %d, got %v", i, i, w.JobIdx)
		}
	}
}

func TestJobJoinExplicitIndex(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "owner", "hi"); err != nil {
		t.Fatalf("new owner: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "worker", "hi"); err != nil {
		t.Fatalf("new worker: %v", err)
	}

	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "job", "create", "test", "--as", "owner")
	if err != nil {
		t.Fatalf("job create: %v", err)
	}
	jobGUID := strings.TrimSpace(output)
	suffix := jobGUID[4:8]

	// Join with explicit index 5
	cmd = NewRootCmd("test")
	output, err = executeCommand(cmd, "job", "join", jobGUID, "--as", "worker", "--idx", "5")
	if err != nil {
		t.Fatalf("job join: %v", err)
	}
	workerID := strings.TrimSpace(output)
	expectedWorkerID := "worker[" + suffix + "-5]"
	if workerID != expectedWorkerID {
		t.Fatalf("expected %q, got %q", expectedWorkerID, workerID)
	}
}

func TestIsAmbiguousMention(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "owner", "hi"); err != nil {
		t.Fatalf("new owner: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "worker", "hi"); err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// Before job: @worker is not ambiguous
	dbConn := openProjectDB(t, projectDir)
	isAmbig, err := db.IsAmbiguousMention(dbConn, "worker")
	if err != nil {
		t.Fatalf("is ambiguous: %v", err)
	}
	if isAmbig {
		t.Fatal("expected @worker not to be ambiguous before job")
	}
	dbConn.Close()

	// Create job and join as worker
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "job", "create", "test", "--as", "owner")
	if err != nil {
		t.Fatalf("job create: %v", err)
	}
	jobGUID := strings.TrimSpace(output)

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "job", "join", jobGUID, "--as", "worker"); err != nil {
		t.Fatalf("job join: %v", err)
	}

	// After job with worker: @worker is ambiguous
	dbConn = openProjectDB(t, projectDir)
	defer dbConn.Close()
	isAmbig, err = db.IsAmbiguousMention(dbConn, "worker")
	if err != nil {
		t.Fatalf("is ambiguous: %v", err)
	}
	if !isAmbig {
		t.Fatal("expected @worker to be ambiguous when job has workers")
	}

	// @owner should not be ambiguous (no workers)
	isAmbig, err = db.IsAmbiguousMention(dbConn, "owner")
	if err != nil {
		t.Fatalf("is ambiguous owner: %v", err)
	}
	if isAmbig {
		t.Fatal("expected @owner not to be ambiguous (no workers)")
	}
}

func TestJobCreateWithContext(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "owner", "hi"); err != nil {
		t.Fatalf("new owner: %v", err)
	}

	// Create job with context
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "job", "create", "with context", "--as", "owner", "--context", `{"issues":["fray-abc","fray-xyz"]}`)
	if err != nil {
		t.Fatalf("job create: %v", err)
	}
	jobGUID := strings.TrimSpace(output)

	dbConn := openProjectDB(t, projectDir)
	defer dbConn.Close()

	job, err := db.GetJob(dbConn, jobGUID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.Context == nil {
		t.Fatal("expected job context")
	}
	if len(job.Context.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(job.Context.Issues))
	}
	if job.Context.Issues[0] != "fray-abc" || job.Context.Issues[1] != "fray-xyz" {
		t.Fatalf("unexpected issues: %v", job.Context.Issues)
	}
}

func TestJobJoinNonexistentJob(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "worker", "hi"); err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// Try to join non-existent job
	cmd = NewRootCmd("test")
	_, err = executeCommand(cmd, "job", "join", "job-notfound", "--as", "worker")
	if err == nil {
		t.Fatal("expected error joining non-existent job")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}
