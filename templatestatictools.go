package templatestatictools

import (
	"bytes"
	"encoding/json"
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
	Name string
	Root *templateRef
}

// templateRef is a later-to-be-evaluated reference in a template.
type templateRef struct {
	// string representation of the path to this reference.
	// e.g. "Foo.Bar.Baz" for {{ .Foo.Bar.Baz }}
	Path string

	// TODO:
	// Accessor = string | "mapKey" type | "templateArg" name
	//
	// Accessor of a ref for X:
	//   {{ .X }} -> "X"
	//   {{ index .Map .X }} -> "mapKey" (typeof .X)
	//   {{ template "name" .X }} -> "templateArg" "name"

	Pos parse.Pos

	// map of child references.
	// e.g. When Path is "Foo", Children["Bar"] is a reference to {{ .Foo.Bar }}
	Children map[string]*templateRef

	TypeSource typeSpec // fieldOf <ref> <field> / rangeValueOf <ref> / returnValueOf <ref>

	// TypeConstraints []typeSpec // argOf <ref> <pos> / rangeKeyOf <ref> / rangable

	Type types.Type

	// type expectation of this reference.
	// deprecated: to be replaced by TypeConstraints.
	ExpectedRangable bool // FIXME: could be more precise, e.g. ExpectedTypes

	Annotations map[string][]string

	root *templateRef
}

func (r *templateRef) MarshalJSON() ([]byte, error) {
	v := map[string]any{
		"Path":        r.Path,
		"Pos":         r.Pos,
		"Children":    r.Children,
		"TypeSource":  r.TypeSource,
		"Annotations": r.Annotations,
	}
	if r.Type != nil {
		v["Type"] = r.Type.String()
	}
	return json.Marshal(v)
}

type typeSpec struct {
	Kind      typeSpecKind
	ref       *templateRef
	stringArg string
	uintArg   uint
}

type typeSpecKind int

const (
	typeSpecKindUnknown typeSpecKind = iota

	// ref = reference which has the field or method
	// stringArg = name of the field/method
	// uintArg = (unused)
	typeSpecKindFieldOf

	// stringArg = (unused)
	// uintArg = (unused)
	typeSpecKindRangeValueOf

	// stringArg = (unused)
	// uintArg = (unused)
	typeSpecKindReturnValueOf

	// stringArg = (unused)
	// uintArg = (unused)
	typeSpecKindRangeKeyOf

	// stringArg = (unused)
	// uintArg = index of the argument
	typeSpecKindArgOf

	typeSpecKindRangable
)

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
			Root: &templateRef{
				Path:     "",
				Pos:      tree.Root.Pos,
				Children: map[string]*templateRef{},
				root:     nil,
			},
		}
		c.Blocks[tree.Name] = block
		c.visitNodes(block.Root, tree.Root)
	}

	return nil
}

