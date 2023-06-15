package main

import (
	"flag"
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ryanfrishkorn/snip"
	"github.com/ryanfrishkorn/snip/database"
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
       -t <name>              use specified name

snip attach                   attach a file to specified snip
       add <uuid> <file ...>  add attachment files to snip
       list                   list all attachments in database
         -sort <size|name>    sort by snip field (default: name)

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
	addName := addCmd.String("t", "", "specify name")

	attachCmd := flag.NewFlagSet("attach", flag.ExitOnError)
	attachAddCmd := flag.NewFlagSet("add", flag.ExitOnError)
	attachListCmd := flag.NewFlagSet("ls", flag.ExitOnError)
	attachListSort := attachListCmd.String("sort", "name", "field to sort attachment list by")
	attachRemoveCmd := flag.NewFlagSet("rm", flag.ExitOnError)
	attachWriteCmd := flag.NewFlagSet("write", flag.ExitOnError)

	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getRawData := getCmd.Bool("raw", false, "output only raw data")
	getRandom := getCmd.Bool("random", false, "view a random snip")

	listCmd := flag.NewFlagSet("ls", flag.ExitOnError)

	renameCmd := flag.NewFlagSet("rename", flag.ExitOnError)

	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	// fuzzy data search by default unless field is specified
	searchField := searchCmd.String("f", "data", "field to search")

	rmCmd := flag.NewFlagSet("rm", flag.ExitOnError)

	// establish action
	if len(os.Args) < 2 {
		Usage()
	}
	action := os.Args[1]

	var err error
	database.Conn, err = sqlite3.Open(dbFilePath)
	if err != nil {
		log.Fatal().Err(err).Msg("error opening database")
	}
	defer database.Conn.Close()

	// ensure database is present
	err = snip.CreateNewDatabase()
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
		s.Name = *addName
		// generate name if empty
		if s.Name == "" {
			s.Name = s.GenerateName(5)
		}

		log.Debug().
			Str("UUID", s.UUID.String()).
			Str("timestamp", s.Timestamp.String()).
			Str("name", s.Name).
			Bytes("Data", s.Data).
			Msg("first snip object")
		err = snip.InsertSnip(s)
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

			// should always have at least two arguments, uuid and at least one file
			if len(attachAddCmd.Args()) < 2 {
				log.Debug().Int("length", len(attachAddCmd.Args())).Msg("argument length")
				log.Debug().Str("args", strings.Join(attachAddCmd.Args(), " ")).Msg("arguments")
				Usage()
			}
			// INSERT new attachments
			id := attachAddCmd.Args()[0]
			fmt.Println("id: ", id)
			// validate UUID
			s, err := snip.GetFromUUID(id)
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
				// name is filename if not supplied
				err = s.Attach(basename, data)
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

			list, err := snip.GetAttachmentsAll()
			if err != nil {
				log.Fatal().Err(err).Msg("could not list all attachments")
			}
			// build list
			// use this function to not load overhead of Data field since it will not be used
			var attachments []snip.Attachment
			for _, id := range list {
				a, err := snip.GetAttachmentMetadata(id)
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
			case "name":
				fallthrough
			default:
				fmt.Println("*attachListSort: ", *attachListSort)
				sort.Slice(attachments, func(i, j int) bool {
					// this is deliberate reversal to sort the largest items first
					return attachments[i].Name < attachments[j].Name
				})
			}

			// print analysis
			fmt.Printf("%s %s %42s %s\n", "count", "uuid", "size", "name")
			for idx, a := range attachments {
				fmt.Printf("%5d %s %10d %s\n", idx+1, a.UUID, a.Size, truncateStr(a.Name, 60))
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
				err = snip.DeleteAttachment(id)
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
			a, err := snip.GetAttachmentFromUUID(id)
			// assign outfile name or use saved name if omitted
			if len(attachWriteCmd.Args()) == 2 {
				outfile = attachWriteCmd.Args()[1]
			} else {
				outfile = a.Name
			}
			bytesWritten, err := snip.WriteAttachment(a.UUID, outfile)
			if err != nil {
				log.Fatal().Err(err).Msg("error writing attachment to file")
			}
			fmt.Printf("%s written to %s %d bytes\n", a.Name, outfile, bytesWritten)
		default:
			Usage()
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
			allSnips, err := snip.List(0)
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
		s, err := snip.GetFromUUID(idStr)
		if err != nil {
			log.Fatal().Err(err).Str("uuid", idStr).Msg("error retrieving snip with uuid")
		}

		if *getRawData {
			fmt.Printf("%s", s.Data)
		} else {
			fmt.Printf("uuid: %s\n", s.UUID.String())
			fmt.Printf("name: %s\n", s.Name)
			fmt.Printf("timestamp: %s\n", s.Timestamp.Format(time.RFC3339Nano))
			fmt.Printf("data: \n")
			fmt.Printf("----\n")
			fmt.Printf("%s", s.Data)
			fmt.Printf("\n----\n")
			// print attachments if present
			fmt.Printf("attachments:\n")
			for idx, a := range s.Attachments {
				fmt.Printf("  %d - %s %s %d bytes\n", idx, a.UUID.String(), a.Name, a.Size)
			}
		}

	case "ls":
		if err := listCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing ls arguments")
		}
		results, err := snip.GetAllMetadata()
		if err != nil {
			log.Fatal().Err(err).Msg("error listing items")
		}
		for idx, id := range results {
			s, err := snip.GetFromUUID(id.String())
			if err != nil {
				fmt.Fprintf(os.Stderr, "error getting snip uuid: %s\n", id.String())
				log.Fatal().Err(err).Str("uuid", s.UUID.String()).Msg("error parsing uuid")
			}
			fmt.Printf("%d %s %s\n", idx+1, s.UUID, s.Name)
		}

	case "rename":
		if err := renameCmd.Parse(os.Args[2:]); err != nil {
			log.Fatal().Err(err).Msg("error parsing rename arguments")
		}
		// require one argument
		if len(renameCmd.Args()) != 2 {
			fmt.Fprintf(os.Stderr, "rename action requires two arguments\n")
			log.Fatal().Err(err).Msg("error parsing rename arguments")
		}

		idStr := renameCmd.Args()[0]
		newName := renameCmd.Args()[1]
		// no empty strings allowed
		if newName == "" {
			fmt.Fprintf(os.Stderr, "new name must not be an empty string\n")
			log.Fatal().Err(err).Msg("no empty string allowed for renaming")
		}
		s, err := snip.GetFromUUID(idStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not retrieve snip with id: %s\n", idStr)
		}
		oldName := s.Name
		s.Name = newName
		err = s.Update()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error updating snip %s %v", idStr, err)
			log.Fatal().Err(err).Msg("could not update snip")
		}
		fmt.Printf("renamed %s %s -> %s\n", s.UUID.String(), oldName, newName)

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
			err = snip.Delete(id)
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
		fmt.Fprintf(os.Stderr, "searching data field for: \"%s\"\n", term)
		switch *searchField {
		case "data":
			results, err = snip.SearchDataTerm(term)
			if err != nil {
				log.Fatal().Err(err).Msg("error while searching for term")
			}

		case "uuid":
			results, err = snip.SearchUUID(term)
			if err != nil {
				log.Fatal().Err(err).Msg("error while searching for term")
			}
		}

		// fmt.Printf("results: %d\n\n", len(results))
		for idx, s := range results {
			fmt.Printf("%d uuid: %s name: %s\n", idx+1, s.UUID.String(), s.Name)
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
