package snip

import (
	"testing"
)

func TestNew(t *testing.T) {
	result, err := New()
	if err != nil {
		t.Errorf("error creating new snip struct: %v", err)
	}

	// default empty bytes
	if len(result.Data) != 0 {
		t.Errorf("result.Data expected zero length data, got %v", result.Data)
	}
}
