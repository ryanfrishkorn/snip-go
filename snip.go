package snip

import (
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/ryanfrishkorn/snip/database"
	"os"
	"regexp"
	"strings"
	"time"
)

// Snip represents a snippet of data with additional metadata
type Snip struct {
	Attachments []Attachment
	Data        []byte
	Timestamp   time.Time
	Name        string
	UUID        uuid.UUID
}

// Attach adds files associated with a snip
func (s *Snip) Attach(name string, data []byte) error {
	// build and insert attachment
	a := NewAttachment()
	a.Data = data
	a.Name = name
	a.SnipUUID = s.UUID

	stmt, err := database.Conn.Prepare(`INSERT INTO snip_attachment (uuid, snip_uuid, timestamp, name, data, size) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	err = stmt.Exec(a.UUID.String(), a.SnipUUID.String(), a.Timestamp.String(), a.Name, a.Data, len(a.Data))
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

// GenerateName returns a clean string derived from processing the data field
func (s *Snip) GenerateName(wordCount int) string {
	data := FlattenString(string(s.Data))
	// FIXME by allowing additional sensible characters such as `:`
	pattern := regexp.MustCompile(`\w+`)
	name := pattern.FindAllString(data, wordCount)
	return strings.Join(name, " ")
}

// Rename updates the name field of a snip
func (s *Snip) Rename(newName string) error {
	s.Name = newName
	err := s.Update()
	if err != nil {
		return err
	}
	return nil
}

// Update writes all fields, overwriting existing snip data
func (s *Snip) Update() error {
	// verify that current record is present and unique
	stmt, err := database.Conn.Prepare(`SELECT count() FROM snip where uuid = ?`, s.UUID.String())
	if err != nil {
		return err
	}
	defer stmt.Close()

	// update
	err = stmt.Exec()
	if err != nil {
		return err
	}
	count := 0
	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return err
		}
		if !hasRow {
			break
		}
		count++

		var n int
		err = stmt.Scan(&n)
		if err != nil {
			return fmt.Errorf("error scanning for value during update preliminary check")
		}
		if n != 1 {
			return fmt.Errorf("count query returned more than one row, meaning uuid may have duplicate entries")
		}
	}

	if count != 1 {
		return fmt.Errorf("should have returned 1 snip record, found %d", count)
	}

	// FIXME handle attachments
	// update the record
	stmt2, err := database.Conn.Prepare(`UPDATE snip SET (data, timestamp, name) = (?, ?, ?) WHERE uuid = ?`)
	if err != nil {
		return err
	}
	defer stmt2.Close()

	err = stmt2.Exec(string(s.Data), s.Timestamp.Format(time.RFC3339Nano), s.Name, s.UUID.String())
	if err != nil {
		return err
	}
	return nil
}

// CreateNewDatabase creates a new sqlite3 database
func CreateNewDatabase() error {
	// build schema
	err := database.Conn.Exec(`CREATE TABLE IF NOT EXISTS snip(uuid TEXT, timestamp TEXT, name TEXT, data TEXT)`)
	if err != nil {
		return err
	}
	err = database.Conn.Exec(`CREATE TABLE IF NOT EXISTS snip_attachment(uuid TEXT, snip_uuid TEXT, timestamp TEXT, name TEXT, data BLOB, size INTEGER)`)
	if err != nil {
		return err
	}

	return nil
}

// Delete removes a snip from the database
func Delete(id uuid.UUID) error {
	// remove associated attachments
	attachments, err := GetAttachments(id)
	if err != nil {
		return err
	}
	for _, a := range attachments {
		err = DeleteAttachment(a.UUID)
		if err != nil {
			return err
		}
	}
	// remove
	stmt, err := database.Conn.Prepare(`DELETE from snip WHERE uuid = ?`, id.String())
	if err != nil {
		return err
	}
	defer stmt.Close()
	err = stmt.Exec()
	if err != nil {
		return err
	}
	return nil
}

// DeleteAttachment deletes an attachment from the database
func DeleteAttachment(id uuid.UUID) error {
	// remove
	stmt, err := database.Conn.Prepare(`DELETE FROM snip_attachment WHERE uuid = ?`, id.String())
	if err != nil {
		return err
	}
	defer stmt.Close()
	err = stmt.Exec()
	if err != nil {
		return err
	}
	return nil
}

func GetAllMetadata() ([]uuid.UUID, error) {
	var snipIDs []uuid.UUID

	stmt, err := database.Conn.Prepare(`SELECT uuid from snip`)
	if err != nil {
		return snipIDs, err
	}
	defer stmt.Close()

	err = stmt.Exec()
	if err != nil {
		return snipIDs, err
	}

	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return snipIDs, err
		}
		if !hasRow {
			break
		}
		var idStr string
		err = stmt.Scan(&idStr)
		if err != nil {
			return snipIDs, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return snipIDs, err
		}
		snipIDs = append(snipIDs, id)
	}
	return snipIDs, nil
}

// GetAttachmentsAll returns a slice of uuids for all attachments in the system
func GetAttachmentsAll() ([]uuid.UUID, error) {
	var attachmentIDs []uuid.UUID

	stmt, err := database.Conn.Prepare(`SELECT uuid from snip_attachment`)
	if err != nil {
		return attachmentIDs, err
	}
	defer stmt.Close()

	err = stmt.Exec()
	if err != nil {
		return attachmentIDs, err
	}

	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return attachmentIDs, err
		}
		if !hasRow {
			break
		}
		var idStr string
		err = stmt.Scan(&idStr)
		if err != nil {
			return attachmentIDs, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return attachmentIDs, err
		}
		attachmentIDs = append(attachmentIDs, id)
	}
	return attachmentIDs, nil
}

// GetAttachments returns a slice of Attachment associated with the supplied snip uuid
func GetAttachments(searchUUID uuid.UUID) ([]Attachment, error) {
	var attachments []Attachment

	ids, err := GetAttachmentsUUID(searchUUID)
	if err != nil {
		return attachments, err
	}

	for _, id := range ids {
		a, err := GetAttachmentFromUUID(id.String())
		if err != nil {
			return attachments, err
		}
		attachments = append(attachments, a)
	}
	return attachments, nil
}

// GetAttachmentsUUID returns a slice of attachment uuids associated with supplied snip uuid
func GetAttachmentsUUID(snipUUID uuid.UUID) ([]uuid.UUID, error) {
	var results []uuid.UUID

	stmt, err := database.Conn.Prepare(`SELECT uuid FROM snip_attachment WHERE snip_uuid = ?`)
	if err != nil {
		return results, err
	}
	defer stmt.Close()

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
func GetFromUUID(searchUUID string) (Snip, error) {
	s := Snip{}

	// determine exact or partial matching
	var exactMatch bool
	var maxLength = 36
	var err error
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
		stmt, err = database.Conn.Prepare(`SELECT uuid, data, timestamp, name FROM snip WHERE uuid = ?`, searchUUID)
	} else {
		searchUUIDFuzzy := "%" + searchUUID + "%"
		stmt, err = database.Conn.Prepare(`SELECT uuid, data, timestamp, name FROM snip WHERE uuid LIKE ?`, searchUUIDFuzzy)
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
		if err != nil {
			return s, err
		}
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
		var name string
		err = stmt.Scan(&id, &data, &timestamp, &name)
		if err != nil {
			return s, err
		}
		s.Data = []byte(data)
		s.UUID, err = uuid.Parse(id)
		if err != nil {
			return s, fmt.Errorf("error parsing uuid string into struct")
		}
		s.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		s.Name = name
		if err != nil {
			return s, err
		}
	}
	if resultCount == 0 {
		return s, fmt.Errorf("database search returned zero results")
	}

	// gather attachments
	s.Attachments, err = GetAttachments(s.UUID)
	if err != nil {
		return s, err
	}

	return s, nil
}

// InsertSnip adds a new Snip to the database
func InsertSnip(s Snip) error {
	// do not insert without data
	if len(s.Data) == 0 {
		return fmt.Errorf("refusing to insert zero-length data")
	}

	stmt, err := database.Conn.Prepare(`INSERT INTO snip VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// reference
	err = stmt.Exec(s.UUID.String(), s.Timestamp.Format(time.RFC3339Nano), s.Name, string(s.Data))
	if err != nil {
		return err
	}
	return nil
}

// List returns a slice of all Snips in the database
func List(limit int) ([]Snip, error) {
	var results []Snip
	var stmt *sqlite3.Stmt
	var err error

	if limit != 0 {
		stmt, err = database.Conn.Prepare(`SELECT uuid, timestamp, name, data from snip LIMIT ?`, limit)
		if err != nil {
			return results, err
		}
	} else {
		stmt, err = database.Conn.Prepare(`SELECT uuid, timestamp, name, data from snip`)
		if err != nil {
			return results, err
		}
	}
	defer stmt.Close()

	for {
		hasRow, err := stmt.Step()
		if !hasRow {
			break
		}

		var idStr string
		var timestampStr string
		var name string
		var data []byte

		err = stmt.Scan(&idStr, &timestampStr, &name, &data)
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
			Name:      name,
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
		Name:      "",
		UUID:      uuid.New(),
	}, nil
}

// SearchDataTerm returns a slice of Snips whose data matches supplied terms
func SearchDataTerm(term string) ([]Snip, error) {
	var searchResult []Snip
	if term == "" {
		return searchResult, fmt.Errorf("refusing to search for empty string")
	}

	// make term search fuzzy
	termFuzzy := "%" + term + "%"
	stmt, err := database.Conn.Prepare(`SELECT uuid from snip where data LIKE ?`, termFuzzy)
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
			// TODO revisit this logic, why not return error?
			break
		}

		s, err := GetFromUUID(idStr)
		if err != nil {
			return searchResult, err
		}
		searchResult = append(searchResult, s)
	}

	return searchResult, nil
}

