package snip

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/ryanfrishkorn/snip/database"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var DatabasePath = "test.sqlite3"
var UUIDTest = uuid.New()
var DataTest = "this is VeRy UnIQu3 sample data, and stemming is good for searching"
var NameTest = "Test Snip of the Century"

// AddDataCSV adds data to the test database
func AddDataCSV() error {
	// TODO check for exising database, we must create it from scratch
	_, err := os.Stat(DatabasePath)
	if err == nil {
		return fmt.Errorf("test database %s already exists, remove and test again", DatabasePath)
	}

	cmd := exec.Command("sqlite3", DatabasePath, ".mode csv", ".import --csv testing/snip.csv snip")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during snip CSV import: %v", err)
	}

	cmd = exec.Command("sqlite3", DatabasePath, ".mode csv", ".import --csv testing/snip_attachment.csv snip_attachment")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during snip_attachment CSV import: %v", err)
	}
	return nil
}

// AddWikiData converts xml data to snip objects and adds them for testing
func AddWikiData(file string) error {

	type page struct {
		Title    string `xml:"title"`
		Revision struct {
			ID        int    `xml:"id"`
			Timestamp string `xml:"timestamp"`
			Text      string `xml:"text"`
		} `xml:"revision"`
	}

	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()

	d := xml.NewDecoder(zr)
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return fmt.Errorf("decoding token: %v", err)
		}
		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local == "page" {
				var doc page
				if err := d.DecodeElement(&doc, &t); err != nil {
					return err
				}
				// log.Debug().Str("title", doc.Title).Msg("document parsed")

				s := New()
				s.Data = doc.Revision.Text
				s.Name = doc.Title
				s.Timestamp, err = time.Parse(time.RFC3339, doc.Revision.Timestamp)
				if err != nil {
					return err
				}

				err = InsertSnip(s)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func TestMain(m *testing.M) {
	var err error

	// add basic data from CSV before opening database below
	err = AddDataCSV()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error importing CSV data to test database: %v", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "finished CSV import\n")

	database.Conn, err = sqlite3.Open(DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening sqlite test database")
		os.Exit(1)
	}

	// close database after all tests have run
	defer func() {
		database.Conn.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error closing test database %s: %v", DatabasePath, err)
			os.Exit(1)
		}
	}()

	/*
		err = AddWikiData("testing/enwiki-partial.xml.gz")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error importing Wikipedia data to test database: %v", err)
			os.Exit(1)
		}
	*/

	code := m.Run()

	// remove database file
	err = os.Remove(DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error removing temporary test database %s: %v", DatabasePath, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func TestNew(t *testing.T) {
	result := New()

	// default empty bytes
	if len(result.Data) != 0 {
		t.Errorf("result.Data expected zero length data, got %v", result.Data)
	}
}

func TestCreateNewDatabase(t *testing.T) {
	err := CreateNewDatabase()
	if err != nil {
		t.Errorf("error creating new sqlite database: %v", err)
	}
}

func TestInsertSnip(t *testing.T) {
	err := CreateNewDatabase()
	if err != nil {
		t.Errorf("error createing new sqlite database: %v", err)
	}

	s := New()
	s.Name = NameTest
	s.Data = DataTest

	// hijack for testing
	s.UUID = UUIDTest
	err = InsertSnip(s)
	if err != nil {
		t.Errorf("error inserting snip: %v", err)
	}
}

func TestGetFromUUID(t *testing.T) {
	s, err := GetFromUUID(UUIDTest.String())
	if err != nil {
		t.Errorf("error retrieving uuid %s: %v", UUIDTest, err)
	}

	// check integrity
	if s.UUID != UUIDTest {
		t.Errorf("expected UUID of %s, got %s", UUIDTest.String(), s.UUID.String())
	}
	if strings.Compare(s.Data, DataTest) != 0 {
		t.Errorf("expected snip data and DataTest to be equal, Compare returned non-zero")
	}
}

func TestFlattenString(t *testing.T) {
	original := "This is  a\n\nstring that\thas\t\tlots of  whitespace."
	expected := "This is a string that has lots of whitespace."
	modified := FlattenString(original)
	if strings.Compare(expected, modified) != 0 {
		t.Errorf(`expected string "%s", got "%s"`, expected, modified)
	}
}

func TestSnipCountWords(t *testing.T) {
	s := New()
	s.Data = "This data\tcontains  eight words\nin its entirety."
	expected := 8
	count := s.CountWords()
	if expected != count {
		t.Errorf("expected %d, got %d", expected, count)
	}
}

func TestSnipGenerateName(t *testing.T) {
	s := New()
	s.Data = "My day   at\n the\taquarium started out"

	expected := "My day at the aquarium"
	modified := s.GenerateName(5)
	if strings.Compare(expected, modified) != 0 {
		t.Errorf(`expected string "%s", got "%s"`, expected, modified)
	}
}

func TestSnipUpdate(t *testing.T) {
	s := New()
	id := s.UUID
	s.Data = DataTest
	s.Name = "test"
	err := InsertSnip(s)
	if err != nil {
		t.Fatal(err)
	}

	// cleanup - leave it the way you found it
	defer func() {
		err := Delete(id)
		if err != nil {
			t.Fatalf("delete function returned error: %v", err)
		}
	}()

	s.Name = "test2"
	err = s.Update()
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	c, err := GetFromUUID(id.String())
	if err != nil {
		t.Error(err)
	}
	if c.Name != "test2" {
		// update must have failed
		t.Error("database update failed")
	}
	// TODO modify and verify changes on all fields
}

func TestSnipIndex(t *testing.T) {
	ids, err := GetAllSnipIDs()
	if err != nil {
		t.Errorf("could not get all snip ids: %v", err)
	}

	for _, id := range ids {
		s, err := GetFromUUID(id.String())
		if err != nil {
			t.Errorf("could not obtain snip %s: %v", id, err)
		}

		err = s.Index()
		if err != nil {
			t.Error(err)
		}
	}
}

func TestSplitWords(t *testing.T) {
	text := `This is simple test data. Let's keep it simple, for the time being.
This is the second line.`
	expect := []string{
		"This",
		"is",
		"simple",
		"test",
		"data.",
		"Let's",
		"keep",
		"it",
		"simple,",
		"for",
		"the",
		"time",
		"being.",
		"This",
		"is",
		"the",
		"second",
		"line.",
	}
	textSplit := SplitWords(text)
	// t.Logf("expect: %v, %d", expect, len(expect))
	// t.Logf("   got: %v, %d", textSplit, len(textSplit))

	validate := func(a []string, b []string) bool {
		for idx := range a {
			if strings.Compare(a[idx], b[idx]) != 0 {
				t.Errorf(`"%s" != "%s"`, a[idx], b[idx])
				return false
			}
		}
		return true
	}
	if !validate(expect, textSplit) {
		t.Errorf("word slices failed comparison, \nexpected: %v\n      got %v", expect, textSplit)
	}
}
