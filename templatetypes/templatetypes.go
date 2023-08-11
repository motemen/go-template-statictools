package templatetypes

import (
	"fmt"
	"go/types"
	"log"
	"regexp"
	"strings"
	"text/template/parse"

	"golang.org/x/tools/go/packages"

	"go.uber.org/multierr"
)

type state struct {
	Types  map[parse.Node]types.Type
	Errors []error
}

type TypeCheckError struct {
	Message string
}

func (e TypeCheckError) Error() string {
	return e.Message
}

// {{/* @key value */}}
var rxAnnotation = regexp.MustCompile(`^/\*\s*@(\w+)\s+(.*?)\s*\*/$`)

func (s *state) walk(dot *types.Type, node parse.Node) {
	if node == nil {
		return
	}

	switch node := node.(type) {
	case *parse.CommentNode:
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
					s.errorf("failed to load package %q: %v", pkgName, err)
				}
				if pkgs[0].Errors != nil {
					s.errorf("failed to load package %q: %v", pkgName, pkgs[0].Errors)
				}
				for _, pkg := range pkgs {
					obj := pkg.Types.Scope().Lookup(typeName)
					if obj == nil {
						continue
					}
					*dot = obj.Type()
					return
				}
				s.errorf("cannot load type %s.%s", pkgName, typeName)
			}
		}

	case *parse.ActionNode:
		s.checkPipeline(*dot, node.Pipe)

	case *parse.BreakNode, *parse.ContinueNode:
		return

	case *parse.IfNode:
		s.walkIfOrWith(parse.NodeIf, dot, node.Pipe, node.List, node.ElseList)

	case *parse.ListNode:
		if node == nil {
			return
		}
		for _, node := range node.Nodes {
			s.walk(dot, node)
		}

	case *parse.RangeNode:
		s.walkRange(dot, node)

	case *parse.TemplateNode:
		s.walkTemplate(dot, node)

	case *parse.WithNode:
		s.walkIfOrWith(parse.NodeWith, dot, node.Pipe, node.List, node.ElseList)

	case *parse.TextNode:

	default:
		s.TODO("walk: not implemented: %s (%T)", node, node)
	}
}

func (s *state) walkIfOrWith(nodeType parse.NodeType, dot *types.Type, pipe *parse.PipeNode, list, elseList *parse.ListNode) {
	switch nodeType {
	case parse.NodeWith:
		newDot := s.checkPipeline(*dot, pipe)
		s.walk(&newDot, list)
		s.walk(&newDot, elseList)
	case parse.NodeIf:
		s.walk(dot, list)
		s.walk(dot, elseList)
	default:
		panic("unreachable")
	}
}

func (s *state) walkRange(dot *types.Type, r *parse.RangeNode) {
	typ := peel(s.checkPipeline(*dot, r.Pipe))
	// TODO: assign
	switch typ := typ.(type) {
	case *types.Slice:
		elemType := typ.Elem()
		s.walk(&elemType, r.List)
		return
	case *types.Array:
		elemType := typ.Elem()
		s.walk(&elemType, r.List)
		return
	case *types.Map:
		// TODO
	case *types.Chan:
		// TODO
	default:
		s.errorf("range can't iterate over %v", *dot)
	}

	s.TODO("walkRange: %s", typ)
}

func (s *state) walkTemplate(dot *types.Type, t *parse.TemplateNode) {
	// TODO
	s.TODO("not implemted: walkTemplate: %s", t)
}

func (s *state) checkPipeline(dot types.Type, pipe *parse.PipeNode) (final types.Type) {
	if pipe == nil {
		return
	}

	for _, cmd := range pipe.Cmds {
		final = s.checkCommand(dot, cmd, final)
	}

	return
}

// ref. text/template.state.evalCommand()
func (s *state) checkCommand(dot types.Type, cmd *parse.CommandNode, final types.Type) types.Type {
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
	}

	// TODO: notAFunction
	switch firstWord.(type) {
	case *parse.DotNode:
		return dot
	}

	s.TODO("checkCommand: %s (%T)", firstWord, firstWord)

	return nil
}

