package templatetypes

import (
	"fmt"
	"go/types"
	"regexp"
	"strings"
	"text/template/parse"

	"golang.org/x/tools/go/packages"
)

type state struct {
	Types map[parse.Node]types.Type
}

type TypeCheckError struct {
	Err error
}

func (e TypeCheckError) Error() string {
	return e.Err.Error()
}

func (e TypeCheckError) Unwrap() error {
	return e.Err
}

// {{/* @key value */}}
var rxAnnotation = regexp.MustCompile(`^/\*\s*@(\w+)\s+(.*?)\s*\*/$`)

func (s *state) walk(dot *types.Type, node parse.Node) {
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

	case *parse.ListNode:
		for _, node := range node.Nodes {
			s.walk(dot, node)
		}

	case *parse.TextNode:

	default:
		panic(fmt.Sprintf("not implemented: %T", node))
	}
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
	}

	// ...
	panic("TODO")
}

func (s *state) checkFieldNode(dot types.Type, field *parse.FieldNode, args []parse.Node, final types.Type) types.Type {
	return s.checkFieldChain(dot, dot, field, field.Ident, args, final)
}

func (s *state) checkFieldChain(dot, receiver types.Type, node parse.Node, ident []string, args []parse.Node, final types.Type) types.Type {
	n := len(ident)
	for i := 0; i < n-1; i++ {
		receiver = s.checkField(dot, ident[i], node, nil, nil, receiver)
	}

	return s.checkField(dot, ident[n-1], node, args, final, receiver)
}

func (s *state) checkField(dot types.Type, fieldName string, node parse.Node, args []parse.Node, final types.Type, receiver types.Type) types.Type {
	if receiver == nil {
		return nil
	}

	// TODO: check method

	origReceiver := receiver
	hasArgs := len(args) > 1 || final != nil

peelType:
	for {
		switch r := receiver.(type) {
		case *types.Named:
			receiver = r.Underlying()
		default:
			break peelType
		}
	}

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

	panic(TypeCheckError{fmt.Errorf("can't evaluate field %s in type %v", fieldName, origReceiver)})
}

func (s *state) errorf(format string, args ...any) {
	panic(TypeCheckError{fmt.Errorf(format, args...)})
}

func Check(tree *parse.Tree) (err error) {
	defer func() {
		v := recover()
		if e, ok := v.(error); ok {
			err = e
		} else if v == nil {
		} else {
			panic(v)
		}
	}()

	var dot types.Type
	s := &state{}
	s.walk(&dot, tree.Root)

	return
}
