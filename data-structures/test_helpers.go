package data_structures

import (
	"testing"
)

func expect[T comparable](t *testing.T, received, target T) {
	if target != received {
		t.Errorf("Expected: %v, Recieved: %v", target, received)
	}
}

func expectPanic(t *testing.T, fn func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("did not panic")
		}
	}()
	fn()
}
