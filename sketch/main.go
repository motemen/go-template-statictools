package main

import (
	"log"
	"text/template/parse"
)

func main() {
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
