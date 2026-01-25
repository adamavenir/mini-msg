package db

// topoSortThreads sorts threads so parents appear before children.
// This ensures FK constraints on parent_thread are satisfied during insert.
func topoSortThreads(threads []ThreadJSONLRecord) []ThreadJSONLRecord {
	if len(threads) == 0 {
		return threads
	}

	// Build a map for quick lookup
	byGUID := make(map[string]ThreadJSONLRecord, len(threads))
	for _, t := range threads {
		byGUID[t.GUID] = t
	}

	// Track visited and result
	visited := make(map[string]bool, len(threads))
	result := make([]ThreadJSONLRecord, 0, len(threads))

	// DFS helper - adds thread after all ancestors
	var visit func(guid string)
	visit = func(guid string) {
		if visited[guid] {
			return
		}
		visited[guid] = true

		thread, ok := byGUID[guid]
		if !ok {
			return
		}

		// Visit parent first (if exists)
		if thread.ParentThread != nil && *thread.ParentThread != "" {
			visit(*thread.ParentThread)
		}

		result = append(result, thread)
	}

	// Visit all threads
	for _, t := range threads {
		visit(t.GUID)
	}

	return result
}
