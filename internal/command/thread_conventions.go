package command

import (
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func ensureThread(ctx *CommandContext, name string, parent *types.Thread, subscribers []string) (*types.Thread, error) {
	var parentGUID *string
	if parent != nil {
		parentGUID = &parent.GUID
	}

	thread, err := db.GetThreadByName(ctx.DB, name, parentGUID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		created, err := db.CreateThread(ctx.DB, types.Thread{
			Name:         name,
			ParentThread: parentGUID,
			Status:       types.ThreadStatusOpen,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendThread(ctx.Project.DBPath, created, subscribers); err != nil {
			return nil, err
		}
		now := time.Now().Unix()
		for _, agentID := range subscribers {
			if agentID == "" {
				continue
			}
			if err := db.SubscribeThread(ctx.DB, created.GUID, agentID, now); err != nil {
				return nil, err
			}
		}
		return &created, nil
	}

	if len(subscribers) > 0 {
		now := time.Now().Unix()
		for _, agentID := range subscribers {
			if agentID == "" {
				continue
			}
			if err := db.SubscribeThread(ctx.DB, thread.GUID, agentID, now); err != nil {
				return nil, err
			}
			if err := db.AppendThreadSubscribe(ctx.Project.DBPath, db.ThreadSubscribeJSONLRecord{
				ThreadGUID:   thread.GUID,
				AgentID:      agentID,
				SubscribedAt: now,
			}); err != nil {
				return nil, err
			}
		}
	}

	return thread, nil
}
