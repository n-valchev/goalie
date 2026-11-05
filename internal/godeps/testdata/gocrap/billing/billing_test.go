package billing

import "testing"

func TestEveryBranch(t *testing.T) {
	if !(Invoice{}).Paid() {
		t.Error("Paid")
	}
	if Sign(-1) != -1 || Sign(1) != 1 {
		t.Error("Sign")
	}
	if Band(-1) != "low" || Band(10) != "high" || Band(5) != "mid" {
		t.Error("Band")
	}
	if Size(-1) != "none" || Size(5) != "small" || Size(50) != "medium" || Size(500) != "large" {
		t.Error("Size")
	}
}
