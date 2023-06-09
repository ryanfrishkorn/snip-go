package main

import (
	"flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ryanfrishkorn/snip"
	"io"
	"os"
	"time"
)

// ProgramOptions contains all options derived from command invocation
type ProgramOptions struct {
	debug        *bool
	databasePath *string
	dataFromFile *string
	help         *bool
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

func main() {
	// preliminaries
	homePath := os.Getenv("HOME")
	dbFilename := ".snip.sqlite3"
	if homePath == "" {
		log.Fatal().Msg("could not retrieve $HOME environment variable")
	}

	// parse options
	var options = ProgramOptions{}
	options.databasePath = flag.String("d", homePath+"/"+dbFilename, "database file location")
	options.dataFromFile = flag.String("f", "", "use data from specified file")
	// options.dataFromStdin = flag.Bool("stdin", true, "use data from standard input")
	options.debug = flag.Bool("debug", false, "enable debug logging")
	options.help = flag.Bool("help", false, "print help information")
	flag.Parse()

	// configure logging
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *options.debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano

	log.Info().Msg("program execution start")

	// ensure database is present
	snip.CreateNewDatabase(*options.databasePath)

	// create simple object
	s, err := snip.New()
	if err != nil {
		log.Fatal().Msg("could not create new Snip")
	}

	// file input takes precedence, but default to standard input
	if *options.dataFromFile != "" {
		data, err := readFromFile(*options.dataFromFile)
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

	log.Info().Str("UUID", s.UUID.String()).Bytes("Data", s.Data).Msg("first snip object")
	err = snip.InsertSnip(*options.databasePath, s)
	if err != nil {
		log.Fatal().Err(err).Msg("error inserting Snip into database")
	}

	log.Info().Msg("program execution complete")
}
