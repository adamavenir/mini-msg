package chat

import (
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// getPendingPermissionGUIDs returns a map of permission GUIDs that are truly pending
// (not yet approved/denied in permissions.jsonl).
func (m *Model) getPendingPermissionGUIDs() map[string]bool {
	pending := make(map[string]bool)
	perms, err := db.ReadPermissions(m.projectRoot)
	if err != nil {
		return pending
	}
	for _, perm := range perms {
		if perm.Status == types.PermissionStatusPending || perm.Status == "" {
			pending[perm.GUID] = true
		}
	}
	return pending
}

// pinnedPermissionsHeight returns the number of lines used by pinned permission requests.
// Currently disabled - returns 0 to avoid layout complexity.
// Permission requests are shown inline in the viewport instead.
func (m *Model) pinnedPermissionsHeight() int {
	// DISABLED: Pinned permissions cause layout complexity.
	// Permission requests are still shown inline in the viewport.
	return 0
}
