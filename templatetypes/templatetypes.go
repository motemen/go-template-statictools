package templatetypes

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

	"golang.org/x/tools/go/packages"

	"go.uber.org/multierr"
)

type Checker struct {
	filename string
	content  []byte
	errors   []error
	vars     []variable
	tree     *parse.Tree
	treeMap  map[string]*parse.Tree
}

type variable struct {
	name string
	typ  types.Type
}

func (s *Checker) setTopVarType(typ types.Type) {
	if len(s.vars) == 1 && s.vars[0].name == "$" {
		s.vars[0].typ = typ
	}
}

func (s *Checker) varType(name string) (types.Type, error) {
	for i := len(s.vars) - 1; i >= 0; i-- {
		if s.vars[i].name == name {
			return s.vars[i].typ, nil
		}
	}
	return nil, fmt.Errorf("undefined variable: %s", name)
}

type TypeCheckError struct {
	Pos     parse.Pos
	Message string
}

func (e TypeCheckError) Error() string {
	return e.Message
}

// {{/* @key value */}}
var rxAnnotation = regexp.MustCompile(`^/\*\s*@(\w+)\s+(.*?)\s*\*/$`)

// walk walks node.
// It returns new dot type. Only if @type annotation is given, the type will change.
func (s *Checker) walk(dot types.Type, node parse.Node) types.Type {
	if node == nil {
		return dot
	}

	switch node := node.(type) {
	case *parse.CommentNode:
		// Special case for static analysis.
		// If the comment is the form /* @type: <fullType> */,
		// The dot type is annotated as fullType.
		m := rxAnnotation.FindStringSubmatch(node.Text)
		if m != nil {
			key, value := m[1], m[2]
			if key == "type" {
				p := strings.LastIndex(value, ".")
				pkgName, typeName := value[:p], value[p+1:]

				pkgs, err := packages.Load(&packages.Config{
					Mode:  packages.NeedTypes | packages.NeedTypesInfo,
					Tests: true, // FIXME: this is only for testing purpose
				}, pkgName)
				if err != nil {
					s.errorf(node, "failed to load package %q: %v", pkgName, err)
				}
				if pkgs[0].Errors != nil {
					s.errorf(node, "failed to load package %q: %v", pkgName, pkgs[0].Errors)
				}
				for _, pkg := range pkgs {
					obj := pkg.Types.Scope().Lookup(typeName)
					if obj == nil {
						continue
					}
					// TODO: compare dot with obj.Type()
					s.setTopVarType(obj.Type())
					return obj.Type()
				}
				s.errorf(node, "cannot load type %s.%s", pkgName, typeName)
			}
		}

	case *parse.ActionNode:
		s.checkPipeline(dot, node.Pipe)

	case *parse.BreakNode, *parse.ContinueNode:
		return dot

	case *parse.IfNode:
		s.walkIfOrWith(parse.NodeIf, dot, node.Pipe, node.List, node.ElseList)

	case *parse.ListNode:
		if node == nil {
			return dot
		}
		for _, node := range node.Nodes {
			dot = s.walk(dot, node)
		}

	case *parse.RangeNode:
		s.walkRange(dot, node)

	case *parse.TemplateNode:
		s.walkTemplate(dot, node)

	case *parse.WithNode:
		s.walkIfOrWith(parse.NodeWith, dot, node.Pipe, node.List, node.ElseList)

	case *parse.TextNode:

	default:
		s.TODO(node, "walk: not implemented: %s (%T)", node, node)
	}

	return dot
}

func (s *Checker) walkIfOrWith(nodeType parse.NodeType, dot types.Type, pipe *parse.PipeNode, list, elseList *parse.ListNode) {
	switch nodeType {
	case parse.NodeWith:
		newDot := s.checkPipeline(dot, pipe)
		s.walk(newDot, list)
		s.walk(dot, elseList)
	case parse.NodeIf:
		s.walk(dot, list)
		s.walk(dot, elseList)
	default:
		panic("unreachable")
	}
}

func (s *Checker) walkRange(dot types.Type, r *parse.RangeNode) {
	typ := peelType(s.checkPipeline(dot, r.Pipe))

	// TODO: assign
	switch typ := typ.(type) {
	case *types.Slice:
		elemType := typ.Elem()
		_ = s.walk(elemType, r.List)
		return
	case *types.Array:
		elemType := typ.Elem()
		_ = s.walk(elemType, r.List)
		return
	case *types.Map:
		// TODO
	case *types.Chan:
		// TODO
	default:
		s.errorf(r, "range can't iterate over %v", dot)
		return
	}

	s.TODO(r, "walkRange: %s", typ)
}

