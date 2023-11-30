package templatetypes

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"gotest.tools/v3/assert"
)

type Dot1 struct {
	Foo   string
	Inner Dot1Inner
	Slice []Dot1ContainedValue
	Map   map[string]Dot1ContainedValue
	Func1 func(n int, s string) FuncResult
	Intf  Dot1InnerInterface
	Dot1Embedded
}

func (Dot1) Method() string {
	return "method"
}

type Dot1Inner struct {
	InnerField int
}

type Dot1InnerInterface interface {
	InnerMethod() Dot1Inner
}

type Dot1InnerImpl string

func (Dot1InnerImpl) InnerMethod() Dot1Inner {
	return Dot1Inner{InnerField: 999}
}

type Dot1ContainedValue struct {
	Value bool
}

type Dot1Embedded struct {
	EmbeddedInner Dot1EmbeddedInner
}

type Dot1EmbeddedInner struct {
	EmbeddedInnerField string
}

type FuncResult struct {
	ResultField string
}

func TestCheck(t *testing.T) {
	type testCase struct {
		name         string
		template     string
		errorMessage string
	}

	tests := []testCase{
		{
			"no type specification",
			`{{.Foo}}{{.Inner.InnerField}}`,
			"",
		},
		{
			"typecheck passes", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Foo}}`,
			"",
		},
		{
			"nonexistent field", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Foo}}{{.Bar}}`,
			"can't evaluate field Bar in type github.com/motemen/go-template-statictools/templatetypes.Dot1",
		},
		{
			"nonexistent type", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.InvalidType */}}
{{.Foo}}{{.Bar}}`,
			"@type github.com/motemen/go-template-statictools/templatetypes.InvalidType: cannot load type github.com/motemen/go-template-statictools/templatetypes.InvalidType",
		},
		{
			"nested field", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Inner.InnerField}}`,
			"",
		},
		{
			"nested field, no existent", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Inner.InnerField.InnerInnerField}}`,
			"can't evaluate field InnerInnerField in type int",
		},
		{
			"with", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{with .Inner}}{{.InnerField}}{{end}}`,
			"",
		},
		{
			"with, invalid", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{with .Inner}}{{.Invalid}}{{end}}`,
			"can't evaluate field Invalid in type github.com/motemen/go-template-statictools/templatetypes.Dot1Inner",
		},
		{
			"with, type annotation inside", `
{{with .Inner}}
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1Inner */}}
{{.InnerField}}
{{end}}`,
			"",
		},
		{
			"with, type annotation inside", `
{{with .Inner}}
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1Inner */}}
{{.NonExistent}}
{{end}}`,
			"can't evaluate field NonExistent in type github.com/motemen/go-template-statictools/templatetypes.Dot1Inner",
		},
		{
			"range", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{range .Slice}}{{.Value}}{{end}}`,
			"",
		},
		{
			"range error", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{range .Foo}}{{.Value}}{{end}}`,
			"range can't iterate over string, pipe: .Foo",
		},
		{
			"builtin function index", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{index .Map "foo"}}`,
			"",
		},
		{
			"builtin function index", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{(index .Map "foo").Value}}`,
			"",
		},
		{
			"builtin function index", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{(index .Map "foo").InvalidKey}}`,
			"can't evaluate field InvalidKey in type github.com/motemen/go-template-statictools/templatetypes.Dot1ContainedValue",
		},
		{
			"with, type annotation inside", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{with .Inner}}
  {{.InnerField}}
  {{$.Inner.InnerField}}
{{end}}`,
			"",
		},
		{
			"with, type annotation inside", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{with .Inner}}
  {{.InnerField}}
  {{$.InvalidKey}}
{{end}}`,
			"can't evaluate field InvalidKey in type github.com/motemen/go-template-statictools/templatetypes.Dot1",
		},
		{
			"template", `
{{define "subtemplate"}}{{.InnerField}}{{end}}
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{template "subtemplate" .Inner}}`,
			"",
		},
		{
			"template error", `
{{define "subtemplate"}}{{.InvalidKey}}{{end}}
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{template "subtemplate" .Inner}}`,
			"can't evaluate field InvalidKey in type github.com/motemen/go-template-statictools/templatetypes.Dot1Inner",
		},
		{
			"with else", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{with .Inner}}
  {{.InnerField}}
{{else}}
  {{.InnerField}}
{{end}}`,
			"can't evaluate field InnerField in type github.com/motemen/go-template-statictools/templatetypes.Dot1",
		},
		{
			"invalid arg of user-defined function", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{customFunc .InvalidKey}}
`,
			"can't evaluate field InvalidKey in type github.com/motemen/go-template-statictools/templatetypes.Dot1",
		},
		{
			"builtin len", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{len "foobar"}}
{{len .Slice}}
{{len .Map}}
{{len .Inner}}
`,
			"function len: invalid argument type github.com/motemen/go-template-statictools/templatetypes.Dot1Inner",
		},
		{
			"map", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Map.foo.InvalidField}}`,
			"can't evaluate field InvalidField in type github.com/motemen/go-template-statictools/templatetypes.Dot1ContainedValue",
		},
		{
			"range on map", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{range .Map}}{{.Value}}{{/* @debug show . */}}{{end}}`,
			"",
		},
		{
			"range on map", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{range .Map}}{{.InvalidField}}{{end}}`,
			"can't evaluate field InvalidField in type github.com/motemen/go-template-statictools/templatetypes.Dot1ContainedValue",
		},
		{
			"method", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Method}}`, "",
		},
		{
			"method", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Intf.InnerMethod.InnerField}}`, "",
		},
		{
			"embedded", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.EmbeddedInner.EmbeddedInnerField}}`, "",
		},
		{
			"range (1-var, no assign)", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{range $i, $item := .Slice}}
{{$i}}: {{$item.Value}}
{{end}}`, "",
		},
		{
			"range (2-var, no assign)", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{range $item := .Slice}}
{{$item.Value}}
{{end}}`, "",
		},
		{
			"range (1-var, assign)", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{ $i := "" }}
{{ $item := "" }}
{{range $i, $item = .Slice}}
{{$i}}: {{$item.Value}}
{{end}}`, "",
		},
		{
			"range (2-var, assign)", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{ $item := "" }}
{{range $item = .Slice}}
{{$item.Value}}
{{end}}`, "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var s Checker
			err := s.Parse("", strings.NewReader(test.template))
			assert.NilError(t, err)

			err = s.Check("")
			if test.errorMessage == "" {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, test.errorMessage)
			}
		})
	}

	for _, test := range tests {
		t.Run("sanity check - "+test.name, func(t *testing.T) {
			tmpl := template.Must(template.New(test.name).Funcs(template.FuncMap{
				"customFunc": func() string { return "customFunc" },
			}).Parse(test.template))
			dot1 := Dot1{
				Foo: "foo",
				Inner: Dot1Inner{
					InnerField: 42,
				},
				Slice: []Dot1ContainedValue{
					{Value: true},
					{Value: false},
				},
				Map: map[string]Dot1ContainedValue{
					"foo": {Value: true},
					"bar": {Value: false},
				},
				Func1: func(n int, s string) FuncResult {
					return FuncResult{
						ResultField: s + strings.Repeat("!", n),
					}
				},
				Intf: Dot1InnerImpl("inner"),
			}
			var buf bytes.Buffer
			err := tmpl.Execute(&buf, dot1)
			if test.errorMessage == "" {
				assert.NilError(t, err)
				t.Logf("%q -> %q", test.template, buf.String())
			} else {
				// do not assert here, since even if the typecheck fails,
				// the execution may succeed
			}
		})
	}
}
