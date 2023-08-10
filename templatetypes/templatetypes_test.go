package templatetypes

import (
	"testing"
	"text/template/parse"

	"gotest.tools/v3/assert"
)

type Dot1 struct {
	Foo     string
	Nested1 struct {
		Nested2 bool
	}
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
{{.Nested1.Nested2}}`,
			"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree := &parse.Tree{
				Mode: parse.ParseComments | parse.SkipFuncCheck,
			}

			_, err := tree.Parse(test.template, "", "", map[string]*parse.Tree{})
			assert.NilError(t, err)

			err = Check(tree)
			if test.errorMessage == "" {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, test.errorMessage)
			}
		})
	}
}
