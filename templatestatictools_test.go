package templatestatictools

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"text/template"
	"text/template/parse"

	"gotest.tools/v3/assert"
)

type TestTemplateVar struct {
	Foo string
}

func TestChecker_Validate(t *testing.T) {
	parse := func(s string) (*Checker, error) {
		var checker Checker
		err := checker.Parse(strings.NewReader(s))
		return &checker, err
	}

	testParseValidate := func(s string, f func(t *testing.T, checker *Checker)) {
		checker, err := parse(s)
		assert.NilError(t, err)
		err = checker.Check()
		assert.NilError(t, err)
		t.Run(s, func(t *testing.T) {
			f(t, checker)
		})
	}

	testParseValidate("{{/* @type github.com/motemen/go-template-statictools.TestTemplateVar */}}{{.Foo}}", func(t *testing.T, checker *Checker) {
		t.Log(checker.Blocks[""])
	})
}

func TestChecker_Parse(t *testing.T) {
	parse := func(s string) (*templateRef, error) {
		var checker Checker
		err := checker.Parse(strings.NewReader(s))
		return checker.Blocks[""].Root, err
	}

	testParse := func(s string, f func(t *testing.T, root *templateRef)) {
		root, err := parse(s)
		assert.NilError(t, err)
		t.Run(s, func(t *testing.T) {
			f(t, root)
		})
	}

	dig := func(r *templateRef, path string) *templateRef {
		for _, name := range strings.Split(path, ".") {
			r = r.Children[name]
			if r == nil {
				return nil
			}
		}
		return r
	}

	refExists := func(t *testing.T, root *templateRef, path string) *templateRef {
		t.Helper()
		ref := dig(root, path)
		assert.Assert(t, ref != nil, "ref %q does not exist", path)
		return ref
	}

	testParse(`{{with .Meth .Arg}}{{.Foo}}{{end}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "Meth")
		refExists(t, root, "Arg")
		t.Skip("TODO")
		assert.Assert(t, dig(root, "Meth.Foo") == nil) // or not?
		refExists(t, root, "Meth().Foo")
	})

	testParse(`Hello`, func(t *testing.T, root *templateRef) {
	})

	testParse(`{{.Foo}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "Foo")
	})

	testParse(`{{.Foo.Bar}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "Foo")
		refExists(t, root, "Foo.Bar")
	})

	testParse(`{{with .Foo}}{{.Bar.Baz}}{{$.Qux.Quux}}{{end}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "Foo.Bar")
		refExists(t, root, "Qux.Quux")
		assert.Assert(t, dig(root, "Foo.Qux") == nil)
	})

	testParse(`{{with (index .Map .Key)}}{{$.Bar}}{{end}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "Map")
		refExists(t, root, "Key")
		refExists(t, root, "Bar")
	})

	testParse(`{{range .Foo}}{{.Bar}}{{end}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "Foo")
		// refExists(t, root, "Foo[].Bar")
	})

	testParse(`{{range (index .Map .Key)}}{{.Foo}}{{$.Bar}}{{end}}`, func(t *testing.T, root *templateRef) {
		t.Skip("TODO")
		refExists(t, root, ".Map")
		refExists(t, root, ".Key")
		refExists(t, root, ".Map[].Foo")
		refExists(t, root, ".Bar")
	})

	testParse(`{{if .Foo}}{{.Bar}}{{end}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "Foo")
		refExists(t, root, "Bar")
		assert.Assert(t, dig(root, "Foo.Bar") == nil)
	})

	testParse(`{{if eq .X .Y}}{{.Bar}}{{end}}`, func(t *testing.T, root *templateRef) {
		refExists(t, root, "X")
		refExists(t, root, "Y")
		refExists(t, root, "Bar")
	})
}

func Test_debugDumpTemplateAST(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(`{{index .Map .Key}}{{with .V1.V2 (index .Map .Key).V3 (index .Map) | .M1 .M2 | len}}{{end}}`))
	for _, n := range tmpl.Root.Nodes {
		rv := reflect.ValueOf(n).Elem()
		rf := rv.FieldByName("Pipe")
		pipe := rf.Interface().(*parse.PipeNode)
		t.Logf("%T", n)
		for i, cmd := range pipe.Cmds {
			t.Logf("cmd[%d] %q", i, cmd)
			for i, arg := range cmd.Args {
				t.Logf("  arg[%d] %T %q", i, arg, arg)
			}
		}
	}

	tmpl = template.Must(template.New("").Parse(`.Foo | .Meth = {{.Foo | .Meth}} .Foo.Meth = {{.Foo.Meth}} .Meth .Foo = {{.Meth .Foo}} .K2 | index . = {{.K2 | index .}}`))
	var buf bytes.Buffer
	m := Mether{"Foo": Mether{"Bar": "baz"}, "K": "V", "K2": "K"}
	err := tmpl.Execute(&buf, m)
	assert.NilError(t, err)
	t.Log(buf.String())
}

type Mether map[string]any

func (m Mether) Meth(x ...any) string {
	return fmt.Sprintf("(%s).Meth(%v)", m, x)
}
