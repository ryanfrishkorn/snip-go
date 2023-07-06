package main

import (
	"flag"
	"fmt"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"github.com/fatih/color"
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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func main() {
	// configure logging
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	zerolog.TimeFieldFormat = time.RFC3339Nano
	optionDebug := os.Getenv("DEBUG")
	if optionDebug != "" && optionDebug != "0" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// check env for explicit database path
	dbFilePath := os.Getenv("SNIP_DB")
	if dbFilePath == "" {
		homePath := os.Getenv("HOME")
		dbFilename := ".snip.sqlite3"
		if homePath == "" {
			fmt.Fprintf(os.Stderr, "please $HOME env to your home directory for database save location")
			log.Debug().Msg("could not retrieve $HOME environment variable")
			os.Exit(1)
		}
		dbFilePath = homePath + "/" + dbFilename
	}

	helpMessage :=
		`usage:
snip add                        add a new snip from standard input
       -f <file>                data from file instead of stdin default
       -n <name>                use specified name

snip attach                     attach a file to specified snip
       add <uuid> <file ...>    add attachment files to snip
       get <uuid>               display attachment metadata and info
       list                     list all attachments in database
         -sort <size|name>      sort by attachment field (default: name)
       rm <uuid ...>            remove attachment
       stdout <uuid>            write data to stdout
       write <file>             write data to file

snip get <uuid>                 retrieve snip with specified uuid
       -raw                     output only raw data from snip

snip ls                         list all snips

snip search <term ...>          return snips whose data contains given term
       -type <data|index>       specify search source (data uses a singular term only)
       -f <field>               search snip field

snip rename <uuid> <new_name>   rename snip

snip rm <uuid ...>              remove snip <uuid> ...
`
	Usage := func() {
		fmt.Fprintf(os.Stderr, "%s", helpMessage)
	}

	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addCmdFile := addCmd.String("f", "", "use data from specified file")
	addCmdName := addCmd.String("n", "", "specify name")
	addCmdUUID := addCmd.String("u", "", "specify uuid")

	attachCmd := flag.NewFlagSet("attach", flag.ExitOnError)
	attachCmdGet := flag.NewFlagSet("get", flag.ExitOnError)
	attachCmdAdd := flag.NewFlagSet("add", flag.ExitOnError)
	attachCmdList := flag.NewFlagSet("ls", flag.ExitOnError)
	attachCmdListSort := attachCmdList.String("sort", "name", "field to sort attachment list by")
	attachCmdRemove := flag.NewFlagSet("rm", flag.ExitOnError)
	attachCmdWrite := flag.NewFlagSet("write", flag.ExitOnError)
	attachCmdWriteForce := attachCmdWrite.Bool("force", false, "force local file overwrite")

	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getCmdRaw := getCmd.Bool("raw", false, "output only raw data")
	getCmdRandom := getCmd.Bool("random", false, "view a random snip")

	listCmd := flag.NewFlagSet("ls", flag.ExitOnError)
	listCmdLong := listCmd.Bool("l", false, "list full uuid instead of short")

	renameCmd := flag.NewFlagSet("rename", flag.ExitOnError)

	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	searchCmdField := searchCmd.String("f", "data", "field to search (data|uuid)")
	searchCmdLimit := searchCmd.Int("limit", 0, "limit search results")
	searchCmdLongUUID := searchCmd.Bool("l", false, "list full uuid instead of short")
	searchCmdType := searchCmd.String("type", "index", "search type (data|index)")

	rmCmd := flag.NewFlagSet("rm", flag.ExitOnError)

	// establish action
	if len(os.Args) < 2 {
		Usage()
		os.Exit(1)
	}
	action := os.Args[1]

	var err error
	database.Conn, err = sqlite3.Open(dbFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "The database could not be opened at this location: %s\n", dbFilePath)
		log.Debug().Err(err).Str("path", dbFilePath).Msg("error opening database")
		os.Exit(1)
	}
	defer database.Conn.Close()

	// ensure database is present
	err = snip.CreateNewDatabase()
	if err != nil {
		fmt.Fprintf(os.Stderr, "There was a problem creating the new database structure.\n")
		log.Debug().Err(err).Msg("error creating database schema")
		os.Exit(1)
	}

	log.Debug().Str("action", action).Msg("action invoked")
	log.Debug().Str("args", strings.Join(os.Args, " ")).Msg("action invoked")

	switch action {
	case "add":
		if err := addCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "The add arguments could not be parsed.\n")
			log.Debug().Err(err).Msg("error parsing add arguments")
			os.Exit(1)
		}

		// create simple object
		s := snip.New()

		// file input takes precedence, but default to standard input
		if *addCmdFile != "" {
			data, err := readFromFile(*addCmdFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "There was a problem reading from the file %s\n", *addCmdFile)
				log.Debug().Err(err).Str("file", *addCmdFile).Msg("error reading from file")
				os.Exit(1)
			}
			s.Data = string(data)
		} else {
			data, err := readFromStdin()
			if err != nil {
				fmt.Fprintf(os.Stderr, "The standard input could not be read.\n")
				log.Debug().Err(err).Msg("error reading from standard input")
				os.Exit(1)
			}
			s.Data = string(data)
		}
		s.Name = *addCmdName
		// generate name if empty
		if s.Name == "" {
			s.Name = s.GenerateName(5)
		}

		// modify uuid if it was specified as an argument
		if *addCmdUUID != "" {
			id, err := uuid.Parse(*addCmdUUID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "There was a problem parsing the supplied uuid %s which may be malformed.\n", *addCmdUUID)
				log.Debug().Err(err).Msg("error parsing uuid from arguments")
				os.Exit(1)
			}
			s.UUID = id
		}

		log.Debug().
			Str("UUID", s.UUID.String()).
			Str("timestamp", s.Timestamp.String()).
			Str("name", s.Name).
			Str("Data", s.Data).
			Msg("first snip object")
		err = snip.InsertSnip(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "There was a problem inserting the new snip into the database.\n")
			log.Debug().Err(err).Msg("error inserting Snip into database")
			os.Exit(1)
		}
		fmt.Printf("added snip uuid: %s\n", s.UUID)
		// index for searching
		err = s.Index()
		if err != nil {
			fmt.Fprintf(os.Stderr, "There was a problem indexing the new snip item.\n")
			log.Debug().Err(err).Str("uuid", s.UUID.String()).Msg("error indexing new snip %s")
			os.Exit(1)
		}

	case "attach":
		if err := attachCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "The attach arguments could not be parsed.\n")
			log.Debug().Err(err).Msg("error parsing attach arguments")
			attachCmd.Usage()
			os.Exit(1)
		}

		// LIST attachments with additional info
		switch attachCmd.Args()[0] {
		case "add":
			if err := attachCmdAdd.Parse(attachCmd.Args()[1:]); err != nil {
				log.Debug().Err(err).Msg("error parsing attach list arguments")
				attachCmdAdd.Usage()
				os.Exit(1)
			}

			// should always have at least two arguments, uuid and at least one file
			if len(attachCmdAdd.Args()) < 2 {
				fmt.Fprintf(os.Stderr, "The attach add command requires at least two arguments, the snip uuid and the local file to attach.\n")
				log.Debug().Int("length", len(attachCmdAdd.Args())).Str("args", strings.Join(attachCmdAdd.Args(), " ")).Msg("arguments")
				attachCmdAdd.Usage()
				os.Exit(1)
			}
			// INSERT new attachments
			id := attachCmdAdd.Args()[0]
			// validate UUID
			s, err := snip.GetFromUUID(id)
			if err != nil {
				log.Debug().Str("uuid", id).Msg("error locating snip uuid")
				os.Exit(1)
			}
			fmt.Printf("attaching files to snip %s %s\n", s.UUID.String(), s.Name)
			// TODO: Do not allow duplicate attachments by calculating checksums at this point.

			for _, filename := range attachCmdAdd.Args()[1:] {
				// attempt to insert file
				data, err := os.ReadFile(filename)
				if err != nil {
					fmt.Fprintf(os.Stderr, "The file %s could not be read.\n", filename)
					log.Debug().Err(err).Str("file", filename).Msg("error reading attachment file data")
					os.Exit(1)
				}
				basename := path.Base(filename)
				// name is filename if not supplied
				err = s.Attach(basename, data)
				if err != nil {
					fmt.Fprintf(os.Stderr, "The attach operation of the file %s had a problem.\n", filename)
					log.Debug().Err(err).Str("filename", filename).Msg("error attaching file")
					// at least attach partial
					continue
				}
				fmt.Printf("attached %s %d bytes\n", filename, len(data))
			}

		case "ls":
			if err := attachCmdList.Parse(attachCmd.Args()[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "The ls arguments could not be parsed.\n")
				log.Debug().Err(err).Msg("error parsing attach list arguments")
				os.Exit(1)
			}

			list, err := snip.GetAttachmentsAll()
			if err != nil {
				fmt.Fprintf(os.Stderr, "There was a problem while gathering the list of attachments.\n")
				log.Debug().Err(err).Msg("could not list all attachments")
				os.Exit(1)
			}
			// build list
			// use this function to not load overhead of Data field since it will not be used
			var attachments []snip.Attachment
			for _, id := range list {
				a, err := snip.GetAttachmentMetadata(id)
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem when attempting to read metadata of snip with id %s\n", id.String())
					log.Debug().Err(err).Str("uuid", id.String()).Msg("error getting attachment metadata")
					os.Exit(1)
				}
				attachments = append(attachments, a)
			}

			switch *attachCmdListSort {
			case "size":
				sort.Slice(attachments, func(i, j int) bool {
					// this is deliberate reversal to sort the largest items first
					return attachments[i].Size > attachments[j].Size
				})
			case "name":
				fallthrough
			default:
				sort.Slice(attachments, func(i, j int) bool {
					// this is deliberate reversal to sort the largest items first
					return attachments[i].Name < attachments[j].Name
				})
			}

			// print analysis
			for idx, a := range attachments {
				// do not print header if no results
				if idx == 0 {
					// print to stderr to easily pipe output
					fmt.Fprintf(os.Stderr, "%s %42s %s\n", "uuid", "size", "name")
				}
				fmt.Printf("%s %10d %s\n", a.UUID, a.Size, a.Name)
			}

		// REMOVE attachments by uuid
		case "rm":
			if err := attachCmdRemove.Parse(attachCmd.Args()[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "The arguments to the rm command could not be parsed.\n")
				log.Debug().Err(err).Msg("error parsing attach remove arguments")
				attachCmdRemove.Usage()
				os.Exit(1)
			}
			// TODO: Check this behavior, don't we need [1:] or something?
			for _, idStr := range attachCmdRemove.Args() {
				id, err := uuid.Parse(idStr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "The supplied id %s could not be validated and may be malformed.\n", idStr)
					log.Debug().Err(err).Str("uuid", "idStr").Msg("error parsing uuid")
				}
				err = snip.DeleteAttachment(id)
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem while trying to delete attachment %s", idStr)
					log.Debug().Err(err).Str("uuid", idStr).Msg("error removing attachment")
				} else {
					fmt.Printf("removed attachment %s\n", id)
				}
			}

		// STANDARD OUTPUT
		case "stdout":
			// output raw data to stdout for piping or analysis
			if err := attachCmdGet.Parse(attachCmd.Args()[1:]); err != nil {
				log.Debug().Err(err).Msg("error parsing attach list arguments")
				attachCmdGet.Usage()
				os.Exit(1)
			}

			if len(attachCmdGet.Args()) != 1 {
				Usage()
				os.Exit(1)
			}

			id, err := uuid.Parse(attachCmdGet.Arg(0))
			if err != nil {
				fmt.Fprintf(os.Stderr, "The provided id could not be parsed and may be malformed.\n")
				os.Exit(1)
			}
			a, err := snip.GetAttachmentFromUUID(id.String())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Could not locate attachment with id %s\n", id)
				log.Debug().Err(err).Str("uuid", id.String()).Msg("could not create attachment from uuid")
				os.Exit(0)
			}
			// output
			fmt.Printf("%s", a.Data)

		// WRITE attachment to file
		case "write":
			if err := attachCmdWrite.Parse(attachCmd.Args()[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "The attach write arguments could not be parsed.\n")
				log.Debug().Err(err).Msg("error parsing attach remove arguments")
				attachCmdWrite.Usage()
				os.Exit(1)
			}
			log.Debug().Str("args", strings.Join(attachCmdWrite.Args(), " ")).Msg("arguments")
			if len(attachCmdWrite.Args()) == 0 || len(attachCmdWrite.Args()) > 2 {
				fmt.Fprintf(os.Stderr, "The attach write command requires either one or two arguments.\n")
				attachCmdWrite.Usage()
				log.Debug().Msg("writing attachment action requires one or two arguments")
				os.Exit(1)
			}

			var outfile string

			idStr := attachCmdWrite.Args()[0]
			// keep this a string
			/*
				id, err := uuid.Parse(idStr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem attempting to validate the id %s which may be malformed.\n", idStr)
					log.Debug().Err(err).Msg("error parsing uuid")
					os.Exit(1)
				}
			*/
			a, err := snip.GetAttachmentFromUUID(idStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "There was a problem locating the attachment with id %s\n", idStr)
				log.Debug().Err(err).Str("id", idStr).Msg("could not get attachment")
				os.Exit(1)
			}
			// assign outfile name or use saved name if omitted
			if len(attachCmdWrite.Args()) == 2 {
				outfile = attachCmdWrite.Args()[1]
			} else {
				outfile = a.Name
			}
			var bytesWritten int
			if *attachCmdWriteForce {
				// DESTRUCTIVE TO LOCAL DATA
				// attempt to overwrite file if a local file of the same name exists
				bytesWritten, err = snip.WriteAttachment(a.UUID, outfile, true)
			} else {
				bytesWritten, err = snip.WriteAttachment(a.UUID, outfile, false)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "There was a problem while writing data for the output file %s\n", outfile)
				log.Debug().Err(err).Msg("error writing attachment to file")
				os.Exit(1)
			}
			fmt.Printf("%s written -> %s %d bytes\n", a.Name, outfile, bytesWritten)
		default:
			Usage()
			os.Exit(1)
		}

	case "get":
		if err := getCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "The get arguments could not be parsed.\n")
			log.Debug().Err(err).Msg("error parsing get arguments")
			os.Exit(1)
		}
		var idStr string

		// random from all snips
		if *getCmdRandom {
			// get list
			// TODO: verify that this does not load everything in memory everywhere immediately
			allSnips, err := snip.List(0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "There was a problem building the list of all snips in the database.\n")
				log.Debug().Err(err).Msg("error retrieving all snips")
				os.Exit(1)
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
		if len(getCmd.Args()) != 1 {
			Usage()
			os.Exit(1)
		}
		idStr = getCmd.Args()[0]

		// If this has not been set by anything above, use the command line.
		if idStr == "" {
			idStr = getCmd.Args()[0]
		}

		// There is no reason to parse this since it may be a fuzzy term. Rely on the errors.
		// TODO handle both cases explicitly and derive functions for full and partial uuid
		s, err := snip.GetFromUUID(idStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "The snip with id %s could not be retrieved.\n", idStr)
			log.Debug().Err(err).Str("uuid", idStr).Msg("error retrieving snip with uuid")
			os.Exit(1)
		}

		if *getCmdRaw {
			fmt.Printf("%s", s.Data)
		} else {
			fmt.Printf("uuid: %s\n", s.UUID.String())
			fmt.Printf("name: %s\n", s.Name)
			fmt.Printf("timestamp: %s\n", s.Timestamp.Format(time.RFC3339Nano))
			fmt.Printf("----\n")
			fmt.Printf("%s", s.Data)
			// add an extra newline if the data does not end with one
			// no one likes their prompt hijacked. This will not affect raw output.
			if !strings.HasSuffix(s.Data, "\n") {
				fmt.Println()
			}
			fmt.Printf("----\n")
			for idx, a := range s.Attachments {
				// print attachments if present
				if idx == 0 {
					fmt.Printf("attachments:\n")
					fmt.Printf("%s %42s %s\n", "uuid", "bytes", "name")
				}
				fmt.Printf("%s %10d %s\n", a.UUID.String(), a.Size, a.Name)
			}
		}

	case "ls":
		if err := listCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "The ls arguments could not be parsed.\n")
			log.Debug().Err(err).Msg("error parsing ls arguments")
			listCmd.Usage()
			os.Exit(1)
		}
		results, err := snip.GetAllSnipIDs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "There was a problem while attempting to obtain the metadata of all snips.\n")
			log.Debug().Err(err).Msg("error listing items metadata")
			os.Exit(1)
		}
		for idx, id := range results {
			s, err := snip.GetFromUUID(id.String())
			if err != nil {
				fmt.Fprintf(os.Stderr, "The snip with uuid: %s could not be obtained from the database.\n", id.String())
				log.Debug().Err(err).Str("uuid", s.UUID.String()).Msg("error obtaining snip from uuid")
				os.Exit(1)
			}
			if idx == 0 {
				if *listCmdLong {
					// long
					fmt.Fprintf(os.Stderr, "%s %36s\n", "uuid", "name")
				} else {
					// short
					fmt.Fprintf(os.Stderr, "%s %8s\n", "uuid", "name")
				}
			}
			if *listCmdLong {
				fmt.Printf("%s %s\n", s.UUID, s.Name)
			} else {
				fmt.Printf("%s %s\n", snip.ShortenUUID(s.UUID)[0], s.Name)
			}
		}

	case "rename":
		if err := renameCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "The rename arguments could not be parsed.\n")
			log.Debug().Err(err).Msg("error parsing rename arguments")
			renameCmd.Usage()
			os.Exit(1)
		}
		// require one argument
		if len(renameCmd.Args()) != 2 {
			fmt.Fprintf(os.Stderr, "The rename command requires two arguments.\n")
			log.Debug().Err(err).Msg("error parsing rename arguments")
			os.Exit(1)
		}

		idStr := renameCmd.Args()[0]
		newName := renameCmd.Args()[1]
		// no empty strings allowed
		if newName == "" {
			fmt.Fprintf(os.Stderr, "The new name cannot be an empty string.\n")
			log.Debug().Err(err).Msg("no empty string allowed for renaming")
			os.Exit(1)
		}
		s, err := snip.GetFromUUID(idStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not retrieve snip with id: %s\n", idStr)
			log.Debug().Err(err).Str("uuid", idStr).Msg("retrieving snip from uuid")
			os.Exit(1)
		}
		oldName := s.Name
		s.Name = newName
		err = s.Update()
		if err != nil {
			fmt.Fprintf(os.Stderr, "There was a problem updating snip with id %s\n", idStr)
			log.Debug().Err(err).Msg("could not update snip")
			os.Exit(1)
		}
		fmt.Printf("renamed %s %s -> %s\n", s.UUID.String(), oldName, newName)

	case "rm":
		if err := rmCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "The rm arguments could not be parsed.\n")
			log.Debug().Err(err).Msg("error parsing rm arguments")
			rmCmd.Usage()
			os.Exit(1)
		}
		for idx, arg := range rmCmd.Args() {
			// parse to uuid because it seems proper
			id, err := uuid.Parse(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Could not parse the id of %d/%d %s\n", idx+1, len(rmCmd.Args()), arg)
				log.Debug().Str("uuid", arg).Err(err).Msg("error parsing uuid input")
				// Do not exit as others may be valid.
				continue
			}
			err = snip.Delete(id)
			if err != nil {
				fmt.Printf("Could not remove %d/%d %s\n", idx+1, len(rmCmd.Args()), arg)
				log.Debug().Str("uuid", arg).Err(err).Msg("error while attempting to delete snip")
			} else {
				// must else because we don't break
				fmt.Printf("removed %d/%d %s\n", idx+1, len(rmCmd.Args()), arg)
			}
		}

	case "search":
		if err := searchCmd.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "The search arguments could not be parsed.\n")
			log.Debug().Err(err).Str("args", strings.Join(searchCmd.Args(), " ")).Msg("error parsing search arguments")
			searchCmd.Usage()
			os.Exit(1)
		}
		if len(searchCmd.Args()) < 1 {
			fmt.Fprintf(os.Stderr, "Must supply at least one search term.\n")
			searchCmd.Usage()
			os.Exit(1)
		}

		var snipResults []snip.Snip

		switch *searchCmdType {
		case "index":
			terms := searchCmd.Args()

			searchResults, err := snip.SearchIndexTerm(terms, true)
			if err != nil {
				fmt.Fprintf(os.Stderr, "There was a problem searching the index for term %s\n", terms)
				log.Debug().Err(err).Msg("error while searching for term")
				os.Exit(1)
			}

			var scores []snip.SearchScore
			for key, result := range searchResults {
				score, err := snip.ScoreCounts(key, terms, result)
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem scoring the item with id %s\n", key)
					log.Debug().Err(err).Str("uuid", key.String()).Msg("scoring the results")
					os.Exit(1)
				}
				// add to sortable slice
				scores = append(scores, snip.SearchScore{UUID: key, Score: score, SearchCounts: result})
			}

			// sorted output by highest score
			sort.Slice(scores, func(i int, j int) bool {
				return scores[i].Score > scores[j].Score
			})

			// enforce limit after sort
			if *searchCmdLimit != 0 && len(scores) > *searchCmdLimit {
				scores = scores[:*searchCmdLimit]
			}
			for _, score := range scores {
				// get full snip to display name
				s, err := snip.GetFromUUID(score.UUID.String())
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem getting the snip to display its name.\n")
					log.Debug().Err(err).Msg("building snip to display name")
					os.Exit(1)
				}
				fmt.Printf("%s\n", s.Name)
				if *searchCmdLongUUID {
					fmt.Printf("  %s ", s.UUID)
				} else {
					fmt.Printf("  %s ", snip.ShortenUUID(s.UUID)[0])
				}
				fmt.Printf("(score: %f, ", score.Score)
				fmt.Printf("words: %d)", s.CountWords())

				// display terms found in document
				for idx, stat := range score.SearchCounts {
					if idx == 0 {
						fmt.Printf(" [")
					} else {
						fmt.Printf(", ")
					}
					fmt.Printf("%s: %d", stat.Stem, stat.Count)
					if idx == len(score.SearchCounts)-1 {
						fmt.Printf("]")
						fmt.Printf("\n")
					}
				}

				// show context
				s, err = snip.GetFromUUID(score.UUID.String())
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem showing search context for item %s\n", score.UUID)
					log.Debug().Err(err).Msg("building snip to obtain search context")
					os.Exit(1)
				}
				for _, term := range terms {
					ctxAll, err := s.GatherContext(term, 6)
					if err != nil {
						fmt.Fprintf(os.Stderr, "There was a problem gathering context for term %s: %v\n", term, err)
						log.Debug().Str("term", term).Str("uuid", score.UUID.String()).Msg("gathering context")
						log.Debug().Err(err).Msg("gathering context")
						os.Exit(1)
					}
					if len(ctxAll) == 0 {
						// in this case, there are no results (which is technically not an error)
						// TODO: perhaps only matching terms should be iterated over instead of supplied terms
						continue
					}

					// log.Debug().Any("ctx", ctxAll).Msg("term context")

					// print each context
					for _, ctx := range ctxAll {
						// these will be printed if not empty
						var before string
						var after string

						// print indexes for begin and end of context (to give more context)
						fmt.Printf("    [%d-%d] ", ctx.BeforeStart, ctx.AfterEnd)
						before = strings.Join(ctx.Before, " ")
						after = strings.Join(ctx.After, " ")
						// log.Debug().Int("ctx.Before", len(ctx.After)).Msg("join before length")
						// log.Debug().Int("ctx.After", len(ctx.After)).Msg("join after length")

						// if we don't check for empty line, it will produce padding
						fmt.Printf(`"`) // quotes separate from before string output
						if before != "" {
							fmt.Printf("%s ", before)
						}
						c := color.New(color.FgRed)
						_, err = c.Printf("%s", ctx.Term)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Color output could not be displayed.\n")
							log.Debug().Err(err).Msg("color print of context term")
							os.Exit(1)
						}
						if after != "" {
							fmt.Printf(" %s", after)
						}
						fmt.Printf(`"`) // quotes separate from after string output
						fmt.Printf("\n")
					}
				}
				fmt.Printf("\n")
			}

			if len(searchResults) <= 0 {
				fmt.Fprintf(os.Stderr, "No results for term \"%s\"\n", terms)
				os.Exit(0)
			}

		case "data":
			term := searchCmd.Args()[0]

			fmt.Fprintf(os.Stderr, "Search type %s on field %s for: \"%s\"\n", *searchCmdType, *searchCmdField, term)
			log.Debug().Str("field", *searchCmdField)

			switch *searchCmdField {
			case "data":
				snipResults, err = snip.SearchDataTerm(term)
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem searching %s field for term %s\n", *searchCmdField, term)
					log.Debug().Err(err).Msg("error while searching for term")
					os.Exit(1)
				}

			case "uuid":
				snipResults, err = snip.SearchUUID(term)
				if err != nil {
					fmt.Fprintf(os.Stderr, "There was a problem searching %s field for term %s\n", *searchCmdField, term)
					log.Debug().Err(err).Msg("error while searching for term")
					os.Exit(1)
				}
			}

			if len(snipResults) <= 0 {
				fmt.Fprintf(os.Stderr, "No results for term \"%s\"\n", term)
				os.Exit(0)
			}
			fmt.Fprintf(os.Stderr, "%s %36s\n", "uuid", "name")
			for _, s := range snipResults {
				fmt.Printf("%s %s\n", s.UUID.String(), s.Name)
			}
		}

	case "index":
		// rebuild index
		fmt.Fprintf(os.Stderr, "dropping index...")
		err := snip.DropIndex()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error")
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "success\n")

		fmt.Fprintf(os.Stderr, "indexing...")

		ids, err := snip.GetAllSnipIDs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error")
			os.Exit(1)
		}
		numLength := 0
		for idx, id := range ids {
			// assign for next time
			numLength = len(strconv.Itoa(idx+1)) + 1 + len(strconv.Itoa(len(ids)))
			progressStr := fmt.Sprintf("%d/%d", idx+1, len(ids))
			fmt.Fprintf(os.Stderr, progressStr)
			s, err := snip.GetFromUUID(id.String())
			if err != nil {
				fmt.Fprintf(os.Stderr, "error")
				os.Exit(1)
			}
			log.Debug().Str("uuid", s.UUID.String()).Msg("indexing snip")
			err = s.Index()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error indexing item %s\n", s.UUID)
				os.Exit(1)
			}
			for i := 0; i < numLength; i++ {
				fmt.Fprintf(os.Stderr, "\b \b")
			}
		}
		fmt.Fprintf(os.Stderr, "success\n")

	default:
		Usage()
		os.Exit(1)
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
func truncateStr(text string, max int, suffix string) string {
	// trade empty for empty
	if text == "" {
		return ""
	}

	cutoff := max
	truncate := false
	// use runes
	if utf8.RuneCountInString(text) > max {
		truncate = true
		cutoff = max - len(suffix)
	}
	if truncate {
		if len(text) <= cutoff {
			return text + suffix
		} else {
			return text[:cutoff] + suffix
		}
	}
	return text
}
