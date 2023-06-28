package snip

import (
	"embed"
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/kljensen/snowball"
	"github.com/rs/zerolog/log"
	"github.com/ryanfrishkorn/snip/database"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	//go:embed words_princeton.txt
	f             embed.FS
	wordsBytes, _ = f.ReadFile("words_princeton.txt")
)

// SearchCount contains info about a search term frequency from the index
type SearchCount struct {
	Term  string
	Count int
}

type SearchResult struct {
	UUID  uuid.UUID
	Terms []SearchCount
}

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

	err = stmt.Exec(a.UUID.String(), a.SnipUUID.String(), a.Timestamp.Format(time.RFC3339Nano), a.Name, a.Data, len(a.Data))
	if err != nil {
		return err
	}
	return nil
}

// CountWords returns an integer estimating the number of words in data
func (s *Snip) CountWords() int {
	data := FlattenString(string(s.Data))
	re := regexp.MustCompile(`\s+`)
	return len(re.Split(data, -1))
}

// GenerateName returns a clean string derived from processing the data field
func (s *Snip) GenerateName(wordCount int) string {
	data := FlattenString(string(s.Data))
	// FIXME by allowing additional sensible characters such as `:`
	pattern := regexp.MustCompile(`\w+`)
	name := pattern.FindAllString(data, wordCount)
	return strings.Join(name, " ")
}

