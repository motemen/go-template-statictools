package main

import (
	"flag"
	"log"
	"os"

	"github.com/motemen/go-template-statictools/templatetypes"
	"go.uber.org/multierr"
)

// Usage: gotmplcheck [-json] tmpl.in
func main() {
	flag.Parse()

	log.SetFlags(0)

	var checker templatetypes.Checker
	err := checker.ParseFile(flag.Args()[0])
	if err != nil {
		panic(err)
	}

	err = checker.Check()
	if err != nil {
		for _, err := range multierr.Errors(err) {
			log.Println(checker.FormatError(err))
		}
		os.Exit(1)
	}
}
