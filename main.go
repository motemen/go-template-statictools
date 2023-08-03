package main

import (
	"bytes"
	"fmt"
	"go/types"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"text/template/parse"

	"go.uber.org/multierr"
	"golang.org/x/tools/go/packages"
)

type templateBlock struct {
	name string
	root *templateVar
}

type templateVar struct {
	path             string
	pos              parse.Pos
	children         map[string]*templateVar
	expectedRangable bool
	annotations      map[string][]string
}

type Checker struct {
	filename string
	content  []byte
	blocks   map[string]*templateBlock
	treeMap  map[string]*parse.Tree
}

func init() {
	log.SetFlags(0)
}

func (c *Checker) ParseFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	c.filename = filename
	return c.Parse(f)
}

func (c *Checker) Parse(r io.Reader) error {
	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	c.content = content
	c.treeMap = map[string]*parse.Tree{}

	t := parse.Tree{Mode: parse.ParseComments | parse.SkipFuncCheck}
	_, err = t.Parse(string(content), "", "", c.treeMap)
	if err != nil {
		return err
	}

	c.blocks = map[string]*templateBlock{}
	for _, tree := range c.treeMap {
		block := &templateBlock{
			name: tree.Name,
			root: &templateVar{
				path:     "",
				pos:      tree.Root.Pos,
				children: map[string]*templateVar{},
			},
		}
		c.blocks[tree.Name] = block
		visitNodes(block.root, tree.Root)
	}

	return nil
}

func (c *Checker) Check() error {
	var errors error
	for _, block := range c.blocks {
		err := c.validateTemplate(block.root, nil)
		if err != nil {
			errors = multierr.Append(errors, err)
		}
	}
	return errors
}

func (c *Checker) position(pos parse.Pos) string {
	n := int(pos)
	content := c.content[:n]
	nl := bytes.LastIndexByte(content, '\n')

	col := n - nl
	line := 1 + bytes.Count(content, []byte{'\n'})
	filename := c.filename
	if filename == "" {
		filename = "-"
	}
	return fmt.Sprintf("%s:%d:%d", filename, line, col)
}

func main() {
	var c Checker
	err := c.ParseFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	err = c.Check()
	if err != nil {
		for _, err := range multierr.Errors(err) {
			log.Println(err)
		}
		os.Exit(1)
	}
}

func (c *Checker) validateTemplate(root *templateVar, typ types.Type) error {
	var errors error

	// ref. (*text/template.state).evalField
	if typ == nil {
		if len(root.annotations["type"]) == 0 {
			// TODO
			// return fmt.Errorf("no @type annotation")
			return nil
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
					if err := c.validateTemplate(v, ft); err != nil {
						errors = multierr.Append(errors, err)
					}
					break
				}
			}
			if !found {
				errors = multierr.Append(errors, fmt.Errorf("%s: %s not found in %s", c.position(v.pos), v.path, origType))
			}

		default:
			log.Printf("%s: BUG: unexpected type: %s in %s", c.position(v.pos), origType, v.path)
		}
	}

	return errors
}

// {{/* @key value */}}
var rxAnnotation = regexp.MustCompile(`^/\*\s*@(\w+)\s+(.*?)\s*\*/$`)

func visitNodes(v *templateVar, root *parse.ListNode) {
	if root == nil {
		return
	}

	for _, n := range root.Nodes {
		if c, ok := n.(*parse.CommentNode); ok {
			m := rxAnnotation.FindStringSubmatch(c.Text)
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

		newV := visitPipeNode(v, pipe)
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

func visitPipeNode(v *templateVar, pipe *parse.PipeNode) *templateVar {
	var result *templateVar

	for _, cmd := range pipe.Cmds {
		for _, arg := range cmd.Args {
			if fn, ok := arg.(*parse.FieldNode); ok {
				cur := v
				for _, ident := range fn.Ident {
					if _, ok := cur.children[ident]; !ok {
						cur.children[ident] = &templateVar{
							path:     cur.path + "." + ident,
							pos:      fn.Pos,
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
