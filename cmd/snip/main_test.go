package main_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

var (
	appName     string
	appPath     string
	workingPath string
	dbName      = "test.sqlite"
)

func TestMain(m *testing.M) {
	var err error

	// assign testing globals
	appName = "snip"
	workingPath, err = os.Getwd()
	if err != nil {
		fmt.Printf("error getting working directory: %v", err)
		os.Exit(1)
	}
	appPath = path.Join(workingPath, appName)

	// build tool
	fmt.Printf("building tool...\n")
	build := exec.Command("go", "build", "-o", appName)
	if err = build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// set env for test database location
	err = os.Setenv("SNIP_DB", path.Join(workingPath, dbName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error setting db path for testing: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("running tests...\n")
	result := m.Run()

	// remove binary after testing
	err = os.Remove(appName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error removing testing binary\n")
		os.Exit(1)
	}

	os.Exit(result)
}

func TestList(t *testing.T) {
	snipCount := 3   // number of snips in test database
	snipColumns := 2 // number of output columns when listing

	t.Run("List", func(t *testing.T) {
		cmd := exec.Command(appPath, "ls")
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			t.Errorf("error opening stdout pipe: %v", err)
		}

		err = cmd.Start()
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}

		// read from program stdout
		buffer := bufio.NewReader(stdoutPipe)
		var outputLines []string

		for {
			line, err := buffer.ReadBytes('\n')
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal("error reading line")
			}
			outputLines = append(outputLines, string(line))
		}

		err = cmd.Wait()
		if err != nil {
			t.Errorf("error waiting for stdout pipe: %v", err)
		}

		// process output
		if len(outputLines) == 0 {
			t.Fatal("expected some bytes read from stdout pipe, got zero")
		}
		if len(outputLines) != snipCount {
			t.Errorf("expected %d lines, got %d", snipCount, len(outputLines))
			t.Errorf("line: %v", outputLines)
		}

		var ids []string
		for _, line := range outputLines {
			lineSplit := strings.Split(line, " ")
			if len(lineSplit) < 2 {
				t.Errorf("expected at least %d columns in list output, got %d", snipColumns, len(lineSplit))
			}
			ids = append(ids, lineSplit[0])
		}
		// only check for expected uuids, since other display aspects are likely to change
		expectedIDs := []string{"65f6930f-e970-4b6e-b10c-fca3dac21c1e", "990a917e-66d3-404b-9502-e8341964730b", "412f7ca8-824c-4c70-80f0-4cca6371e45a"}
		for idx, id := range expectedIDs {
			if ids[idx] != id {
				t.Errorf("expected id %s, got %s", expectedIDs[idx], id)
			}
		}
	})
}
