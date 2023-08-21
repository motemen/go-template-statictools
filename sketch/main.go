package main

import (
	"bytes"
	"fmt"
	"log"
	"text/template"
	"text/template/parse"
)

type Barer struct{}

func (Barer) Bar(x any) string {
	return fmt.Sprintf("Bar(%v)", x)
}

type A []any

func (A) X() string { return "A.X" }

func main() {
	var buf bytes.Buffer
	err := template.Must(template.New("").Parse("{{range .Foo}}{{.X}}{{end}} / {{.Foo.X}}")).Execute(&buf, struct {
		Foo A
	}{
		Foo: []any{struct{ X string }{"X inside slice"}},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(buf.String())

	buf = bytes.Buffer{}
	err = template.Must(template.New("").Parse("{{.A.Bar .N}}")).Execute(&buf, struct {
		A Barer
		N int
	}{
		N: 999,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(buf.String())

	buf = bytes.Buffer{}
	err = template.Must(template.New("").Funcs(template.FuncMap{"f": func(a, b int) int { return a + b }}).Parse("{{call f 1 2}}")).Execute(&buf, struct {
		A Barer
		N int
	}{
		N: 999,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(buf.String())
}

func __main() {
	var buf bytes.Buffer
	err := template.Must(template.New("").Parse("{{ .Foo | .Bar }}")).Execute(&buf, struct {
		Foo int
		Barer
	}{
		Foo: 1,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(buf.String())
}

func _main() {
	log.SetFlags(log.Lshortfile)

	t := parse.Tree{
		// Mode: parse.ParseComments,
		Mode: parse.ParseComments | parse.SkipFuncCheck,
	}
	treeMap := map[string]*parse.Tree{}
	_, err := t.Parse(`
{{/* comment */}}
{{.Var}}
{{with .Sub}}{{.SubVar}}{{end}}
{{range .Slice}}{{.}}{{end}}
{{func .Var2.Var3}}
{{func (index .Var4 .Var6)}}
{{define "sub"}}{{end}}
`, "", "", treeMap)
	if err != nil {
		panic(err)
	}

	for _, n := range t.Root.Nodes {
		log.Printf("%T: %v", n, n)
		switch n := n.(type) {
		case *parse.ActionNode:
			dumpPipe(n.Pipe)

		case *parse.IfNode:
			log.Println(n.Pipe)
		case *parse.ListNode:
			log.Println(n.Nodes)
		case *parse.RangeNode:
			log.Println(n.Pipe)
		case *parse.TemplateNode:
			log.Println(n.Name)
			log.Println(n.Pipe)
		case *parse.TextNode:
			// log.Println(n.Text)
		case *parse.WithNode:
			dumpPipe(n.Pipe)

		case *parse.CommentNode:
			log.Println(n.Text)
		case *parse.DotNode:
			log.Println(n)
		}
	}
}

func dumpPipe(pipe *parse.PipeNode) {
	log.Println("PipeNode ======")
	for i, vn := range pipe.Decl {
		log.Printf("Pipe.Decl[%d]: %v", i, vn)
	}
	for i, cn := range pipe.Cmds {
		log.Printf("Pipe.Cmds[%d]: %v", i, cn)
		for i2, n2 := range cn.Args {
			log.Printf("  Pipe.Cmds[%d].Args[%d]: %#v", i, i2, n2)
		}
	}
	log.Println("===============")
}
