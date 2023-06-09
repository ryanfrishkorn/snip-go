package main

import (
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ryanfrishkorn/snip"
	"io"
	"os"
	"strings"
	"time"
)

// readFromFile reads all data from specified file
func readFromFile(path string) ([]byte, error) {
	// TODO check file size for sanity to avoid polluting a database
	f, err := os.ReadFile(path)
	if err != nil {
		return []byte{}, err
	}
	return f, nil
}

// readFromStdin reads all data from standard input
func readFromStdin() ([]byte, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

func main() {
	// configure logging
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	zerolog.TimeFieldFormat = time.RFC3339Nano
	optionDebug := os.Getenv("DEBUG")
	if optionDebug != "" && optionDebug != "0" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// preliminaries
	homePath := os.Getenv("HOME")
	dbFilename := ".snip.sqlite3"
	if homePath == "" {
		log.Fatal().Msg("could not retrieve $HOME environment variable")
	}
	dbFilePath := homePath + "/" + dbFilename

	// establish action
	if len(os.Args) < 2 {
		log.Fatal().Msg("please use a valid action")
	}
	action := os.Args[1]

	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addDataFromFile := addCmd.String("f", "", "use data from specified file")

	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getByUUID := getCmd.String("u", "", "retrieve by snip UUID")

	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)

	// ensure database is present
	err := snip.CreateNewDatabase(dbFilePath)
	if err != nil {
		log.Fatal().Err(err).Msg("error opening database")
	}

	fmt.Printf("action: %s\n", action)
	helpOutput := func() {
		fmt.Printf("valid subcommands:\n")
		fmt.Printf("  add - add a new snip to the database\n")
		fmt.Printf("  get - retrieve a snip from the database\n")
		fmt.Printf("  search - return snips whose data contains given term\n")
		os.Exit(1)
	}

	switch action {
	case "add":
		if err := addCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing add arguments")
		}

		// create simple object
		s, err := snip.New()
		if err != nil {
			log.Fatal().Msg("could not create new Snip")
		}

		// file input takes precedence, but default to standard input
		if *addDataFromFile != "" {
			data, err := readFromFile(*addDataFromFile)
			if err != nil {
				log.Fatal().Err(err).Msg("error reading from file")
			}
			s.Data = data
		} else {
			data, err := readFromStdin()
			if err != nil {
				log.Fatal().Msg("error reading from standard input")
			}
			s.Data = data
		}

		log.Debug().Str("UUID", s.UUID.String()).Bytes("Data", s.Data).Msg("first snip object")
		err = snip.InsertSnip(dbFilePath, s)
		if err != nil {
			log.Fatal().Err(err).Msg("error inserting Snip into database")
		}
		fmt.Printf("successfully added snip with uuid: %s\n", s.UUID)

	case "get":
		if err := getCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing get arguments")
		}
		id, err := uuid.Parse(*getByUUID)
		if err != nil {
			log.Fatal().Err(err).Msg("error converting from bytes to uuid type")
		}
		s, err := snip.GetFromUUID(dbFilePath, id)
		if err != nil {
			log.Fatal().Err(err).Str("uuid", *getByUUID).Msg("error retrieving snip with uuid")
		}
		fmt.Printf("uuid: %s\n", s.UUID.String())
		fmt.Printf("data: %s\n", s.Data)

	case "search":
		if err := searchCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing search arguments")
		}
		term := searchCmd.Args()[0]
		fmt.Printf("term: %s\n", term)
		results, err := snip.SearchDataTerm(dbFilePath, term)
		if err != nil {
			log.Fatal().Err(err).Msg("error while searching for term")
		}
		fmt.Printf("results: %d\n", len(results))
		for idx, s := range results {
			fmt.Printf("index: %d uuid: %s data: %s\n", idx, s.UUID.String(), strings.TrimSuffix(string(s.Data), "\n"))
		}

	default:
		helpOutput()
	}

	log.Debug().Msg("program execution complete")
}
