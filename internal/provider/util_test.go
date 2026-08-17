package provider

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDescribeExecErrPrefersTrimmedStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf '  useful failure  \\n' >&2; exit 7")
	_, err := cmd.Output()
	got := describeExecErr(err)
	if got == nil || got.Error() != "useful failure" {
		t.Fatalf("describeExecErr() = %v, want trimmed stderr", got)
	}
}

func TestDescribeExecErrFallsBackToOriginalError(t *testing.T) {
	want := errors.New("ordinary failure")
	if got := describeExecErr(want); got != want {
		t.Fatalf("describeExecErr() returned %v, want original error", got)
	}
}

func TestDescribeExecErrFallsBackWhenStderrIsBlank(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf ' \\n\\t' >&2; exit 1")
	_, err := cmd.Output()
	got := describeExecErr(err)
	if got == nil || !strings.Contains(got.Error(), "exit status 1") {
		t.Fatalf("describeExecErr() = %v, want original exit error", got)
	}
}
