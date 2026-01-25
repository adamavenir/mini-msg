package command

import (
	"encoding/json"
	"io"
)

func writeInitError(errOut io.Writer, jsonMode bool, err error) error {
	if !jsonMode {
		return err
	}

	result := initResult{
		Initialized: false,
		Error:       err.Error(),
	}
	_ = json.NewEncoder(errOut).Encode(result)
	return nil
}
