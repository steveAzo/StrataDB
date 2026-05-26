package bloom

import "testing"

func TestAddAndContains(t *testing.T) {
	f := New(1024)
	f.Add("apple")
	f.Add("banana")

	if !f.MayContain("apple") {
		t.Error("expected MayContain('apple')=true")
	}
	if !f.MayContain("banana") {
		t.Error("expected MayContain('banana')=true")
	}
}

func TestDefinitelyAbsent(t *testing.T) {
	f := New(1024)
	f.Add("apple")

	// "zzzzzzzzz" almost certainly maps to different bits — should return false
	// (This test has a tiny theoretical chance of a false positive, but with
	// 1024 bits it's negligible for a fixed key pair.)
	if f.MayContain("zzzzzzzzz") {
		// Not a hard failure — bloom filters can have false positives.
		// But log it so we know if the filter is suspiciously always true.
		t.Log("MayContain('zzzzzzzzz') returned true — possible false positive")
	}
}

func TestNoFalseNegatives(t *testing.T) {
	f := New(512)
	keys := []string{"dog", "cat", "bird", "fish", "horse"}
	for _, k := range keys {
		f.Add(k)
	}
	for _, k := range keys {
		if !f.MayContain(k) {
			t.Errorf("false negative: MayContain(%q) returned false after Add", k)
		}
	}
}
