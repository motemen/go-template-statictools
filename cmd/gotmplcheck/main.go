package main

import (
	"flag"
	"log"
	"os"
	"text/template/parse"

	"github.com/motemen/go-template-statictools/templatetypes"
	"go.uber.org/multierr"
)

// Usage: gotmplcheck [-json] tmpl.in
func main() {
	flag.Parse()
	content, err := os.ReadFile(flag.Args()[0])
	if err != nil {
		panic(err)
	}

	t := &parse.Tree{Mode: parse.ParseComments | parse.SkipFuncCheck}
	treeMap := map[string]*parse.Tree{}
	_, err = t.Parse(string(content), "", "", treeMap)
	if err != nil {
		panic(err)
	}

	for _, tree := range treeMap {
		err = templatetypes.Check(tree, treeMap)
		if err != nil {
			for _, err := range multierr.Errors(err) {
				log.Println(err)
			}
			os.Exit(1)
		}
	}
}
