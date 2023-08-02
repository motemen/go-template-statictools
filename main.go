package main

import (
	"log"
	"os"
	"text/template/parse"

	"github.com/k0kubun/pp/v3"
)

type templateVar struct {
	path   string
	fields map[string]*templateVar
	// expectedType
}

func main() {
	log.SetFlags(log.Lshortfile)
	content, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	t := parse.Tree{
		Mode: parse.ParseComments | parse.SkipFuncCheck,
	}

	treeMap := map[string]*parse.Tree{}

	_, err = t.Parse(string(content), "", "", treeMap)
	if err != nil {
		panic(err)
	}

	blocks := map[string]*templateVar{}
	for _, tree := range treeMap {
		vars := &templateVar{
			path:   "",
			fields: map[string]*templateVar{},
		}

		visitNodes(vars, tree.Root)
		blocks[tree.Name] = vars
	}

	pp.Println(blocks)
}

func visitNodes(v *templateVar, root *parse.ListNode) {
	if root == nil {
		return
	}

	for _, n := range root.Nodes {
		var pipe *parse.PipeNode
		switch n := n.(type) {
		case *parse.ActionNode:
			pipe = n.Pipe
		case *parse.IfNode:
			pipe = n.Pipe
		case *parse.RangeNode:
			pipe = n.Pipe
		case *parse.WithNode:
			pipe = n.Pipe
		case *parse.TemplateNode:
			pipe = n.Pipe
		}
		if pipe == nil {
			continue
		}

		newV := visitPipe(v, pipe)
		if newV == nil {
			newV = v
		}

		switch n := n.(type) {
		case *parse.IfNode:
			visitNodes(v, n.List)
			visitNodes(v, n.ElseList)
		case *parse.RangeNode:
			visitNodes(newV, n.List)
		case *parse.WithNode:
			visitNodes(newV, n.List)
		}
	}
}

func visitPipe(v *templateVar, pipe *parse.PipeNode) *templateVar {
	log.Println("# PipeNode ======")

	var result *templateVar
	for _, cmd := range pipe.Cmds {
		for _, arg := range cmd.Args {
			if fn, ok := arg.(*parse.FieldNode); ok {
				cur := v
				for _, ident := range fn.Ident {
					if _, ok := cur.fields[ident]; !ok {
						cur.fields[ident] = &templateVar{
							path:   cur.path + "." + ident,
							fields: map[string]*templateVar{},
						}
					}
					cur = cur.fields[ident]
				}
				if result == nil {
					result = cur
				}

				log.Printf("  idents: %#v", fn.Ident)
			}
		}
	}
	log.Println("=================")

	return result
}
