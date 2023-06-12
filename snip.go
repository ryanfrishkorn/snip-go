package snip

import (
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"regexp"
	"strings"
	"time"
)

// Snip represents a snippet of data with additional metadata
type Snip struct {
	Attachments []Attachment
	Data        []byte
	Timestamp   time.Time
	Title       string
	UUID        uuid.UUID
}

// Attach adds files associated with a snip
func (s *Snip) Attach(path string, title string, data []byte) error {
	conn, err := sqlite3.Open(path)
	if err != nil {
		return err
	}

	// build and insert attachment
	a := NewAttachment()
	a.Data = data
	a.Title = title
	a.SnipUUID = s.UUID

	stmt, err := conn.Prepare(`INSERT INTO snip_attachment (uuid, snip_uuid, timestamp, title, data, size) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	err = stmt.Exec(a.UUID.String(), a.SnipUUID.String(), a.Timestamp.String(), a.Title, a.Data, len(a.Data))
	if err != nil {
		return err
	}
	return nil
}

// CountWords returns an integer estimating the number of words in data
func (s *Snip) CountWords() int {
	data := FlattenString(string(s.Data))
	return len(strings.Split(data, " "))
}

// GenerateTitle returns a clean string derived from processing the data field
func (s *Snip) GenerateTitle(wordCount int) string {
	data := FlattenString(string(s.Data))
	// FIXME by allowing additional sensible characters such as `:`
	pattern := regexp.MustCompile(`\w+`)
	title := pattern.FindAllString(data, wordCount)
	return strings.Join(title, " ")
}

// CreateNewDatabase creates a new sqlite3 database
func CreateNewDatabase(path string) error {
	conn, err := sqlite3.Open(path)
	if err != nil {
		return err
	}
	defer conn.Close()

	// build schema
	err = conn.Exec(`CREATE TABLE IF NOT EXISTS snip(uuid TEXT, timestamp TEXT, title TEXT, data TEXT)`)
	err = conn.Exec(`CREATE TABLE IF NOT EXISTS snip_attachment(uuid TEXT, snip_uuid TEXT, timestamp TEXT, title TEXT, data BLOB, size INTEGER)`)
	if err != nil {
		return err
	}

	return nil
}

// Delete removes a snip from the database
func Delete(path string, id uuid.UUID) error {
	conn, err := sqlite3.Open(path)
	if err != nil {
		return err
	}
	defer conn.Close()

	// remove
	stmt, err := conn.Prepare(`DELETE from snip WHERE uuid = ? LIMIT 1`, id.String())
	if err != nil {
		return err
	}
	err = stmt.Exec()
	if err != nil {
		return err
	}
	stmt.Close()
	return nil
}

// GetAttachments returns a slice of Attachment associated with the supplied snip uuid
func GetAttachments(path string, searchUUID uuid.UUID) ([]Attachment, error) {
	var attachments []Attachment

	ids, err := GetAttachmentsUUIDs(path, searchUUID)
	if err != nil {
		return attachments, err
	}
	log.Debug()

	for _, id := range ids {
		a, err := GetAttachmentFromUUID(path, id)
		if err != nil {
			return attachments, err
		}
		attachments = append(attachments, a)
	}
	return attachments, nil
}

// GetAttachmentsUUIDs returns a slice of attachment uuids associated with supplied snip uuid
func GetAttachmentsUUIDs(path string, snipUUID uuid.UUID) ([]uuid.UUID, error) {
	var results []uuid.UUID

	conn, err := sqlite3.Open(path)
	if err != nil {
		return results, err
	}
	defer conn.Close()

	stmt, err := conn.Prepare(`SELECT uuid FROM snip_attachment WHERE snip_uuid = ?`)
	if err != nil {
		return results, err
	}
	err = stmt.Exec(snipUUID.String())
	if err != nil {
		return results, err
	}

	resultCount := 0
	for {
		hasRow, err := stmt.Step()
		if !hasRow {
			break
		}
		resultCount++

		var idStr string
		err = stmt.Scan(&idStr)
		if err != nil {
			return results, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return results, err
		}
		results = append(results, id)
	}
	return results, nil
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
func GetFromUUID(path string, searchUUID string) (Snip, error) {
	s := Snip{}
	conn, err := sqlite3.Open(path)
	if err != nil {
		return s, err
	}
	defer conn.Close()

	// determine exact or partial matching
	var exactMatch bool
	var maxLength = 36
	length := len(searchUUID)

	switch {
	case length > maxLength || length == 0:
		return s, fmt.Errorf("supplied uuid string must be 1 to %d characters", maxLength)
	case length == maxLength:
		exactMatch = true
	default:
		exactMatch = false
	}

	var stmt *sqlite3.Stmt
	if exactMatch {
		stmt, err = conn.Prepare(`SELECT uuid, data, timestamp, title FROM snip WHERE uuid = ?`, searchUUID)
	} else {
		searchUUIDFuzzy := "%" + searchUUID + "%"
		stmt, err = conn.Prepare(`SELECT uuid, data, timestamp, title FROM snip WHERE uuid LIKE ?`, searchUUIDFuzzy)
	}
	if err != nil {
		return s, err
	}
	defer stmt.Close()

	if err != nil {
		return s, err
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
			return s, fmt.Errorf("database search returned multiple results")
		}

		var data string
		var id string
		var timestamp string
		var title string
		err = stmt.Scan(&id, &data, &timestamp, &title)
		if err != nil {
			return s, err
		}
		s.Data = []byte(data)
		s.UUID, err = uuid.Parse(id)
		if err != nil {
			return s, fmt.Errorf("error parsing uuid string into struct")
		}
		s.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		s.Title = title
		if err != nil {
			return s, err
		}
	}
	if resultCount == 0 {
		return s, fmt.Errorf("database search returned zero results")
	}

	// gather attachments
	s.Attachments, err = GetAttachments(path, s.UUID)
	if err != nil {
		return s, err
	}

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

	stmt, err := conn.Prepare(`INSERT INTO snip VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// reference
	err = stmt.Exec(s.UUID.String(), s.Timestamp.Format(time.RFC3339Nano), s.Title, string(s.Data))
	if err != nil {
		return err
	}
	return nil
}

// List returns a slice of all Snips in the database
func List(path string, limit int) ([]Snip, error) {
	var results []Snip
	var stmt *sqlite3.Stmt

	conn, err := sqlite3.Open(path)
	if err != nil {
		return results, err
	}
	defer conn.Close()

	if limit != 0 {
		stmt, err = conn.Prepare(`SELECT uuid, timestamp, title, data from snip LIMIT ?`, limit)
		if err != nil {
			return results, err
		}
	} else {
		stmt, err = conn.Prepare(`SELECT uuid, timestamp, title, data from snip`)
	}
	if err != nil {
		return results, err
	}
	defer stmt.Close()

	for {
		hasRow, err := stmt.Step()
		if !hasRow {
			break
		}

		var idStr string
		var timestampStr string
		var title string
		var data []byte

		err = stmt.Scan(&idStr, &timestampStr, &title, &data)
		if err != nil {
			break
		}

		id, err := uuid.Parse(idStr)
		if err != nil {
			return results, err
		}

		timestamp, err := time.Parse(time.RFC3339Nano, timestampStr)
		// construct item
		s := Snip{
			UUID:      id,
			Timestamp: timestamp,
			Title:     title,
			Data:      data,
		}
		results = append(results, s)
	}
	return results, nil
}

// New returns a new snippet and generates a new UUID for it
func New() (Snip, error) {
	return Snip{
		Data:      []byte{},
		Timestamp: time.Now(),
		Title:     "",
		UUID:      uuid.New(),
	}, nil
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

		s, err := GetFromUUID(path, idStr)
		if err != nil {
			return searchResult, err
		}
		searchResult = append(searchResult, s)
	}

	return searchResult, nil
}

// SearchUUID returns a slice of Snips with uuids matching partial search term
func SearchUUID(path string, term string) ([]Snip, error) {
	var searchResult []Snip
	if term == "" {
		return searchResult, fmt.Errorf("refusing to search for empty string")
	}
	conn, err := sqlite3.Open(path)
	if err != nil {
		return searchResult, err
	}
	defer conn.Close()

	termFuzzy := "%" + term + "%"
	stmt, err := conn.Prepare(`SELECT uuid from snip where uuid LIKE ?`, termFuzzy)
	if err != nil {
		return searchResult, err
	}
	defer stmt.Close()

	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return searchResult, err
		}
		if !hasRow {
			break
		}

		var idStr string
		err = stmt.Scan(&idStr)
		if err != nil {
			// TODO scrutinize this
			break
		}
		s, err := GetFromUUID(path, idStr)
		if err != nil {
			return searchResult, err
		}
		searchResult = append(searchResult, s)
	}
	return searchResult, nil
}
