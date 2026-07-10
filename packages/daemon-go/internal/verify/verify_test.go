package verify

import (
	"strings"
	"testing"
)

func TestRunWithAllPass(t *testing.T) {
	res := RunWith([]string{"a", "b"}, func(cmd string) (string, bool) {
		return "ok:" + cmd, true
	})
	if !res.AllPassed {
		t.Fatal("all commands passed → AllPassed should be true")
	}
	if len(res.Checks) != 2 {
		t.Fatalf("want 2 checks, got %d", len(res.Checks))
	}
	if len(res.Failed()) != 0 {
		t.Errorf("no failures expected, got %v", res.Failed())
	}
}

func TestRunWithOneFails(t *testing.T) {
	res := RunWith([]string{"go build", "go test", "lint"}, func(cmd string) (string, bool) {
		return "", cmd != "go test" // go test fails
	})
	if res.AllPassed {
		t.Fatal("a failing command must make AllPassed false")
	}
	failed := res.Failed()
	if len(failed) != 1 || failed[0] != "go test" {
		t.Fatalf("Failed() = %v, want [go test]", failed)
	}
}

func TestRunWithEmptyIsPass(t *testing.T) {
	res := RunWith(nil, func(string) (string, bool) { return "", false })
	if !res.AllPassed || len(res.Checks) != 0 {
		t.Fatal("no checks → trivially passed, no check rows")
	}
}

func TestRunWithSkipsBlankCommands(t *testing.T) {
	calls := 0
	res := RunWith([]string{"", "  ", "real"}, func(string) (string, bool) { calls++; return "", true })
	if calls != 1 {
		t.Fatalf("blank commands should be skipped; executor called %d times", calls)
	}
	if len(res.Checks) != 1 {
		t.Fatalf("want 1 check, got %d", len(res.Checks))
	}
}

func TestOutputTruncated(t *testing.T) {
	big := strings.Repeat("x", 5000)
	res := RunWith([]string{"c"}, func(string) (string, bool) { return big, true })
	if len(res.Checks[0].Output) > 2100 {
		t.Errorf("output should be truncated, got %d chars", len(res.Checks[0].Output))
	}
}
