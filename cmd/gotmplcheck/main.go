package main

import (
	"flag"
	"log"
	"os"

	"github.com/motemen/go-template-statictools/templatetypes"
	"go.uber.org/multierr"
)

func main() {
	flag.Parse()

	log.SetFlags(0)

	args := flag.Args()
	if len(args) == 0 {
		usageAndExit()
	}

	var checker templatetypes.Checker
	for _, arg := range args {
		err := checker.ParseFile(arg)
		if err != nil {
			panic(err)
		}
	}

	err := checker.Check(args[0])
	if err != nil {
		for _, err := range multierr.Errors(err) {
			log.Println(checker.FormatError(err))
		}
		os.Exit(1)
	}
}

func usageAndExit() {
	log.Printf("Usage: %s <file> ...", os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}
