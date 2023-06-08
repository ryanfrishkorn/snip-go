package snip

import (
	"os"
	"testing"
)

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
	databasePath := "test.sqlite3"

	err := CreateNewDatabase(databasePath)
	defer func() {
		err := os.Remove(databasePath)
		if err != nil {
			t.Errorf("error removing test database file: %v", err)
		}
	}()
	if err != nil {
		t.Errorf("error creating new sqlite database: %v", err)
	}
}

func TestInsertSnip(t *testing.T) {
	databasePath := "test.sqlite3"

	err := CreateNewDatabase(databasePath)
	if err != nil {
		t.Errorf("error createing new sqlite database: %v", err)
	}
	defer os.Remove(databasePath)

	s, err := New()
	if err != nil {
		t.Errorf("error creating new snip")
	}
	s.Data = []byte("test data")

	err = InsertSnip(databasePath, s)
	if err != nil {
		t.Errorf("error inserting snip: %v", err)
	}
}
