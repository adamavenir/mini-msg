package db

import (
	_ "embed"
)

//go:embed templates/router.mld
var RouterTemplate []byte
