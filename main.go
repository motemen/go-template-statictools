package main

import (
	"fmt"
	"go/types"
	"log"
	"os"
	"strings"
	"text/template/parse"

	"golang.org/x/tools/go/packages"

	"github.com/k0kubun/pp/v3"
)

type templateVar struct {
	path             string
	fields           map[string]*templateVar
	expectedRangable bool
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

	targetType := os.Args[2]
	p := strings.LastIndex(targetType, ".")
	pkgName, typeName := targetType[:p], targetType[p+1:]

	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo,
	}, pkgName)
	if err != nil {
		log.Fatal(err)
	}

	obj := pkgs[0].Types.Scope().Lookup(typeName)

	err = validateTemplate(blocks[""], obj.Type())
	if err != nil {
		log.Fatal(err)
	}
}

func validateTemplate(root *templateVar, typ types.Type) error {
	// ref. (*text/template.state).evalField
	origType := typ
	for {
		if t, ok := typ.(*types.Named); ok {
			typ = t.Underlying()
		} else if t, ok := typ.(*types.Pointer); ok {
			typ = t.Elem()
		} else {
			break
		}
	}

	for n, v := range root.fields {
		switch t := typ.(type) {
		case *types.Struct:
			var found bool
			for i := 0; i < t.NumFields(); i++ {
				f := t.Field(i)
				if f.Name() == n {
					found = true
					ft := f.Type()
					if v.expectedRangable {
						if t, ok := ft.(*types.Slice); ok {
							ft = t.Elem()
						} else {
							return fmt.Errorf("%s: expected slice, but got %s", v.path, ft)
						}
					}
					if err := validateTemplate(v, ft); err != nil {
						return err
					}
					break
				}
			}
			if !found {
				return fmt.Errorf("%s not found in %s", v.path, origType)
			}

		default:
			return fmt.Errorf("%s: unexpected type: %s (%T)", v.path, typ, typ)
		}
	}

	return nil
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
		if _, ok := n.(*parse.RangeNode); ok {
			newV.expectedRangable = true
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
			}
		}
	}

	return result
}
