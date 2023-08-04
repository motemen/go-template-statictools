package templatestatictools

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

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
			r = r.Refs[name]
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