func (c *Checker) Check() error {
	var errors error
	for _, block := range c.Blocks {
		err := c.validateTemplate(block.Root, nil)
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
			Mode:  packages.NeedTypes | packages.NeedTypesInfo,
			Tests: true, // FIXME: this is only for testing purpose
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
				continue
			}
			typ = obj.Type()
			break
		}
		if typ == nil {
			return fmt.Errorf("%s not found", targetType)
		}

		root.Type = typ
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

	for n, v := range root.Children {
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
					} else {
						v.Type = ft
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

func (c *Checker) visitNodes(ref *templateRef, root *parse.ListNode) {
	if root == nil {
		return
	}

	// ref. state.walk()
	// https://github.com/golang/go/blob/go1.20.7/src/text/template/exec.go#L261
	for _, n := range root.Nodes {
		if c, ok := n.(*parse.CommentNode); ok {
			m := rxAnnotation.FindStringSubmatch(c.Text)
			if m != nil {
				if ref.Annotations == nil {
					ref.Annotations = map[string][]string{}
				}
				ref.Annotations[m[1]] = append(ref.Annotations[m[1]], m[2])
			}
			// NOTE(motemen): maybe add `specificType "<typename>"` toref.TypeSources?
			continue
		}

		var pipe *parse.PipeNode
		switch n := n.(type) {
		case *parse.ActionNode:
			// {{ .Foo.Bar }}
			pipe = n.Pipe

		case *parse.IfNode:
			// {{ if .Foo.Bar }}
			pipe = n.Pipe

		case *parse.RangeNode:
			// {{ range .Foo.Bar }}
			pipe = n.Pipe

		case *parse.WithNode:
			// {{ with .Foo.Bar }}
			pipe = n.Pipe

		case *parse.TemplateNode:
			// {{ template "foo" .Foo.Bar }}
			pipe = n.Pipe

		case *parse.TextNode:
			// nop
			continue

		default:
			c.debug("%s: BUG: unexpected node: %T", c.position(n.Position()), n)
			continue
		}

		pipeRef := c.visitPipeNode(ref, pipe)

		switch n := n.(type) {
		case *parse.ActionNode:
			// nop

		case *parse.IfNode:
			c.visitNodes(ref, n.List)
			c.visitNodes(ref, n.ElseList)

		case *parse.RangeNode:
			// visit range body with new context
			if pipeRef == nil {
				// TODO: given the following template,
				//   {{ range (index .Map .Key) }}{{.Field}}{{end}}
				c.debug("%s: FIXME: could not evaluate ref for %T: pipe=%s", c.position(n.Position()), n, n.Pipe)
			} else {
				pipeRef.ExpectedRangable = true
				c.visitNodes(pipeRef, n.List)
			}

		case *parse.WithNode:
			if pipeRef == nil {
				c.debug("%s: FIXME: could not evaluate ref for %T: pipe=%s", c.position(n.Position()), n, n.Pipe)
				dummy := &templateRef{
					Path:     "#dummy",
					Pos:      -1,
					Children: map[string]*templateRef{},
					root:     ref.Root(), // Want to keep this
				}
				c.visitNodes(dummy, n.List)
			} else {
				c.visitNodes(pipeRef, n.List)
			}

		default:
			c.debug("%s: skip further processing; node=%s (%T)", c.position(n.Position()), n, n)
		}
	}
}

// visitPipeNode visits a pipe node and deepens the template reference tree.
// returns a new templateRef for later use, if the pipe node is a range or with.
func (c *Checker) visitPipeNode(env *templateRef, pipe *parse.PipeNode) *templateRef {
	var result *templateRef
	// if true, further evaluation is skipped
	// mostly because of lack of implementation
	var invalid bool

	c.debug("%s: visitPipeNode: pipe=%s", c.position(pipe.Position()), pipe)

	// cf. evalCommand

	// pipe = cmd0 | cmd1 ...
	for i, cmd := range pipe.Cmds {
		// cmd = arg0 arg1 arg2
		// where
		//   arg = FieldNode .X.Y.Z
		//       | ChainNode (index .M .K).Foo
		//       | IdentifierNode len
		//       | ...

		c.debug("%s: visitPipeNode: - cmd[%d] %q", c.position(cmd.Position()), i, cmd)

		if i > 0 {
			c.debug("%s: visitPipeNode: TODO: pipe result", c.position(cmd.Position()), i)
			continue
		}

		var err error
		result, err = c.visitArgs(env, cmd.Args)
		if err != nil {
			c.debug("%s: visitPipeNode: visitArgs: %s", c.position(cmd.Position()), err)
			invalid = true
			_ = invalid
			result = nil
		}
		/*
			for i, arg := range cmd.Args {
				c.debug("%s: visitPipeNode:   - arg[%d] %q (%T)", c.position(arg.Position()), i, arg, arg)

				var err error
				argRefs[i], err = c.visitArg(env, arg)
				if err != nil {
					c.debug("%s: visitPipeNode:     %s", c.position(arg.Position()), err)
					invalid = true
					result = nil
				}
			}

			if !invalid {
				var err error
				result, err = c.evalArgs(i == 0, argRefs, result)
				if err != nil {
					c.debug("%s: visitPipeNode: evalArgs: %s", c.position(cmd.Position()), err)
					invalid = true
					result = nil
				}
			}
		*/

	}

	return result
}

func (c *Checker) visitArgs(env *templateRef, args []parse.Node) (*templateRef, error) {
	first := args[0]
	switch first := first.(type) {
	case *parse.FieldNode:
		argRefs := make([]*templateRef, len(args))
		for i, arg := range args {
			var err error
			argRefs[i], err = c.visitArg(env, arg)
			_ = err // TODO
		}
		if len(args) == 1 {
			return argRefs[0], nil
		} else {
			c.debug("TODO: visitArgs: not implemented: %v", args)
		}

	case *parse.VariableNode:
		if len(args) == 1 {
			if first.Ident[0] == "$" {
				return c.visitField(env.Root(), first.Ident[1:]), nil
			}
		} else {
			c.debug("TODO: visitArgs: not implemented: %s", first)
		}

	case *parse.IdentifierNode:
		switch first.Ident {
		case "index":
			c.debug("index. args = %v", args)
			if len(args) == 3 {
				m, err := c.visitArg(env, args[1])
				if err != nil {
					return nil, err
				}
				_, err = c.visitArg(env, args[2])
				if err != nil {
					return nil, err
				}
				// (index .Map .Key) -> indexValueOf(.Map, .Key), whose path := ".Map[]"
				// k.typeConstraint += typeSpec{kind: typeSpecRangeKeyOf, ref: m}
				rangeValueRef := &templateRef{
					root: env.Root(),
					Path: m.Path + "[]",
					TypeSource: typeSpec{
						Kind: typeSpecKindRangeValueOf,
						ref:  m,
					},
				}
				m.Children["[]"] = rangeValueRef

				return rangeValueRef, nil
			}
		}
	}

	return nil, fmt.Errorf("TODO (first = %T)", first)
}

// {{.Foo}} -> fieldRef("Foo") -> type: field(.Foo)
// {{.Foo .Bar}} -> call(fieldRef("Foo"), fieldRef("Bar")) -> type: resultOf(.Foo)
// {{index .Map .Key}} -> call(function("index"), fieldRef("Key")) -> type: rangeValueOf(.Map)
func (c *Checker) evalArgs(first bool, argRefs []*templateRef, prevRef *templateRef) (*templateRef, error) {
	if !first {
		argRefs = append(argRefs, prevRef)
	}

	if len(argRefs) == 1 {
		return argRefs[0], nil
	}

	return nil, fmt.Errorf("evalArgs: not implemented")
}

func (c *Checker) visitField(env *templateRef, path []string) *templateRef {
	cur := env
	for _, ident := range path {
		if _, ok := cur.Children[ident]; !ok {
			path := cur.Path
			if env.ExpectedRangable {
				path += "[]"
			}
			if cur.Children == nil {
				cur.Children = map[string]*templateRef{}
			}
			cur.Children[ident] = &templateRef{
				Path:     path + "." + ident,
				Pos:      cur.Pos,
				Children: map[string]*templateRef{},
				root:     env.Root(),
			}
		}
		cur = cur.Children[ident]
	}
	return cur
}

func (c *Checker) visitArg(env *templateRef, arg parse.Node) (*templateRef, error) {
	switch arg := arg.(type) {
	case *parse.FieldNode:
		return c.visitField(env, arg.Ident), nil

	case *parse.DotNode:
		return env, nil

	case *parse.VariableNode:
		if arg.Ident[0] == "$" {
			return c.visitField(env.Root(), arg.Ident[1:]), nil
		}

	case *parse.PipeNode:
		c.visitPipeNode(env, arg)

	case *parse.IdentifierNode:
		// TODO

	case *parse.NilNode, *parse.NumberNode, *parse.BoolNode, *parse.StringNode:
		// TODO

	default:
		c.debug("unknown arg: %s (%T)", arg, arg)
	}

	return nil, fmt.Errorf("not implemented: %s (%T)", arg, arg)
}
