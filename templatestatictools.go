package templatestatictools

import (
	"bytes"
	"encoding/json"
	"flag"
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

type TemplateVar struct {
	Foo1 string
}

type templateBlock struct {
	Name string
	Refs *templateRef
}

type templateRef struct {
	Path             string
	Pos              parse.Pos
	Refs             map[string]*templateRef
	ExpectedRangable bool // FIXME
	Annotations      map[string][]string
	root             *templateRef
}

func (r *templateRef) Root() *templateRef {
	if r.root == nil {
		return r
	}
	return r.root
}

type Checker struct {
	Filename string
	content  []byte
	Blocks   map[string]*templateBlock
	treeMap  map[string]*parse.Tree
}

func init() {
	log.SetFlags(0)
}

func (c *Checker) debug(format string, args ...any) {
	log.Printf("debug: "+format, args...)
}

func (c *Checker) ParseFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	c.Filename = filename
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

	c.Blocks = map[string]*templateBlock{}
	for _, tree := range c.treeMap {
		block := &templateBlock{
			Name: tree.Name,
			Refs: &templateRef{
				Path: "",
				Pos:  tree.Root.Pos,
				Refs: map[string]*templateRef{},
				root: nil,
			},
		}
		c.Blocks[tree.Name] = block
		c.visitNodes(block.Refs, tree.Root)
	}

	return nil
}

