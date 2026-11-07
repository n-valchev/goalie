package text

import "testing"

func TestEveryBranch(t *testing.T) {
	if Plain("a") != "a" {
		t.Error("Plain")
	}
	if OrDash("") != "-" || OrDash("a") != "a" {
		t.Error("OrDash")
	}
	if Bounded("") != "-" || Bounded("0123456789") != "012345678" || Bounded("a") != "a" {
		t.Error("Bounded")
	}
}
