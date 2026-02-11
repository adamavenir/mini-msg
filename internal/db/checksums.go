package db

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type fileChecksum struct {
	SHA256 string `json:"sha256"`
	Lines  int64  `json:"lines"`
	Mtime  int64  `json:"mtime"`
}

type checksumIndex map[string]map[string]fileChecksum

func updateChecksum(projectPath, dataPath string) error {
	machineID, fileName, ok := sharedPathParts(projectPath, dataPath)
	if !ok {
		return nil
	}
	checksumPath := filepath.Join(resolveFrayDir(projectPath), "shared", "checksums.json")
	if err := ensureDir(filepath.Dir(checksumPath)); err != nil {
		return err
	}

	file, err := os.OpenFile(checksumPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	checksums, err := readChecksums(file)
	if err != nil {
		return err
	}

	sum, lines, mtime, err := computeChecksum(dataPath)
	if err != nil {
		return err
	}

	if checksums[machineID] == nil {
		checksums[machineID] = map[string]fileChecksum{}
	}
	checksums[machineID][fileName] = fileChecksum{
		SHA256: sum,
		Lines:  lines,
		Mtime:  mtime,
	}

	return writeChecksums(file, checksums)
}

func validateChecksums(projectPath string) error {
	if !IsMultiMachineMode(projectPath) {
		return nil
	}
	checksumPath := filepath.Join(resolveFrayDir(projectPath), "shared", "checksums.json")
	if err := ensureDir(filepath.Dir(checksumPath)); err != nil {
		return err
	}

	file, err := os.OpenFile(checksumPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	checksums, err := readChecksums(file)
	if err != nil {
		return err
	}

	changed := false
	sharedFiles := []string{messagesFile, threadsFile, questionsFile, agentStateFile}
	for _, dir := range GetSharedMachinesDirs(projectPath) {
		machineID := filepath.Base(dir)
		for _, name := range sharedFiles {
			path := filepath.Join(dir, name)
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			dataMtime := info.ModTime().UnixMilli()
			entry := checksums[machineID][name]

			if entry.SHA256 == "" || entry.Mtime == 0 {
				sum, lines, mtime, err := computeChecksum(path)
				if err != nil {
					return err
				}
				if checksums[machineID] == nil {
					checksums[machineID] = map[string]fileChecksum{}
				}
				checksums[machineID][name] = fileChecksum{SHA256: sum, Lines: lines, Mtime: mtime}
				changed = true
				continue
			}

			if dataMtime > entry.Mtime {
				sum, lines, mtime, err := computeChecksum(path)
				if err != nil {
					return err
				}
				if checksums[machineID] == nil {
					checksums[machineID] = map[string]fileChecksum{}
				}
				checksums[machineID][name] = fileChecksum{SHA256: sum, Lines: lines, Mtime: mtime}
				changed = true
				continue
			}

			sum, lines, mtime, err := computeChecksum(path)
			if err != nil {
				return err
			}
			if sum != entry.SHA256 {
				log.Printf("warning: checksum mismatch for %s/%s", machineID, name)
				if checksums[machineID] == nil {
					checksums[machineID] = map[string]fileChecksum{}
				}
				checksums[machineID][name] = fileChecksum{SHA256: sum, Lines: lines, Mtime: mtime}
				changed = true
			}
		}
	}

	if changed {
		return writeChecksums(file, checksums)
	}
	return nil
}

func sharedPathParts(projectPath, dataPath string) (string, string, bool) {
	frayDir := resolveFrayDir(projectPath)
	sharedRoot := filepath.Join(frayDir, "shared", "machines")
	rel, err := filepath.Rel(sharedRoot, dataPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "", false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func computeChecksum(path string) (string, int64, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer file.Close()

	hash := sha256.New()
	buf := make([]byte, 32*1024)
	var lines int64
	for {
		n, err := file.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			_, _ = hash.Write(chunk)
			lines += int64(bytes.Count(chunk, []byte{'\n'}))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", 0, 0, err
		}
	}

	info, err := file.Stat()
	if err != nil {
		return "", 0, 0, err
	}

	return hex.EncodeToString(hash.Sum(nil)), lines, info.ModTime().UnixMilli(), nil
}

// ComputeChecksum exposes the checksum helper for sync tooling.
func ComputeChecksum(path string) (string, int64, int64, error) {
	return computeChecksum(path)
}

func readChecksums(file *os.File) (checksumIndex, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return checksumIndex{}, nil
	}
	var checksums checksumIndex
	if err := json.Unmarshal(data, &checksums); err != nil {
		log.Printf("warning: invalid checksums.json, rebuilding: %v", err)
		return checksumIndex{}, nil
	}
	return checksums, nil
}

func writeChecksums(file *os.File, checksums checksumIndex) error {
	data, err := json.MarshalIndent(checksums, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}