func (c *Checker) Check() error {
	var errors error
	for _, block := range c.Blocks {
		err := c.validateTemplate(block.Refs, nil)
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
	filename := c.Filename
	if filename == "" {
		filename = "-"
	}
	return fmt.Sprintf("%s:%d:%d", filename, line, col)
}

// Usage: gotmplcheck [-json] tmpl.in
func main() {
	var flagJson bool
	flag.BoolVar(&flagJson, "json", false, "output parsed result as JSON")
	flag.Parse()

	var c Checker
	err := c.ParseFile(flag.Args()[0])
	if err != nil {
		log.Fatal(err)
	}

	if flagJson {
		b, err := json.MarshalIndent(c.Blocks, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(b))
		return
	}

	err = c.Check()
	if err != nil {
		for _, err := range multierr.Errors(err) {
			log.Println(err)
		}
		os.Exit(1)
	}
}

func (c *Checker) validateTemplate(root *templateRef, typ types.Type) error {
	var errors error

	// ref. (*text/template.state).evalField
	if typ == nil {
		if len(root.Annotations["type"]) == 0 {
			// TODO
			// return fmt.Errorf("no @type annotation")
			return nil
		}
		if len(root.Annotations["type"]) > 1 {
			return fmt.Errorf("multiple @type annotations")
		}

		targetType := root.Annotations["type"][0]
		p := strings.LastIndex(targetType, ".")
		pkgName, typeName := targetType[:p], targetType[p+1:]

		pkgs, err := packages.Load(&packages.Config{
			Mode: packages.NeedTypes | packages.NeedTypesInfo,
		}, pkgName)
		if err != nil {
			return err
		}
		log.Println(pkgs)
		if pkgs[0].Errors != nil {
			return pkgs[0].Errors[0]
		}
		for _, pkg := range pkgs {
			obj := pkg.Types.Scope().Lookup(typeName)
			if obj == nil {
				return fmt.Errorf("%s not found", targetType)
			}
			typ = obj.Type()
			break
		}
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

	for n, v := range root.Refs {
		switch t := typ.(type) {
		case *types.Struct:
			var found bool
			for i := 0; i < t.NumFields(); i++ {
				f := t.Field(i)
				if f.Name() == n {
					found = true
					ft := f.Type()
					if v.ExpectedRangable {
						if t, ok := ft.(*types.Slice); ok {
							ft = t.Elem()
						} else {
							return fmt.Errorf("%s: expected slice, but got %s", v.Path, ft)
						}
					}
					if err := c.validateTemplate(v, ft); err != nil {
						errors = multierr.Append(errors, err)
					}
					break
				}
			}
			if !found {
				errors = multierr.Append(errors, fmt.Errorf("%s: %s not found in %s", c.position(v.Pos), v.Path, origType))
			}

		case *types.Interface:
			var found bool
			for i := 0; i < t.NumMethods(); i++ {
				m := t.Method(i)
				if m.Name() == n {
					found = true
					mt := m.Type().(*types.Signature)
					rt := mt.Results().At(0).Type()
					if v.ExpectedRangable {
						if t, ok := rt.(*types.Slice); ok {
							rt = t.Elem()
						} else {
							return fmt.Errorf("%s: expected slice, but got %s", v.Path, mt)
						}
					}
					if err := c.validateTemplate(v, rt); err != nil {
						errors = multierr.Append(errors, err)
					}
					break
				}
			}
			if !found {
				errors = multierr.Append(errors, fmt.Errorf("%s: %s not found in %s", c.position(v.Pos), v.Path, origType))
			}

		default:
			log.Printf("%s: BUG: unexpected type: %s in %s", c.position(v.Pos), origType, v.Path)
		}
	}

	return errors
}

// {{/* @key value */}}
var rxAnnotation = regexp.MustCompile(`^/\*\s*@(\w+)\s+(.*?)\s*\*/$`)

func (c *Checker) visitNodes(v *templateRef, root *parse.ListNode) {
	if root == nil {
		return
	}

	for _, n := range root.Nodes {
		if c, ok := n.(*parse.CommentNode); ok {
			m := rxAnnotation.FindStringSubmatch(c.Text)
			if m != nil {
				if v.Annotations == nil {
					v.Annotations = map[string][]string{}
				}
				v.Annotations[m[1]] = append(v.Annotations[m[1]], m[2])
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

		newV := c.visitPipeNode(v, pipe)

		switch n := n.(type) {
		case *parse.ActionNode:
			// nop
		case *parse.IfNode:
			c.visitNodes(v, n.List)
			c.visitNodes(v, n.ElseList)
		case *parse.RangeNode:
			if newV == nil {
				c.debug("%s: skip further processing; pipe=%s; node=%T", c.position(n.Position()), n.Pipe, n)
			} else {
				newV.ExpectedRangable = true
				c.visitNodes(newV, n.List)
			}
		case *parse.WithNode:
			if newV == nil {
				// c.debug("%s: skip further processing; pipe=%s; node=%T", c.position(n.Position()), n.Pipe, n)
				dummy := &templateRef{
					Path: "#dummy",
					Pos:  -1,
					Refs: map[string]*templateRef{},
					root: v.Root(), // Need to keep this
				}
				c.visitNodes(dummy, n.List)
			} else {
				c.visitNodes(newV, n.List)
			}
		default:
			c.debug("%s: skip further processing; node=%s (%T)", c.position(n.Position()), n, n)
		}
	}
}

func (c *Checker) visitPipeNode(v *templateRef, pipe *parse.PipeNode) *templateRef {
	var result *templateRef

	for _, cmd := range pipe.Cmds {
		for _, arg := range cmd.Args {
			if fn, ok := arg.(*parse.FieldNode); ok {
				cur := v
				for _, ident := range fn.Ident {
					if _, ok := cur.Refs[ident]; !ok {
						path := cur.Path
						if v.ExpectedRangable {
							path += "[]"
						}
						cur.Refs[ident] = &templateRef{
							Path: path + "." + ident,
							Pos:  fn.Pos,
							Refs: map[string]*templateRef{},
							root: v.Root(),
						}
					}
					cur = cur.Refs[ident]
				}
				if result == nil {
					result = cur
				}
			} else if _, ok := arg.(*parse.DotNode); ok {
				if result == nil {
					result = v
				}
			} else if vn, ok := arg.(*parse.VariableNode); ok {
				if vn.Ident[0] == "$" {
					cur := v.Root()
					for _, ident := range vn.Ident[1:] {
						if _, ok := cur.Refs[ident]; !ok {
							path := cur.Path
							if v.ExpectedRangable {
								path += "[]"
							}
							cur.Refs[ident] = &templateRef{
								Path: path + "." + ident,
								Pos:  vn.Pos,
								Refs: map[string]*templateRef{},
								root: v.Root(),
							}
						}
						cur = cur.Refs[ident]
					}
					if result == nil {
						result = cur
					}
				}
			} else if pn, ok := arg.(*parse.PipeNode); ok {
				c.visitPipeNode(v, pn)
			} else if _, ok := arg.(*parse.IdentifierNode); ok {
				// ignore for now
			} else {
				c.debug("unknown arg: %s (%T)", arg, arg)
			}
		}
	}

	return result
}
