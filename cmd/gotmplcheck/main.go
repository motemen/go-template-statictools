package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	tmplstatic "github.com/motemen/go-template-statictools"

	"go.uber.org/multierr"
)

// Usage: gotmplcheck [-json] tmpl.in
func main() {
	var flagJson bool
	flag.BoolVar(&flagJson, "json", false, "output parsed result as JSON")
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
	if err != nil {
		for _, err := range multierr.Errors(err) {
			log.Println(err)
		}
		os.Exit(1)
	}
}
