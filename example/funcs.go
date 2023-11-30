package example

import "text/template"

var funcs = template.FuncMap{
	"pi": func() float64 { return 3.14 },
}
