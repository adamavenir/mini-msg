package db

import (
	_ "embed"
)

//go:embed templates/mentions.mld
var MentionsRouterTemplate []byte

//go:embed templates/status.mld
var StatusTemplate []byte

//go:embed templates/wake-router.mld
var WakeRouterTemplate []byte

//go:embed templates/wake-prompt.mld
var WakePromptTemplate []byte

//go:embed templates/stdout-repair.mld
var StdoutRepairTemplate []byte
