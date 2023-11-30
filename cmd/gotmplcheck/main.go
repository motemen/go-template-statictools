package main

import (
	"flag"
	"log"
	"os"

	"github.com/motemen/go-template-statictools/templatetypes"
)

func main() {
	var (
		flagDot     = flag.String("dot", "", "`path/to.type` of template data")
		flagVerbose = flag.Bool("verbose", false, "enable verbose logging")
	)

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

	if flagDot != nil && *flagDot != "" {
		checker.DotType = *flagDot
	}
	if flagVerbose != nil {
		checker.Verbose = *flagVerbose
	}

	err := checker.Check(args[0])
	if err != nil {
		if u, ok := err.(interface{ Unwrap() []error }); ok {
			for _, err := range u.Unwrap() {
				log.Println(checker.FormatError(err))
			}
		} else {
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
