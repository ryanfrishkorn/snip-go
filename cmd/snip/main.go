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
	"time"
)

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

	helpOutput := func() {
		fmt.Printf("valid subcommands:\n")
		fmt.Printf("  add - add a new snip\n")
		fmt.Printf("  get - retrieve snip with specified uuid\n")
		fmt.Printf("  ls - list all snips\n")
		fmt.Printf("  search - return snips whose data contains given term\n")
		os.Exit(1)
	}

	// establish action
	if len(os.Args) < 2 {
		helpOutput()
	}
	action := os.Args[1]

	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addDataFromFile := addCmd.String("f", "", "use data from specified file")

	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getRawData := getCmd.Bool("raw", false, "output only raw data")

	listCmd := flag.NewFlagSet("ls", flag.ExitOnError)
	listLimit := listCmd.Int("l", 0, "limit results")

	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	// fuzzy data search by default unless field is specified
	searchField := searchCmd.String("f", "data", "field to search")

	// ensure database is present
	err := snip.CreateNewDatabase(dbFilePath)
	if err != nil {
		log.Fatal().Err(err).Msg("error opening database")
	}

	log.Debug().Str("action", action).Msg("action invoked")

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
		// generate title if empty
		if s.Title == "" {
			s.Title = s.GenerateTitle(5)
		}

		log.Debug().
			Str("UUID", s.UUID.String()).
			Str("timestamp", s.Timestamp.String()).
			Str("title", s.Title).
			Bytes("Data", s.Data).
			Msg("first snip object")
		err = snip.InsertSnip(dbFilePath, s)
		if err != nil {
			log.Fatal().Err(err).Msg("error inserting Snip into database")
		}
		fmt.Printf("successfully added snip with uuid: %s\n", s.UUID)

	case "get":
		if err := getCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing get arguments")
		}
		idStr := getCmd.Args()[0]
		id, err := uuid.Parse(idStr)
		if err != nil {
			log.Fatal().Err(err).Msg("error converting from bytes to uuid type")
		}
		s, err := snip.GetFromUUID(dbFilePath, id)
		if err != nil {
			log.Fatal().Err(err).Str("uuid", id.String()).Msg("error retrieving snip with uuid")
		}

		if *getRawData {
			fmt.Printf("%s", s.Data)
		} else {
			fmt.Printf("uuid: %s\n", s.UUID.String())
			fmt.Printf("timestamp: %s\n", s.Timestamp.Format(time.RFC3339Nano))
			fmt.Printf("data: \n")
			fmt.Printf("----\n")
			fmt.Printf("%s", s.Data)
			fmt.Printf("\n----\n")
		}

	case "ls":
		if err := listCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing search arguments")
		}
		results, err := snip.List(dbFilePath, *listLimit)
		if err != nil {
			log.Fatal().Err(err).Msg("error listing items")
		}
		fmt.Printf("results: %d\n", len(results))
		for _, s := range results {
			fmt.Printf("%s %s\n", s.UUID, s.Title)
		}

	case "search":
		if err := searchCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing search arguments")
		}

		var results []snip.Snip
		term := searchCmd.Args()[0]
		fmt.Printf("searching data field for: %s\n", term)
		switch *searchField {
		case "data":
			results, err = snip.SearchDataTerm(dbFilePath, term)
			if err != nil {
				log.Fatal().Err(err).Msg("error while searching for term")
			}

		case "uuid":
			results, err = snip.SearchUUID(dbFilePath, term)
			if err != nil {
				log.Fatal().Err(err).Msg("error while searching for term")
			}
		}

		fmt.Printf("results: %d\n\n", len(results))
		for _, s := range results {
			printSearchResult(s)
			fmt.Printf("\n")
		}

	default:
		helpOutput()
	}

	log.Debug().Msg("program execution complete")
}

// printSearchResult prints a summary of a result
func printSearchResult(s snip.Snip) {
	// truncate data for display
	maxChars := 70
	// dataSummary := snip.FlattenString(string(s.Data))
	dataSummary := string(s.Data)

	if len(dataSummary) < maxChars {
		maxChars = len(dataSummary)
	}

	dataSummary = dataSummary[:maxChars]
	fmt.Printf("uuid: %s timestamp: %s\ntitle: %s words: %d\n----\n%s\n----\n",
		s.UUID.String(),
		s.Timestamp.Format(time.RFC3339Nano),
		s.GenerateTitle(5),
		s.CountWords(),
		dataSummary)
}

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
