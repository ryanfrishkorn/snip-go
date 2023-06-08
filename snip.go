package snip

import (
	"github.com/google/uuid"
)

type Snip struct {
	UUID uuid.UUID
	Data string
}

// New returns a new snippet and generates a new UUID for it
func New() (Snip, error) {
	return Snip{
		UUID: CreateUUID(),
		Data: "",
	}, nil
}

func CreateUUID() uuid.UUID {
	return uuid.New()
}
