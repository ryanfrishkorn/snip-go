package snip

import (
	"bytes"
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/ryanfrishkorn/snip/database"
	"os"
	"strings"
	"testing"
)

var DatabasePath = "test.sqlite3"
var UUIDTest = uuid.New()
var DataTest = []byte("this is VeRy UnIQu3 sample data")

func TestMain(m *testing.M) {
	var err error
	database.Conn, err = sqlite3.Open(DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening sqlite test database")
		os.Exit(1)
	}
	code := m.Run()

	// remove database after all tests have run
	err = database.Conn.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error closing test database %s: %v", DatabasePath, err)
		os.Exit(1)
	}
	err = os.Remove(DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error removing temporary test database %s: %v", DatabasePath, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func TestNew(t *testing.T) {
	result, err := New()
	if err != nil {
		t.Errorf("error creating new snip struct: %v", err)
	}

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

	s, err := New()
	if err != nil {
		t.Errorf("error creating new snip")
	}
	s.Data = []byte(DataTest)

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
	if bytes.Compare(s.Data, DataTest) != 0 {
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

func TestSnip_CountWords(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Errorf("error generating new snip: %v", err)
	}
	s.Data = []byte("This data contains eight words in its entirety")
	expected := 8
	count := s.CountWords()
	if expected != count {
		t.Errorf("expected %d, got %d", expected, count)
	}
}

func TestSnip_GenerateName(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Errorf("error generating new snip: %v", err)
	}
	s.Data = []byte("My day   at\n the\taquarium started out")

	expected := "My day at the aquarium"
	modified := s.GenerateName(5)
	if strings.Compare(expected, modified) != 0 {
		t.Errorf(`expected string "%s", got "%s"`, expected, modified)
	}
}

func TestSnip_Update(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	id := s.UUID
	s.Data = DataTest
	s.Name = "test"
	err = InsertSnip(s)
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
