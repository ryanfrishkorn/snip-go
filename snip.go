package snip

import (
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"regexp"
	"strings"
	"time"
)

// Snip represents a snippet of data with additional metadata
type Snip struct {
	Data      []byte
	Timestamp time.Time
	UUID      uuid.UUID
}

// CountWords returns an integer estimating the number of words in data
func (s *Snip) CountWords() int {
	data := FlattenString(string(s.Data))
	return len(strings.Split(data, " "))
}

// GenerateTitle returns a clean string derived from processing the data field
func (s *Snip) GenerateTitle(wordCount int) string {
	data := FlattenString(string(s.Data))
	pattern := regexp.MustCompile(`\w+`)
	title := pattern.FindAllString(data, wordCount)
	return strings.Join(title, " ")
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

// FlattenString returns a string with all newline, tabs, and spaces squeezed
func FlattenString(input string) string {
	// remove newlines and tabs
	dataSummary := strings.ReplaceAll(input, "\n", " ")
	dataSummary = strings.ReplaceAll(dataSummary, "\t", " ")
	// squeeze whitespace
	pattern := regexp.MustCompile(` +`)
	dataSummary = pattern.ReplaceAllString(dataSummary, " ")

	return dataSummary
}

// GetFromUUID retrieves a single Snip by its unique identifier
func GetFromUUID(path string, searchUUID uuid.UUID) (Snip, error) {
	s := Snip{}
	conn, err := sqlite3.Open(path)
	if err != nil {
		return s, err
	}
	defer conn.Close()

	stmt, err := conn.Prepare(`SELECT uuid, data, timestamp FROM snip WHERE uuid = ?`)
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
	var id string
	var timestamp string
	err = stmt.Scan(&id, &data, &timestamp)
	if err != nil {
		return s, err
	}
	s.Data = []byte(data)
	s.UUID, err = uuid.Parse(id)
	if err != nil {
		return s, fmt.Errorf("error parsing uuid string into struct")
	}
	s.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return s, err
	}
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

	err = stmt.Exec(string(s.Data), s.Timestamp.Format(time.RFC3339Nano), s.UUID.String())
	if err != nil {
		return err
	}
	return nil
}

// SearchDataTerm returns a slice of Snips whose data matches supplied terms
func SearchDataTerm(path string, term string) ([]Snip, error) {
	var searchResult []Snip
	if term == "" {
		return searchResult, fmt.Errorf("refusing to search for empty string")
	}
	conn, err := sqlite3.Open(path)
	if err != nil {
		return searchResult, err
	}
	defer conn.Close()

	// make term search fuzzy
	termFuzzy := "%" + term + "%"
	stmt, err := conn.Prepare(`SELECT uuid from snip where data LIKE ?`, termFuzzy)
	if err != nil {
		return searchResult, err
	}
	defer stmt.Close()

	for {
		hasRow, err := stmt.Step()
		if !hasRow {
			break
		}

		var idStr string
		err = stmt.Scan(&idStr)
		if err != nil {
			break
		}
		id, err := uuid.Parse(idStr)

		s, err := GetFromUUID(path, id)
		if err != nil {
			return searchResult, err
		}
		searchResult = append(searchResult, s)
	}

	return searchResult, nil
}
