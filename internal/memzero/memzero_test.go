package memzero

import "testing"

func TestClear(t *testing.T) {
	b := []byte("secret")
	Clear(b)
	for i, v := range b {
		if v != 0 {
			t.Fatalf("byte %d not zeroed: %v", i, v)
		}
	}
	Clear(nil) // must not panic
}
