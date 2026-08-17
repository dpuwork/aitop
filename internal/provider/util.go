package provider

import (
	"errors"
	"os/exec"
	"strings"
)

// describeExecErr extracts a readable message from a failed exec.Cmd,
// preferring captured stderr over Go's generic exit-status wording.
func describeExecErr(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return errors.New(stderr)
		}
	}
	return err
}
