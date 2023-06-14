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
	"math/rand"
	"os"
	"path"
	"sort"
	"strings"
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

	helpMessage :=
		`usage:
snip add                      add a new snip from standard input
       -f <file>              data from file instead of stdin default
       -t <title>             use specified title

snip attach                   attach a file to specified snip
       add <uuid> <file ...>  add attachment files to snip
       list                   list all attachments in database
         -sort <size|title>   sort by snip field (default: title)

snip get <uuid>               retrieve snip with specified uuid
       -raw                   output only raw data from snip

snip ls                       list all snips

snip search <term>            return snips whose data contains given term
       -f <field>             search snip field

snip rm <uuid ...>            remove snip <uuid> ...
`
	Usage := func() {
		fmt.Fprintf(os.Stderr, "%s", helpMessage)
		os.Exit(1)
	}

	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addDataFromFile := addCmd.String("f", "", "use data from specified file")
	addTitle := addCmd.String("t", "", "specify title")

	attachCmd := flag.NewFlagSet("attach", flag.ExitOnError)
	attachAddCmd := flag.NewFlagSet("add", flag.ExitOnError)
	attachListCmd := flag.NewFlagSet("ls", flag.ExitOnError)
	attachListSort := attachListCmd.String("sort", "title", "field to sort attachment list by")
	attachRemoveCmd := flag.NewFlagSet("rm", flag.ExitOnError)
	attachWriteCmd := flag.NewFlagSet("write", flag.ExitOnError)

	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getRawData := getCmd.Bool("raw", false, "output only raw data")
	getRandom := getCmd.Bool("random", false, "view a random snip")

	listCmd := flag.NewFlagSet("ls", flag.ExitOnError)

	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	// fuzzy data search by default unless field is specified
	searchField := searchCmd.String("f", "data", "field to search")

	rmCmd := flag.NewFlagSet("rm", flag.ExitOnError)

	// establish action
	if len(os.Args) < 2 {
		Usage()
	}
	action := os.Args[1]

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
		switch attachCmd.Args()[0] {
		case "add":
			// attachAddCmd
			if err := attachAddCmd.Parse(attachCmd.Args()[1:]); err != nil {
				log.Fatal().Err(err).Msg("error parsing attach list arguments")
			}

			// should always have two arguments, uuid and at least one file
			if len(attachAddCmd.Args()) != 2 {
				log.Debug().Int("length", len(attachAddCmd.Args())).Msg("argument length")
				log.Debug().Str("args", strings.Join(attachAddCmd.Args(), " ")).Msg("arguments")
				attachAddCmd.Usage()
				os.Exit(1)
			}
			// INSERT new attachments
			id := attachAddCmd.Args()[0]
			fmt.Println("id: ", id)
			// validate UUID
			s, err := snip.GetFromUUID(conn, id)
			if err != nil {
				log.Fatal().Str("uuid", id).Msg("error locating snip uuid")
			}

			for _, filename := range attachAddCmd.Args()[1:] {
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
		case "ls":
			if err := attachListCmd.Parse(attachCmd.Args()[1:]); err != nil {
				log.Fatal().Err(err).Msg("error parsing attach list arguments")
			}

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

			switch *attachListSort {
			case "size":
				sort.Slice(attachments, func(i, j int) bool {
					// this is deliberate reversal to sort the largest items first
					return attachments[i].Size > attachments[j].Size
				})
			default:
				fmt.Println("*attachListSort: ", *attachListSort)
				sort.Slice(attachments, func(i, j int) bool {
					// this is deliberate reversal to sort the largest items first
					return attachments[i].Title < attachments[j].Title
				})
			}

			// print analysis
			fmt.Printf("%s %s %42s %s\n", "count", "uuid", "size", "title")
			for idx, a := range attachments {
				fmt.Printf("%5d %s %10d %s\n", idx+1, a.UUID, a.Size, truncateStr(a.Title, 60))
			}

		// REMOVE attachments by uuid
		case "rm":
			if err := attachRemoveCmd.Parse(attachCmd.Args()); err != nil {
				log.Fatal().Err(err).Msg("error parsing attach remove arguments")
			}
			for _, idStr := range attachRemoveCmd.Args() {
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

		// WRITE attachment to file
		case "write":
			if err := attachWriteCmd.Parse(attachCmd.Args()[1:]); err != nil {
				log.Fatal().Err(err).Msg("error parsing attach remove arguments")
			}
			log.Debug().Str("args", strings.Join(attachWriteCmd.Args(), " ")).Msg("arguments")
			if len(attachWriteCmd.Args()) == 0 || len(attachWriteCmd.Args()) > 2 {
				attachWriteCmd.Usage()
				log.Fatal().Msg("writing attachment action requires one or two arguments")
			}

			var outfile string

			idStr := attachWriteCmd.Args()[0]
			id, err := uuid.Parse(idStr)
			if err != nil {
				log.Fatal().Err(err).Msg("error parsing uuid")
			}
			a, err := snip.GetAttachmentFromUUID(conn, id)
			// assign outfile name or use saved name if omitted
			if len(attachWriteCmd.Args()) == 2 {
				outfile = attachWriteCmd.Args()[1]
			} else {
				outfile = a.Title
			}
			bytesWritten, err := snip.WriteAttachment(conn, a.UUID, outfile)
			if err != nil {
				log.Fatal().Err(err).Msg("error writing attachment to file")
			}
			fmt.Printf("%s written to %s %d bytes\n", a.Title, outfile, bytesWritten)

		}

	case "get":
		if err := getCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing get arguments")
		}
		var idStr string

		// random from all snips
		if *getRandom == true {
			fmt.Fprintf(os.Stderr, "getting random snip\n")
			// get list
			allSnips, err := snip.List(conn, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error listing all snips: %v", err)
				log.Fatal().Err(err).Msg("error retrieving all snips")
			}

			// get random within range
			src := rand.NewSource(time.Now().UnixNano())
			r := rand.New(src)
			index := r.Intn(len(allSnips))
			log.Debug().Int("random index", index).Msg("generated random integer")
			// assign to outside world
			idStr = allSnips[index].UUID.String()
		}

		// obtain uuid specified from argument
		if idStr == "" {
			if len(getCmd.Args()) < 1 {
				getCmd.Usage()
				os.Exit(1)
			}
			idStr = getCmd.Args()[0]
		}
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
		results, err := snip.GetAllMetadata(conn)
		if err != nil {
			log.Fatal().Err(err).Msg("error listing items")
		}
		fmt.Printf("results: %d\n", len(results))
		for _, id := range results {
			s, err := snip.GetFromUUID(conn, id.String())
			if err != nil {
				fmt.Fprintf(os.Stderr, "error getting snip uuid: %s\n", id.String())
				log.Fatal().Err(err).Str("uuid", s.UUID.String()).Msg("error parsing uuid")
			}
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
		Usage()
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
