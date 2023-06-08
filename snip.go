package snip

import (
	"github.com/google/uuid"
	"time"
)

// Snip represents a snippet of data with additional metadata
type Snip struct {
	Data      []byte
	Timestamp time.Time
	UUID      uuid.UUID
}

// New returns a new snippet and generates a new UUID for it
func New() (Snip, error) {
	return Snip{
		Data:      []byte{},
		Timestamp: time.Now(),
		UUID:      uuid.New(),
	}, nil
}