// SearchUUID returns a slice of Snips with uuids matching partial search term
func SearchUUID(term string) ([]Snip, error) {
	var searchResult []Snip
	if term == "" {
		return searchResult, fmt.Errorf("refusing to search for empty string")
	}

	termFuzzy := "%" + term + "%"
	stmt, err := database.Conn.Prepare(`SELECT uuid from snip where uuid LIKE ?`, termFuzzy)
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
		s, err := GetFromUUID(idStr)
		if err != nil {
			return searchResult, err
		}
		searchResult = append(searchResult, s)
	}
	return searchResult, nil
}

// WriteAttachment writes the attached file to the current working directory
func WriteAttachment(id uuid.UUID, outfile string, forceWrite bool) (int, error) {
	a, err := GetAttachmentFromUUID(id.String())
	if err != nil {
		log.Debug().Err(err).Str("uuid", id.String()).Msg("error obtaining attachment from id")
		return 0, err
	}
	// attempt to open file for writing using filename
	_, err = os.Stat(outfile)
	if err == nil && forceWrite == false {
		// ESCAPE HATCH never overwrite data unless the issue is forced
		log.Debug().Str("filename", a.Name).Msg("stat returned no errors, refusing to overwrite file")
		return 0, fmt.Errorf("refusing to overwrite file")
	}
	// DESTRUCTIVE
	f, err := os.Create(outfile)
	if err != nil {
		log.Debug().Err(err).Msg("error opening new file for writing")
		return 0, err
	}
	bytesWritten, err := f.Write(a.Data)
	if err != nil {
		log.Debug().Err(err).Str("filename", a.Name).Msg("error attempting to write data to file")
		return 0, err
	}
	return bytesWritten, err
}