// Index stems all data and writes it to a search table
func (s *Snip) Index() error {
	// parse into valid words
	var wordsEmbedded []string
	var wordsStemmed []string
	var dataStemmed []string

	wordsEmbedded = append(wordsEmbedded, strings.Split(string(wordsBytes), "\n")...)
	// log.Debug().Int("wordsEmbedded", len(wordsEmbedded)).Msg("word dict count")
	// log.Debug().Str("wordsEmbedded[0]", wordsEmbedded[0]).Msg("first word")
	// FIXME - this stem dictionary should be reused so as to not do the work for every snip
	for _, word := range wordsEmbedded {
		stem, err := snowball.Stem(word, "english", true)
		if err != nil {
			return err
		}
		wordsStemmed = append(wordsStemmed, stem)
	}
	// log.Debug().Int("wordsStemmed", len(wordsStemmed)).Msg("word stem count")
	// log.Debug().Str("wordsStemmed[0]", wordsStemmed[0]).Msg("first stem")

	// TODO: remove stop words from dict

	// remove commas and periods
	dataCleaned := strings.ReplaceAll(string(s.Data), ". ", " ")
	dataCleaned = strings.ReplaceAll(dataCleaned, ", ", " ")

	// dataStemmed, err := snowball.Stem(dataCleaned, "english", true)
	dataCleanedSplit := strings.Split(dataCleaned, " ")
	for _, word := range dataCleanedSplit {
		stem, err := snowball.Stem(word, "english", true)
		if err != nil {
			return err
		}
		dataStemmed = append(dataStemmed, stem)
	}

	// build terms and counts
	terms := make(map[string]int, 0)
	for _, term := range dataStemmed {
		// determine if term has already been processed
		_, ok := terms[term]
		if ok {
			// skip
			continue
		}

		// determine if stem is valid
		valid := false
		for _, wordStem := range wordsStemmed {
			if wordStem == term {
				valid = true
				// log.Debug().Str("term", term).Msg("valid")
				break
			}
		}
		if !valid {
			// log.Debug().Str("term", term).Msg("did not match dictionary")
			continue
		}

		// count occurrences
		count := 0
		for _, t := range dataStemmed {
			if term == t {
				count++
			}
		}
		terms[term] = count
		// log.Debug().Str("term", term).Int("count", count).Msg("indexing stem")
	}
	for term, count := range terms {
		err := s.SetIndexTermCount(term, count)
		if err != nil {
			return err
		}
	}

	return nil
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

// SetIndexTermCount inserts or updates the count of a term indexed
func (s *Snip) SetIndexTermCount(term string, count int) error {
	countCurrent, err := GetIndexTermCount(term, s.UUID)
	if err != nil {
		return err
	}

	var stmt *sqlite3.Stmt
	if countCurrent != 0 {
		// remove current count and replace with new count
		stmt, err = database.Conn.Prepare(`UPDATE snip_index SET count = ? WHERE term = ? AND uuid = ?`)
		if err != nil {
			return err
		}
		err = stmt.Exec(count, term, s.UUID.String())
		if err != nil {
			return err
		}
	} else {
		stmt, err = database.Conn.Prepare(`INSERT INTO snip_index (term, uuid, count) VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		err = stmt.Exec(term, s.UUID.String(), count)
		if err != nil {
			return err
		}
	}
	stmt.Close()
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
	err = database.Conn.Exec(`CREATE TABLE IF NOT EXISTS snip_index(term TEXT, uuid TEXT, count INTEGER)`)
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

// DropIndex drops the search index from the database
func DropIndex() error {
	stmt, err := database.Conn.Prepare(`DELETE FROM snip_index`)
	if err != nil {
		return err
	}
	err = stmt.Exec()
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

// GetAllSnipIDs returns a slice of all known snip uuids
func GetAllSnipIDs() ([]uuid.UUID, error) {
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
		if err != nil {
			return results, err
		}
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
		s.Name = name
		s.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
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

// GetIndexTermCount returns the index count for a term matching id
func GetIndexTermCount(term string, id uuid.UUID) (int, error) {
	var matches = 0
	// return zero if nothing matches (which should not be present in database)
	stmt, err := database.Conn.Prepare(`SELECT count from snip_index WHERE term = ? AND uuid = ?`)
	if err != nil {
		return matches, err
	}
	defer stmt.Close()

	err = stmt.Exec(term, id.String())
	if err != nil {
		return matches, err
	}
	hasRow, err := stmt.Step()
	if err != nil {
		return matches, err
	}
	if !hasRow {
		return matches, err
	}
	err = stmt.Scan(&matches)
	if err != nil {
		return matches, err
	}
	return matches, nil
}

// InsertSnip adds a new Snip to the database
func InsertSnip(s Snip) error {
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
		if err != nil {
			return results, err
		}
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
		if err != nil {
			return results, err
		}
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
func New() Snip {
	return Snip{
		Data:      []byte{},
		Timestamp: time.Now(),
		Name:      "",
		UUID:      uuid.New(),
	}
}

// SearchDataTerm returns a slice of Snips whose data matches supplied terms
func SearchDataTerm(term string) ([]Snip, error) {
	var searchResult []Snip
	if term == "" {
		return searchResult, fmt.Errorf("refusing to search for empty string")
	}

	// modify term for fuzziness
	termFuzzy := "%" + term + "%"
	stmt, err := database.Conn.Prepare(`SELECT uuid from snip where data LIKE ?`, termFuzzy)
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

// SearchIndexTerm searches the index and returns results matching the given term
func SearchIndexTerm(terms []string, limit int) (map[uuid.UUID][]SearchCount, error) {
	var searchResults = make(map[uuid.UUID][]SearchCount, 0)
	var limitSet bool = false

	if limit != 0 {
		limitSet = true
	}

	if len(terms) <= 0 {
		return searchResults, fmt.Errorf("refusing to search for empty string")
	}

	for _, term := range terms {
		// stem the term
		termStemmed, err := snowball.Stem(term, "english", true)
		log.Debug().Str("termStemmed", termStemmed).Msg("term stemmed")

		stmt, err := database.Conn.Prepare(`SELECT uuid, count FROM snip_index WHERE term = ?`, termStemmed)
		if err != nil {
			return searchResults, err
		}
		defer stmt.Close()

		for {
			hasRow, err := stmt.Step()
			if err != nil {
				return searchResults, err
			}
			if !hasRow {
				break
			}
			// mind limit
			if limitSet {
				if len(searchResults) > limit {
					break
				}
			}

			var (
				idStr string
				count int
			)
			err = stmt.Scan(&idStr, &count)
			if err != nil {
				return searchResults, err
			}
			id, err := uuid.Parse(idStr)
			if err != nil {
				return searchResults, err
			}
			result := SearchCount{
				Term:  term,
				Count: count,
			}
			searchResults[id] = append(searchResults[id], result)
		}
	}

	return searchResults, nil
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
	if err == nil && !forceWrite {
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
