package main

import (
	"flag"
	"log"
	"os"

	"github.com/motemen/go-template-statictools/templatetypes"
)

func main() {
	var (
		flagDot     = flag.String("dot", "", "`path/to/pkg.type` of template data")
		flagVerbose = flag.Bool("verbose", false, "enable verbose logging")
		flagFuncMap = flag.String("funcmap", "", "`path/to/pkg.name` of template FuncMap")
		flagSoft    = flag.Bool("soft", false, "allow undefined functions or templates")
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

	if flagDot != nil {
		checker.DotType = *flagDot
	}
	if flagVerbose != nil {
		checker.Verbose = *flagVerbose
	}
	if flagFuncMap != nil {
		checker.FuncMapVar = *flagFuncMap
	}
	if flagSoft != nil && *flagSoft {
		checker.AllowUndefinedFuncs = true
		checker.AllowUndefinedTemplates = true
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
