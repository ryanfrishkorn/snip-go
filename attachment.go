package snip

import (
	"github.com/google/uuid"
	"time"
)

// Attachment represents data (binary safe) associated with a specific snip
type Attachment struct {
	UUID      uuid.UUID
	Data      []byte
	Size      int64
	SnipUUID  uuid.UUID
	Timestamp time.Time
	Title     string
}

// NewAttachment returns a new attachment struct with current defaults
func NewAttachment() Attachment {
	return Attachment{
		Data:      []byte{},
		Size:      0,
		Timestamp: time.Now(),
		Title:     "",
		UUID:      uuid.New(),
	}
}
