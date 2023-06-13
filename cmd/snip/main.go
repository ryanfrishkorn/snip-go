package main

import (
	"flag"
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ryanfrishkorn/snip"
	"io"
	"os"
	"path"
	"sort"
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
		fmt.Printf("  attach - attach a file to specified snip\n")
		fmt.Printf("  get - retrieve snip with specified uuid\n")
		fmt.Printf("  ls - list all snips\n")
		fmt.Printf("  rm - remove snip <uuid> ...\n")
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
	addTitle := addCmd.String("t", "", "specify title")

	attachCmd := flag.NewFlagSet("attach", flag.ExitOnError)
	attachList := attachCmd.Bool("ls", false, "list all attachments")
	attachRemove := attachCmd.Bool("rm", false, "remove supplied attachment uuids")
	attachWrite := attachCmd.Bool("write", false, "write attachment to local file")

	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getRawData := getCmd.Bool("raw", false, "output only raw data")

	listCmd := flag.NewFlagSet("ls", flag.ExitOnError)
	listLimit := listCmd.Int("l", 0, "limit results")

	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	// fuzzy data search by default unless field is specified
	searchField := searchCmd.String("f", "data", "field to search")

	rmCmd := flag.NewFlagSet("rm", flag.ExitOnError)

	conn, err := sqlite3.Open(dbFilePath)
	if err != nil {
		log.Fatal().Err(err).Msg("error opening database")
	}
	defer conn.Close()

	// ensure database is present
	err = snip.CreateNewDatabase(conn)
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
		s.Title = *addTitle
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
		err = snip.InsertSnip(conn, s)
		if err != nil {
			log.Fatal().Err(err).Msg("error inserting Snip into database")
		}
		fmt.Printf("successfully added snip with uuid: %s\n", s.UUID)

	case "attach":
		if err := attachCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing attach arguments")
		}

		// LIST attachments with additional info
		if *attachList == true {
			list, err := snip.GetAttachmentsAll(conn)
			if err != nil {
				log.Fatal().Err(err).Msg("could not list all attachments")
			}
			// build list
			// use this function to not load overhead of Data field since it will not be used
			var attachments []snip.Attachment
			for _, id := range list {
				a, err := snip.GetAttachmentMetadata(conn, id)
				if err != nil {
					log.Fatal().Err(err).Str("uuid", id.String()).Msg("error getting attachment metadata")
				}
				attachments = append(attachments, a)
			}
			// sorting should occur here
			sort.Slice(attachments, func(i, j int) bool {
				// this is deliberate reversal to sort the largest items first
				return attachments[i].Size > attachments[j].Size
			})

			// print analysis
			fmt.Printf("%s %36s %9s %s\n", "count", "uuid", "size", "filename")
			for idx, a := range attachments {
				fmt.Printf("%5d %s %9d %s\n", idx+1, a.UUID, a.Size, truncateStr(a.Title, 60))
			}
			break
		}

		// REMOVE attachments by uuid
		if *attachRemove == true {
			for _, idStr := range attachCmd.Args() {
				id, err := uuid.Parse(idStr)
				if err != nil {
					log.Error().Err(err).Str("uuid", "idStr").Msg("error parsing uuid")
					fmt.Fprintf(os.Stderr, "could not parse attachment %s", idStr)
				}
				err = snip.DeleteAttachment(conn, id)
				if err != nil {
					log.Error().Err(err).Str("uuid", idStr).Msg("error removing attachment")
					fmt.Fprintf(os.Stderr, "could not delete attachment %s", idStr)
				} else {
					fmt.Printf("removed attachment %s\n", id)
				}
			}
			break
		}

		// WRITE attachment to file
		if *attachWrite == true {
			var outfile string

			if len(attachCmd.Args()) == 0 || len(attachCmd.Args()) > 2 {
				log.Fatal().Msg("writing attachment action requires one or two arguments")
			}

			idStr := attachCmd.Args()[0]
			id, err := uuid.Parse(idStr)
			if err != nil {
				log.Fatal().Err(err).Msg("error parsing uuid")
			}
			a, err := snip.GetAttachmentFromUUID(conn, id)
			// assign outfile name or use saved name if omitted
			if len(attachCmd.Args()) == 2 {
				outfile = attachCmd.Args()[1]
			} else {
				outfile = a.Title
			}
			bytesWritten, err := snip.WriteAttachment(conn, a.UUID, outfile)
			if err != nil {
				log.Fatal().Err(err).Msg("error writing attachment to file")
			}
			fmt.Printf("%s written to %s %d bytes\n", a.Title, outfile, bytesWritten)
			break
		}

		// check arguments
		if len(attachCmd.Args()) < 1 {
			attachCmd.Usage()
			log.Fatal().Msg("not enough arguments to attach subcommand")
		}

		// INSERT new attachments
		id := attachCmd.Args()[0]
		// validate UUID
		s, err := snip.GetFromUUID(conn, id)
		if err != nil {
			log.Fatal().Str("uuid", id).Msg("error locating snip uuid")
		}

		for _, filename := range attachCmd.Args()[1:] {
			// attempt to insert file
			data, err := os.ReadFile(filename)
			if err != nil {
				log.Fatal().Err(err).Msg("error reading attachment file data")
			}
			basename := path.Base(filename)
			// title is filename if not supplied
			err = s.Attach(conn, basename, data)
			if err != nil {
				log.Error().Err(err).Str("filename", filename).Msg("error attaching file")
				continue
			}
			fmt.Printf("attached %s %d bytes\n", filename, len(data))
		}

	case "get":
		if err := getCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing get arguments")
		}
		idStr := getCmd.Args()[0]
		if err != nil {
			log.Fatal().Err(err).Msg("error converting from bytes to uuid type")
		}
		s, err := snip.GetFromUUID(conn, idStr)
		if err != nil {
			log.Fatal().Err(err).Str("uuid", idStr).Msg("error retrieving snip with uuid")
		}

		if *getRawData {
			fmt.Printf("%s", s.Data)
		} else {
			fmt.Printf("uuid: %s\n", s.UUID.String())
			fmt.Printf("title: %s\n", s.Title)
			fmt.Printf("timestamp: %s\n", s.Timestamp.Format(time.RFC3339Nano))
			fmt.Printf("data: \n")
			fmt.Printf("----\n")
			fmt.Printf("%s", s.Data)
			fmt.Printf("\n----\n")
			// print attachments if present
			fmt.Printf("attachments:\n")
			for idx, a := range s.Attachments {
				fmt.Printf("  %d - %s %s %d bytes\n", idx, a.UUID.String(), a.Title, a.Size)
			}
		}

	case "ls":
		if err := listCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing ls arguments")
		}
		results, err := snip.List(conn, *listLimit)
		if err != nil {
			log.Fatal().Err(err).Msg("error listing items")
		}
		fmt.Printf("results: %d\n", len(results))
		for _, s := range results {
			fmt.Printf("%s %s\n", s.UUID, s.Title)
		}

	case "rm":
		if err := rmCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing rm arguments")
		}
		for idx, arg := range rmCmd.Args() {
			// parse to uuid because it seems proper
			id, err := uuid.Parse(arg)
			if err != nil {
				fmt.Printf("ERROR removing %d/%d %s...", idx+1, len(rmCmd.Args()), arg)
				log.Debug().Str("uuid", arg).Err(err).Msg("error parsing uuid input")
				continue
			}
			err = snip.Delete(conn, id)
			if err != nil {
				fmt.Printf("ERROR removing %d/%d %s...", idx+1, len(rmCmd.Args()), arg)
				log.Debug().Str("uuid", arg).Err(err).Msg("error while attempting to delete snip")
			}
			fmt.Printf("removed %d/%d %s\n", idx+1, len(rmCmd.Args()), arg)
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
			results, err = snip.SearchDataTerm(conn, term)
			if err != nil {
				log.Fatal().Err(err).Msg("error while searching for term")
			}

		case "uuid":
			results, err = snip.SearchUUID(conn, term)
			if err != nil {
				log.Fatal().Err(err).Msg("error while searching for term")
			}
		}

		fmt.Printf("results: %d\n\n", len(results))
		for _, s := range results {
			fmt.Printf("uuid: %s title: %s\n", s.UUID.String(), s.Title)
		}

	default:
		helpOutput()
	}

	log.Debug().Msg("program execution complete")
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

// truncateStr returns a new string limited to max chars
func truncateStr(text string, max int) string {
	// trade empty for empty
	if len(text) == 0 {
		return ""
	}

	cutoff := max
	suffix := false
	if len(text) > max {
		suffix = true
		cutoff = max - 3
	}
	if suffix == true {
		if len(text) <= cutoff {
			return text + "..."
		} else {
			return text[:cutoff] + "..."
		}
	}
	return text
}