func (s *Checker) walkTemplate(dot types.Type, t *parse.TemplateNode) {
	tree := s.treeMap[t.Name]
	if tree == nil {
		s.errorf(t, "template %q not defined", t.Name)
		return
	}

	dot = s.checkPipeline(dot, t.Pipe)
	newState := *s
	newState.vars = []variable{{"$", dot}}
	newState.errors = nil
	newState.walk(dot, tree.Root)
	s.errors = append(s.errors, newState.errors...)
}

func (s *Checker) checkPipeline(dot types.Type, pipe *parse.PipeNode) (final types.Type) {
	if pipe == nil {
		return
	}

	for _, cmd := range pipe.Cmds {
		final = s.checkCommand(dot, cmd, final)
	}

	return
}

// ref. text/template.state.evalCommand()
func (s *Checker) checkCommand(dot types.Type, cmd *parse.CommandNode, final types.Type) types.Type {
	firstWord := cmd.Args[0]
	switch n := firstWord.(type) {
	case *parse.FieldNode:
		return s.checkFieldNode(dot, n, cmd.Args, final)

	case *parse.ChainNode:
		return s.checkChainNode(dot, n, cmd.Args, final)

	case *parse.IdentifierNode:
		return s.checkFunction(dot, n, cmd, cmd.Args, final)

	case *parse.PipeNode:
		return s.checkPipeline(dot, n)

	case *parse.VariableNode:
		return s.checkVariableNode(dot, n, cmd.Args, final)
	}

	// TODO: notAFunction
	switch firstWord.(type) {
	case *parse.DotNode:
		return dot
	}

	s.TODO(cmd, "checkCommand: %s (%T)", firstWord, firstWord)

	return nil
}

func (s *Checker) checkFieldNode(dot types.Type, field *parse.FieldNode, args []parse.Node, final types.Type) types.Type {
	return s.checkFieldChain(dot, dot, field, field.Ident, args, final)
}

func (s *Checker) checkChainNode(dot types.Type, chain *parse.ChainNode, args []parse.Node, final types.Type) types.Type {
	if len(chain.Field) == 0 {
		s.errorf(chain, "internal error: no fields in checkChainNode")
		return nil
	}
	if chain.Node.Type() == parse.NodeNil {
		s.errorf(chain, "indirection through explicit nil in %s", chain)
		return nil
	}
	// (pipe).Field1.Field2 has pipe as .Node, fields as .Field. Eval the pipeline, then the fields.
	pipe := s.checkArg(dot, chain.Node)
	return s.checkFieldChain(dot, pipe, chain, chain.Field, args, final)
}

func (s *Checker) checkVariableNode(dot types.Type, variable *parse.VariableNode, args []parse.Node, final types.Type) types.Type {
	// $x.Field has $x as the first ident, Field as the second. Eval the var, then the fields.
	typ, err := s.varType(variable.Ident[0])
	if err != nil {
		s.errorf(variable, "%s", err)
		return nil
	}
	if len(variable.Ident) == 1 {
		return typ
	}
	return s.checkFieldChain(dot, typ, variable, variable.Ident[1:], args, final)
}

func (s *Checker) checkFieldChain(dot, receiver types.Type, node parse.Node, ident []string, args []parse.Node, final types.Type) types.Type {
	n := len(ident)
	for i := 0; i < n-1; i++ {
		receiver = s.checkField(dot, ident[i], node, nil, nil, receiver)
	}

	return s.checkField(dot, ident[n-1], node, args, final, receiver)
}

func (s *Checker) checkFunction(dot types.Type, node *parse.IdentifierNode, cmd parse.Node, args []parse.Node, final types.Type) types.Type {
	// TODO
	name := node.Ident

	switch name {
	case "index":
		a1 := s.checkArg(dot, args[1])
		_ = s.checkArg(dot, args[2])
		// TODO: check keyTypeOf(a1) == a2
		return valueTypeOf(a1)

	case "len":
		_ = s.checkArg(dot, args[1])
		return types.Typ[types.Int]

	}

	s.TODO(node, "checkFunction: cmd=%s args=%s", cmd, args)

	return nil
}

