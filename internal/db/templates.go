package db

import (
	_ "embed"
)

//go:embed templates/router.mld
var RouterTemplate []byte

//go:embed templates/neo.mld
var NeoTemplate []byte
