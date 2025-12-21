package command

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/spf13/cobra"
)

// CommandContext provides shared command resources.
type CommandContext struct {
	DB            *sql.DB
	Project       core.Project
	JSONMode      bool
	ChannelID     string
	ChannelName   string
	ProjectConfig *db.ProjectConfig
}

// GetContext resolves database and channel context for a command.
func GetContext(cmd *cobra.Command) (*CommandContext, error) {
	projectAlias, _ := cmd.Flags().GetString("project")
	jsonMode, _ := cmd.Flags().GetBool("json")
	channelRef, _ := cmd.Flags().GetString("in")

	if projectAlias != "" {
		mainProject, err := core.DiscoverProject("")
		if err != nil {
			return nil, err
		}
		mainDB, err := db.OpenDatabase(mainProject)
		if err != nil {
			return nil, err
		}
		if err := db.InitSchema(mainDB); err != nil {
			_ = mainDB.Close()
			return nil, err
		}

		linked, err := db.GetLinkedProject(mainDB, projectAlias)
		_ = mainDB.Close()
		if err != nil {
			return nil, err
		}
		if linked == nil {
			return nil, fmt.Errorf("linked project '%s' not found. Use 'mm link' first", projectAlias)
		}
		if _, err := os.Stat(linked.Path); err != nil {
			return nil, fmt.Errorf("linked project '%s' database not found at %s", projectAlias, linked.Path)
		}

		project, err := projectFromDBPath(linked.Path)
		if err != nil {
			return nil, err
		}
		linkedDB, err := db.OpenDatabase(project)
		if err != nil {
			return nil, err
		}
		if err := db.InitSchema(linkedDB); err != nil {
			_ = linkedDB.Close()
			return nil, err
		}

		config, err := db.ReadProjectConfig(project.DBPath)
		if err != nil {
			_ = linkedDB.Close()
			return nil, err
		}

		return &CommandContext{
			DB:            linkedDB,
			Project:       project,
			JSONMode:      jsonMode,
			ProjectConfig: config,
		}, nil
	}

	ctx, err := ResolveChannelContext(channelRef, "")
	if err != nil {
		return nil, err
	}
	conn, err := db.OpenDatabase(ctx.Project)
	if err != nil {
		return nil, err
	}
	if err := db.InitSchema(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &CommandContext{
		DB:            conn,
		Project:       ctx.Project,
		JSONMode:      jsonMode,
		ChannelID:     ctx.ChannelID,
		ChannelName:   ctx.ChannelName,
		ProjectConfig: ctx.ProjectConfig,
	}, nil
}

// ChannelContext describes resolved channel references.
type ChannelContext struct {
	Project       core.Project
	ChannelID     string
	ChannelName   string
	ProjectConfig *db.ProjectConfig
}

// ResolveChannelContext resolves channel context using global config and local project.
func ResolveChannelContext(channelRef string, cwd string) (*ChannelContext, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	globalConfig, err := core.ReadGlobalConfig()
	if err != nil {
		return nil, err
	}

	if channelRef != "" {
		if id, channel, ok := core.FindChannelByRef(channelRef, globalConfig); ok {
			project, err := projectFromRoot(channel.Path)
			if err != nil {
				return nil, err
			}
			config, err := db.ReadProjectConfig(project.DBPath)
			if err != nil {
				return nil, err
			}
			return &ChannelContext{Project: project, ChannelID: id, ChannelName: channel.Name, ProjectConfig: config}, nil
		}

		if localProject, err := core.DiscoverProject(cwd); err == nil {
			localConfig, err := db.ReadProjectConfig(localProject.DBPath)
			if err != nil {
				return nil, err
			}
			if localConfig != nil && (localConfig.ChannelID == channelRef || localConfig.ChannelName == channelRef) {
				return &ChannelContext{
					Project:       localProject,
					ChannelID:     localConfig.ChannelID,
					ChannelName:   localConfig.ChannelName,
					ProjectConfig: localConfig,
				}, nil
			}
		}

		return nil, fmt.Errorf("channel not found: %s", channelRef)
	}

	localProject, err := core.DiscoverProject(cwd)
	if err != nil {
		return nil, err
	}
	localConfig, err := db.ReadProjectConfig(localProject.DBPath)
	if err != nil {
		return nil, err
	}
	if localConfig == nil || localConfig.ChannelID == "" {
		return nil, errors.New("no channel context: local .mm/ directory has no channel_id")
	}

	return &ChannelContext{
		Project:       localProject,
		ChannelID:     localConfig.ChannelID,
		ChannelName:   localConfig.ChannelName,
		ProjectConfig: localConfig,
	}, nil
}

// ResolveAgentRef resolves agent IDs using known agent aliases.
func ResolveAgentRef(ref string, config *db.ProjectConfig) string {
	normalized := core.NormalizeAgentRef(ref)
	if config == nil || len(config.KnownAgents) == 0 {
		return normalized
	}

	for _, entry := range config.KnownAgents {
		if entry.Name != nil && *entry.Name == normalized {
			return normalized
		}
		if entry.GlobalName != nil && *entry.GlobalName == normalized {
			return derefName(entry.Name, normalized)
		}
		if len(entry.Nicks) > 0 {
			for _, nick := range entry.Nicks {
				if nick == normalized {
					return derefName(entry.Name, normalized)
				}
			}
		}
	}

	return normalized
}

func projectFromRoot(rootPath string) (core.Project, error) {
	dbPath := filepath.Join(rootPath, ".mm", "mm.db")
	if _, err := os.Stat(dbPath); err != nil {
		return core.Project{}, fmt.Errorf("channel database not found at %s", dbPath)
	}
	return core.Project{Root: rootPath, DBPath: dbPath}, nil
}

func projectFromDBPath(dbPath string) (core.Project, error) {
	mmDir := filepath.Dir(dbPath)
	root := filepath.Dir(mmDir)
	if _, err := os.Stat(dbPath); err != nil {
		return core.Project{}, err
	}
	return core.Project{Root: root, DBPath: dbPath}, nil
}

func derefName(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}
