package templatetypes

import (
	"testing"
	"text/template/parse"

	"gotest.tools/v3/assert"
)

type Dot1 struct {
	Foo   string
	Inner Dot1Inner
	Slice []Dot1ContainedValue
	Map   map[string]Dot1ContainedValue
}

type Dot1Inner struct {
	Inner      bool
	InnerField int
}

type Dot1ContainedValue struct {
	Value bool
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
			`{{.Foo}}{{.Bar}}`,
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
			"cannot load type github.com/motemen/go-template-statictools/templatetypes.InvalidType",
		},
		{
			"nested field", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Inner.Inner}}`,
			"",
		},
		{
			"nested field, no existent", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{.Inner.Inner.Inner}}`,
			"can't evaluate field Inner in type bool",
		},
		{
			"with", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{with .Inner}}{{.Inner}}{{end}}`,
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
			"range can't iterate over github.com/motemen/go-template-statictools/templatetypes.Dot1",
		},
		{
			"index", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{index .Map "foo"}}`,
			"",
		},
		{
			"index", `
{{/* @type github.com/motemen/go-template-statictools/templatetypes.Dot1 */}}
{{(index .Map "foo").Value}}`,
			"",
		},
		{
			"index", `
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree := &parse.Tree{
				Mode: parse.ParseComments | parse.SkipFuncCheck,
			}

			treeMap := map[string]*parse.Tree{}
			_, err := tree.Parse(test.template, "", "", treeMap)
			assert.NilError(t, err)

			err = Check(tree, treeMap)
			if test.errorMessage == "" {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, test.errorMessage)
			}
		})
	}
}
