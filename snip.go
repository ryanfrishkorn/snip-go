package snip

import (
	"fmt"
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
	err = conn.Exec(`CREATE TABLE IF NOT EXISTS snip(data TEXT, timestamp TEXT, uuid TEXT)`)
	if err != nil {
		return err
	}

	return nil
}

// GetFromUUID retrieves a single Snip by its unique identifier
func GetFromUUID(path string, searchUUID uuid.UUID) (Snip, error) {
	s := Snip{}
	conn, err := sqlite3.Open(path)
	if err != nil {
		return s, err
	}
	defer conn.Close()

	stmt, err := conn.Prepare(`SELECT data from snip WHERE uuid = ?`)
	if err != nil {
		return s, err
	}
	defer stmt.Close()

	err = stmt.Exec(searchUUID.String())
	if err != nil {
		return s, err
	}

	hasRow, err := stmt.Step()
	if !hasRow {
		return s, fmt.Errorf("database search returned zero results")
	}

	var data string
	err = stmt.Scan(&data)
	if err != nil {
		return s, err
	}
	s.Data = []byte(data)
	//FIXME convert this
	// s.Timestamp = timestamp
	s.UUID = searchUUID

	return s, nil
}

// InsertSnip adds a new Snip to the database
func InsertSnip(path string, s Snip) error {
	// do not insert without data
	if len(s.Data) == 0 {
		return fmt.Errorf("refusing to insert zero-length data")
	}
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