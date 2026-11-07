package goalie_test

import "testing"

const smallFile = "package m\n\nfunc Small(a int) int {\n\tif a > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"

func runCrap(t *testing.T, dir string, want int, args ...string) (stdout, stderr string) {
	t.Helper()
	code, stdout, stderr := runRule(t, dir, "crap", args...)
	if code != want {
		t.Errorf("exit code = %d, want %d; stdout = %q, stderr = %q", code, want, stdout, stderr)
	}
	return stdout, stderr
}

// runCrapWith first writes settings of the crap rule, such as `"coverProfiles": ["p.out"]`, to
// .goalierc.
func runCrapWith(t *testing.T, dir string, want int, rule string, args ...string) (stdout, stderr string) {
	t.Helper()
	write(t, dir, ".goalierc", `{"rules": {"crap": {`+rule+`}}}`)
	return runCrap(t, dir, want, args...)
}

func wantStdout(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
