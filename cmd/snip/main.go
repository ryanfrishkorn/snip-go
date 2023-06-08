package main

import (
	"flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ryanfrishkorn/snip"
	"time"
)

type ProgramOptions struct {
	debug         *bool
	dataFromStdin *bool
	dataFromFile  *string
}

func main() {
	// parse options
	var options = ProgramOptions{}
	options.debug = flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	// configure logging
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *options.debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano

	log.Info().Msg("program execution start")

	// create simple object
	s, err := snip.New()
	if err != nil {
		log.Fatal().Msg("could not create new Snip")
	}
	log.Info().Str("UUID", s.UUID.String()).Msg("first snip object")

	log.Info().Msg("program execution complete")
}
