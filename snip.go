package snip

import (
	"github.com/bvinc/go-sqlite-lite/sqlite3"
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

// CreateNewDatabase creates a new sqlite3 database
func CreateNewDatabase(path string) error {
	conn, err := sqlite3.Open(path)
	if err != nil {
		return err
	}
	defer conn.Close()

	// build schema
	err = conn.Exec(`CREATE TABLE snip(data TEXT, timestamp TEXT, uuid TEXT)`)
	if err != nil {
		return err
	}

	return nil
}

// InsertSnip adds a new Snip to the database
func InsertSnip(path string, s Snip) error {
	conn, err := sqlite3.Open(path)
	if err != nil {
		return err
	}
	defer conn.Close()

	stmt, err := conn.Prepare(`INSERT INTO snip VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	err = stmt.Exec(string(s.Data), s.Timestamp.String(), s.UUID.String())
	if err != nil {
		return err
	}
	return nil
}
