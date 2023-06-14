package snip

import (
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/ryanfrishkorn/snip/database"
	"strconv"
	"time"
)

// Attachment represents data (binary safe) associated with a specific snip
type Attachment struct {
	UUID      uuid.UUID
	Data      []byte
	Size      int
	SnipUUID  uuid.UUID
	Timestamp time.Time
	Title     string
}

// GetAttachmentMetadata returns all fields except Data for analysis without large memory use
func GetAttachmentMetadata(searchUUID uuid.UUID) (Attachment, error) {
	a := Attachment{}

	var stmt *sqlite3.Stmt
	stmt, err := database.Conn.Prepare(`SELECT size, snip_uuid, timestamp, title FROM snip_attachment WHERE uuid = ?`, searchUUID.String())
	if err != nil {
		return a, err
	}
	defer stmt.Close()

	err = stmt.Exec()
	if err != nil {
		return a, err
	}

	resultCount := 0
	for {
		hasRow, err := stmt.Step()
		if !hasRow {
			break
		}
		resultCount++
		// enforce only one result to avoid ambiguous behavior
		if resultCount > 1 {
			return a, fmt.Errorf("database search returned multiple results")
		}

		var (
			size      string
			snipUUID  string
			timestamp string
			title     string
		)
		err = stmt.Scan(&size, &snipUUID, &timestamp, &title)
		if err != nil {
			return a, err
		}
		a.UUID = searchUUID
		if err != nil {
			return a, fmt.Errorf("error parsing uuid string into struct")
		}
		a.Size, err = strconv.Atoi(size)
		a.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		a.Title = title
	}
	if resultCount == 0 {
		return a, fmt.Errorf("database search returned zero results")
	}
	return a, nil
}

func GetAttachmentFromUUID(searchUUID uuid.UUID) (Attachment, error) {
	a := Attachment{}

	var stmt *sqlite3.Stmt
	stmt, err := database.Conn.Prepare(`SELECT data, size, snip_uuid, timestamp, title FROM snip_attachment WHERE uuid = ?`, searchUUID.String())
	if err != nil {
		return a, err
	}
	defer stmt.Close()

	if err != nil {
		return a, err
	}

	resultCount := 0
	for {
		hasRow, err := stmt.Step()
		if !hasRow {
			break
		}
		resultCount++
		// enforce only one result to avoid ambiguous behavior
		if resultCount > 1 {
			return a, fmt.Errorf("database search returned multiple results")
		}

		var (
			data      string
			size      string
			snipUUID  string
			timestamp string
			title     string
		)
		err = stmt.Scan(&data, &size, &snipUUID, &timestamp, &title)
		if err != nil {
			return a, err
		}
		a.UUID = searchUUID
		if err != nil {
			return a, fmt.Errorf("error parsing uuid string into struct")
		}
		a.Data = []byte(data)
		a.Size, err = strconv.Atoi(size)
		a.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		a.Title = title
	}
	if resultCount == 0 {
		return a, fmt.Errorf("database search returned zero results")
	}
	return a, nil
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
