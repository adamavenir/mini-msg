package chat

import "github.com/adamavenir/fray/internal/types"

func (m *Model) calculateMaxDepth(guid string, children map[string][]types.Thread, currentDepth int) int {
	kids, hasKids := children[guid]
	if !hasKids || len(kids) == 0 {
		return currentDepth
	}
	maxDepth := currentDepth
	for _, child := range kids {
		childDepth := m.calculateMaxDepth(child.GUID, children, currentDepth+1)
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}
	return maxDepth
}