func (s *Checker) checkField(dot types.Type, fieldName string, node parse.Node, args []parse.Node, final types.Type, receiver types.Type) types.Type {
	if receiver == nil {
		return nil
	}

	// TODO: check method

	origReceiver := receiver
	hasArgs := len(args) > 1 || final != nil

	receiver = peelType(receiver)

	switch receiver := receiver.(type) {
	case *types.Struct:
		for i := 0; i < receiver.NumFields(); i++ {
			f := receiver.Field(i)
			if f.Name() == fieldName {
				if hasArgs {
					// FIXME
					s.errorf(node, "method %q does not take any arguments", fieldName)
				}
				return f.Type()
			}
		}

	case *types.Map:
		// TODO

	case *types.Pointer:
		// TODO

	}

	s.errorf(node, "can't evaluate field %s in type %v", fieldName, origReceiver)
	return nil
}

func (s *Checker) checkArg(dot types.Type, n parse.Node) types.Type {
	// TODO
	switch arg := n.(type) {
	case *parse.DotNode:
		return dot
	case *parse.NilNode:
		return types.Typ[types.UntypedNil]
	case *parse.FieldNode:
		return s.checkFieldNode(dot, arg, []parse.Node{n}, nil)
	case *parse.VariableNode:
		return s.checkVariableNode(dot, arg, nil, nil)
	case *parse.PipeNode:
		return s.checkPipeline(dot, arg)
	case *parse.IdentifierNode:
		return s.checkFunction(dot, arg, arg, nil, nil)
	case *parse.ChainNode:
		return s.checkChainNode(dot, arg, nil, nil)

	case *parse.StringNode:
		return types.Typ[types.String]
	}

	s.TODO(n, "checkArg: %q (%T)", n, n)

	return dot
}

func (s *Checker) TODO(node parse.Node, format string, args ...any) {
	s.debugf(node, "TODO: "+format, args...)
}

func (s *Checker) debugf(node parse.Node, format string, args ...any) {
	if node == nil {
		log.Printf("debug: "+format, args...)
	} else {
		log.Printf("%s: debug: "+format, append([]any{s.position(node.Position())}, args...)...)
	}
}

func (s *Checker) errorf(node parse.Node, format string, args ...interface{}) {
	s.errors = append(s.errors, TypeCheckError{
		Pos:     node.Position(),
		Message: fmt.Sprintf(format, args...),
	})
}

func peelType(typ types.Type) types.Type {
	for {
		switch t := typ.(type) {
		case *types.Pointer:
			typ = t.Elem()
		case *types.Named:
			typ = t.Underlying()
		default:
			return typ
		}
	}
}

func valueTypeOf(typ types.Type) types.Type {
	switch typ := typ.(type) {
	case *types.Map:
		return typ.Elem()
	case *types.Slice:
		return typ.Elem()
	case *types.Array:
		return typ.Elem()
	case *types.Basic:
		if typ.Kind() == types.String {
			// return the "byte" type
			return types.Typ[types.Byte]
		}
	}

	return nil
}

func (s *Checker) position(pos parse.Pos) string {
	n := int(pos)
	content := s.content[:n]
	nl := bytes.LastIndexByte(content, '\n')

	col := n - nl
	line := 1 + bytes.Count(content, []byte{'\n'})
	filename := s.filename
	if filename == "" {
		filename = "-"
	}
	return fmt.Sprintf("%s:%d:%d", filename, line, col)
}

func (s *Checker) FormatError(err error) string {
	if te, ok := err.(TypeCheckError); ok {
		return fmt.Sprintf("%s: %s", s.position(te.Pos), te.Message)
	} else {
		return err.Error()
	}
}

func (s *Checker) ParseFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	s.filename = filename

	return s.Parse(f)
}

func (s *Checker) Parse(r io.Reader) error {
	tree := &parse.Tree{
		Mode: parse.ParseComments | parse.SkipFuncCheck,
	}
	treeMap := map[string]*parse.Tree{}

	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	_, err = tree.Parse(string(content), "", "", treeMap)
	if err != nil {
		return err
	}

	s.content = content
	s.treeMap = treeMap
	s.tree = tree

	return nil
}

func (s *Checker) Check() error {
	if s.content == nil {
		panic("call Parse first")
	}

	for _, tree := range s.treeMap {
		s.vars = []variable{
			{name: "$", typ: nil},
		}
		s.walk(nil, tree.Root)
	}

	return multierr.Combine(s.errors...)
}
