package snip

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"os"
	"testing"
)

var DATABASE_PATH = "test.sqlite3"
var UUID_TEST = uuid.New()
var DATA_TEST = []byte("this is VeRy UnIQu3 sample data")

func TestMain(m *testing.M) {
	code := m.Run()

	// remove database after all tests have run
	err := os.Remove(DATABASE_PATH)
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
	err := CreateNewDatabase(DATABASE_PATH)
	if err != nil {
		t.Errorf("error creating new sqlite database: %v", err)
	}
}

func TestInsertSnip(t *testing.T) {
	err := CreateNewDatabase(DATABASE_PATH)
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
	err = InsertSnip(DATABASE_PATH, s)
	if err != nil {
		t.Errorf("error inserting snip: %v", err)
	}
}

func TestGetFromUUID(t *testing.T) {
	s, err := GetFromUUID(DATABASE_PATH, UUID_TEST)
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