func (s *state) checkFieldNode(dot types.Type, field *parse.FieldNode, args []parse.Node, final types.Type) types.Type {
	return s.checkFieldChain(dot, dot, field, field.Ident, args, final)
}

func (s *state) checkChainNode(dot types.Type, chain *parse.ChainNode, args []parse.Node, final types.Type) types.Type {
	if len(chain.Field) == 0 {
		s.errorf("internal error: no fields in evalChainNode")
	}
	if chain.Node.Type() == parse.NodeNil {
		s.errorf("indirection through explicit nil in %s", chain)
	}
	// (pipe).Field1.Field2 has pipe as .Node, fields as .Field. Eval the pipeline, then the fields.
	pipe := s.checkArg(dot, chain.Node)
	return s.checkFieldChain(dot, pipe, chain, chain.Field, args, final)
}

func (s *state) checkFieldChain(dot, receiver types.Type, node parse.Node, ident []string, args []parse.Node, final types.Type) types.Type {
	n := len(ident)
	for i := 0; i < n-1; i++ {
		receiver = s.checkField(dot, ident[i], node, nil, nil, receiver)
	}

	return s.checkField(dot, ident[n-1], node, args, final, receiver)
}

func (s *state) checkFunction(dot types.Type, node *parse.IdentifierNode, cmd parse.Node, args []parse.Node, final types.Type) types.Type {
	// TODO
	name := node.Ident

	switch name {
	case "index":
		a1 := s.checkArg(dot, args[1])
		_ = s.checkArg(dot, args[2])
		// TODO: check keyTypeOf(a1) == a2
		return valueTypeOf(a1)
	}

	s.TODO("checkFunction: node=%s cmd=%s args=%s", node, cmd, args)

	return nil
}

func (s *state) checkField(dot types.Type, fieldName string, node parse.Node, args []parse.Node, final types.Type, receiver types.Type) types.Type {
	if receiver == nil {
		return nil
	}

	// TODO: check method

	origReceiver := receiver
	hasArgs := len(args) > 1 || final != nil

	receiver = peel(receiver)

	switch receiver := receiver.(type) {
	case *types.Struct:
		for i := 0; i < receiver.NumFields(); i++ {
			f := receiver.Field(i)
			if f.Name() == fieldName {
				if hasArgs {
					// FIXME
					s.errorf("method %q does not take any arguments", fieldName)
				}
				return f.Type()
			}
		}

	case *types.Map:
		// TODO

	case *types.Pointer:
		// TODO

	}

	s.errorf("can't evaluate field %s in type %v", fieldName, origReceiver)
	return nil
}

func (s *state) checkArg(dot types.Type, n parse.Node) types.Type {
	// TODO
	switch arg := n.(type) {
	case *parse.DotNode:
		return dot
	case *parse.NilNode:
		return types.Typ[types.UntypedNil]
	case *parse.FieldNode:
		return s.checkFieldNode(dot, arg, []parse.Node{n}, nil)
	case *parse.PipeNode:
		return s.checkPipeline(dot, arg)
	}

	s.TODO("checkArg: %q (%T)", n, n)

	return dot
}

func (s *state) TODO(format string, args ...any) {
	log.Printf("TODO: "+format, args...)
}

func (s *state) debugf(format string, args ...any) {
	log.Printf("debug: "+format, args...)
}

func (s *state) errorf(format string, args ...interface{}) {
	log.Printf("error: "+format, args...)
	s.Errors = append(s.Errors, TypeCheckError{fmt.Sprintf(format, args...)})
}

func peel(typ types.Type) types.Type {
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

func Check(tree *parse.Tree) (err error) {
	var dot types.Type
	s := &state{}
	s.walk(&dot, tree.Root)

	return multierr.Combine(s.Errors...)
}
