package tunnel

import "testing"

func TestLimiter(t *testing.T) {
	l := newLimiter(2)
	if !l.acquire() || !l.acquire() {
		t.Fatal("first two slots should be available")
	}
	if l.acquire() {
		t.Fatal("third slot should be refused")
	}
	l.release()
	if !l.acquire() {
		t.Fatal("a slot should be free after release")
	}
}
