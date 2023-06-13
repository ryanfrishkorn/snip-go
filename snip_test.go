package snip

import (
	"bytes"
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
)

var DATABASE_PATH = "test.sqlite3"
var UUID_TEST = uuid.New()
var DATA_TEST = []byte("this is VeRy UnIQu3 sample data")
var conn *sqlite3.Conn

func TestMain(m *testing.M) {
	var err error
	conn, err = sqlite3.Open(DATABASE_PATH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening sqlite test database")
	}
	code := m.Run()

	// remove database after all tests have run
	err = os.Remove(DATABASE_PATH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error removing temporary testing database %s: %v", DATABASE_PATH, err)
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
	err := CreateNewDatabase(conn)
	if err != nil {
		t.Errorf("error creating new sqlite database: %v", err)
	}
}

func TestInsertSnip(t *testing.T) {
	err := CreateNewDatabase(conn)
	if err != nil {
		t.Errorf("error createing new sqlite database: %v", err)
	}

	s, err := New()
	if err != nil {
		t.Errorf("error creating new snip")
	}
	s.Data = []byte(DATA_TEST)

	// hijack for testing
	s.UUID = UUID_TEST
	err = InsertSnip(conn, s)
	if err != nil {
		t.Errorf("error inserting snip: %v", err)
	}
}

func TestGetFromUUID(t *testing.T) {
	s, err := GetFromUUID(conn, UUID_TEST.String())
	if err != nil {
		t.Errorf("error retrieving uuid %s: %v", UUID_TEST, err)
	}

	// check integrity
	if s.UUID != UUID_TEST {
		t.Errorf("expected UUID of %s, got %s", UUID_TEST.String(), s.UUID.String())
	}
	if bytes.Compare(s.Data, DATA_TEST) != 0 {
		t.Errorf("expected snip data and DATA_TEST to be equal, Compare returned non-zero")
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

func TestSnip_GenerateTitle(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Errorf("error generating new snip: %v", err)
	}
	s.Data = []byte("My day   at\n the\taquarium started out")

	expected := "My day at the aquarium"
	modified := s.GenerateTitle(5)
	if strings.Compare(expected, modified) != 0 {
		t.Errorf(`expected string "%s", got "%s"`, expected, modified)
	}
}
