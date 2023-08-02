package main

import (
	"fmt"
	"go/types"
	"log"
	"os"
	"regexp"
	"strings"
	"text/template/parse"

	"golang.org/x/tools/go/packages"

	"github.com/k0kubun/pp/v3"
)

type templateBlock struct {
	name string
	root *templateVar
}

type templateVar struct {
	path             string
	children         map[string]*templateVar
	expectedRangable bool
	annotations      map[string][]string
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

	blocks := map[string]*templateBlock{}
	for _, tree := range treeMap {
		block := &templateBlock{
			name: tree.Name,
			root: &templateVar{
				path:     "",
				children: map[string]*templateVar{},
			},
		}
		blocks[tree.Name] = block
		visitNodes(block.root, tree.Root)
	}

	pp.Println(blocks)

	for name, block := range blocks {
		if err := validateTemplate(block.root, nil); err != nil {
			log.Printf("block %q: %s", name, err)
		}
	}
}

func validateTemplate(root *templateVar, typ types.Type) error {
	// ref. (*text/template.state).evalField
	if typ == nil {
		if len(root.annotations["type"]) == 0 {
			// TODO
			return fmt.Errorf("no @type annotation")
		}
		if len(root.annotations["type"]) > 1 {
			return fmt.Errorf("multiple @type annotations")
		}

		targetType := root.annotations["type"][0]
		p := strings.LastIndex(targetType, ".")
		pkgName, typeName := targetType[:p], targetType[p+1:]

		pkgs, err := packages.Load(&packages.Config{
			Mode: packages.NeedTypes | packages.NeedTypesInfo,
		}, pkgName)
		if err != nil {
			return err
		}
		if pkgs[0].Errors != nil {
			return pkgs[0].Errors[0]
		}
		obj := pkgs[0].Types.Scope().Lookup(typeName)
		if obj == nil {
			return fmt.Errorf("%s not found", targetType)
		}
		typ = obj.Type()
	} else {
		// TODO
	}

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

	for n, v := range root.children {
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
		if c, ok := n.(*parse.CommentNode); ok {
			log.Printf("comment: %s", c.Text)
			m := regexp.MustCompile(`^/\*\s*@(\w+)\s+(.*?)\s*\*/$`).FindStringSubmatch(c.Text)
			if m != nil {
				if v.annotations == nil {
					v.annotations = map[string][]string{}
				}
				v.annotations[m[1]] = append(v.annotations[m[1]], m[2])
			}
			continue
		}

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
					if _, ok := cur.children[ident]; !ok {
						cur.children[ident] = &templateVar{
							path:     cur.path + "." + ident,
							children: map[string]*templateVar{},
						}
					}
					cur = cur.children[ident]
				}
				if result == nil {
					result = cur
				}
			}
		}
	}

	return result
}
