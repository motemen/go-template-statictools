package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"go.uber.org/multierr"

	tmplstatic "github.com/motemen/go-template-statictools"
)

// Usage: gotmplcheck [-json] tmpl.in
func main() {
	var flagJson bool
	var flagJsonTyped bool
	flag.BoolVar(&flagJson, "json", false, "output parsed result as JSON")
	flag.BoolVar(&flagJsonTyped, "json-typed", false, "output parsed result as JSON")
	flag.Parse()

	var c tmplstatic.Checker
	err := c.ParseFile(flag.Args()[0])
	if err != nil {
		log.Fatal(err)
	}

	if flagJson {
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(b))
		return
	}

	err = c.Check()

	if flagJsonTyped {
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(b))
	}

	if err != nil {
		for _, err := range multierr.Errors(err) {
			log.Println(err)
		}
		os.Exit(1)
	}
}
