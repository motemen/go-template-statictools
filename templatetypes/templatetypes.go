package templatetypes

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template/parse"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

type Checker struct {
	// default full annotated (path/to/pkg.type) type path of dot, if any
	DotType string

	// full annotated (path/to/pkg.name) variable that is a text/template.FuncMap for user-defined functions
	FuncMapVar string

	AllowUndefinedFuncs     bool
	AllowUndefinedTemplates bool

	Verbose bool

	errors  []error
	vars    []variable
	treeSet map[string]*parse.Tree
	visited map[*parse.Tree]bool
	funcMap map[string]*types.Signature
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

	s.TODO(nil, "variable: %s", name)

	return nil, nil
}

type TypeCheckError struct {
	Node    parse.Node
	Message string
}

func (e TypeCheckError) Error() string {
	return e.Message
}

// {{/* @key value */}}
var rxAnnotation = regexp.MustCompile(`^/\*\s*@(\w+)\s+(.*?)\s*\*/$`)

func (s *Checker) loadFuncMap(fullType string) (map[string]*types.Signature, error) {
	p := strings.LastIndex(fullType, ".")
	pkgName, varName := fullType[:p], fullType[p+1:]
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax,
		Tests: true, // FIXME: this is only for testing purpose
	}, pkgName)
	if err != nil {
		return nil, err
	}
	pkg := pkgs[0]
	if pkg.Errors != nil {
		return nil, fmt.Errorf("errors: %v", pkg.Errors)
	}

	varObj := pkg.Types.Scope().Lookup(varName)
	if varObj == nil {
		return nil, fmt.Errorf("cannot find %s.%s", pkgName, varName)
	}
	if types.TypeString(varObj.Type(), nil) != "text/template.FuncMap" {
		return nil, fmt.Errorf("%s.%s is not text/template.FuncMap", pkgName, varName)
	}

	funcTypeMap := map[string]*types.Signature{}

	for _, f := range pkg.Syntax {
		path, _ := astutil.PathEnclosingInterval(f, varObj.Pos(), varObj.Pos())
		if _, ok := path[0].(*ast.File); ok {
			continue
		}
		for _, node := range path {
			decl, ok := node.(*ast.GenDecl)
			if !ok || decl.Tok != token.VAR {
				continue
			}

			for _, spec := range decl.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i := range valueSpec.Names {
					if pkg.TypesInfo.Defs[valueSpec.Names[i]] != varObj {
						continue
					}
					lit := valueSpec.Values[i].(*ast.CompositeLit)
					for _, elt := range lit.Elts {
						kv := elt.(*ast.KeyValueExpr)
						key, err := strconv.Unquote(kv.Key.(*ast.BasicLit).Value)
						if err != nil {
							return nil, err
						}
						typ := pkg.TypesInfo.TypeOf(kv.Value)
						funcTypeMap[key] = typ.(*types.Signature)
					}
				}
			}
		}
	}

	return funcTypeMap, nil
}

func (s *Checker) setDotType(fullType string) (types.Type, error) {
	p := strings.LastIndex(fullType, ".")
	pkgName, typeName := fullType[:p], fullType[p+1:]

	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.NeedTypes | packages.NeedTypesInfo,
		Tests: true, // FIXME: this is only for testing purpose
	}, pkgName)
	if err != nil {
		return nil, fmt.Errorf("failed to load pakcage %q: %w", pkgName, err)
	}
	if pkgs[0].Errors != nil {
		return nil, fmt.Errorf("failed to load package %q: %v", pkgName, pkgs[0].Errors)
	}
	for _, pkg := range pkgs {
		obj := pkg.Types.Scope().Lookup(typeName)
		if obj == nil {
			continue
		}
		// TODO: compare dot with obj.Type()
		s.setTopVarType(obj.Type())
		return obj.Type(), nil
	}
	return nil, fmt.Errorf("cannot load type %s.%s", pkgName, typeName)
}

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
				typ, err := s.setDotType(value)
				if err != nil {
					s.errorf(node, "@type %s: %s", value, err)
				}
				return typ
			} else if key == "debug" && value == "show ." {
				s.debugf(node, "dot: %v", dot)
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
	if dot == nil {
		return
	}

	mark := len(s.vars)

	typ := peelType(s.checkPipeline(dot, r.Pipe))
	if typ == nil {
		return
	}

	var keyType, elemType types.Type
	switch typ := typ.(type) {
	case *types.Slice:
		keyType = types.Typ[types.UntypedInt]
		elemType = typ.Elem()
	case *types.Array:
		keyType = types.Typ[types.UntypedInt]
		elemType = typ.Elem()
	case *types.Map:
		keyType = typ.Key()
		elemType = typ.Elem()
	case *types.Chan:
		elemType = typ.Elem()
	default:
		s.errorf(r, "range can't iterate over %v, pipe: %s", typ, r.Pipe)
		return
	}

	if len(r.Pipe.Decl) > 0 {
		if r.Pipe.IsAssign {
			if len(r.Pipe.Decl) == 1 {
				// eg. "$v = range ."
				s.setVar(r.Pipe.Decl[0].Ident[0], elemType)
			} else {
				// eg. "$k, $v = range .""
				s.setVar(r.Pipe.Decl[0].Ident[0], keyType)
				s.setVar(r.Pipe.Decl[1].Ident[0], elemType)
			}
		} else {
			if len(r.Pipe.Decl) == 1 {
				// eg. "$v := range ."
				s.vars[len(s.vars)-1].typ = elemType
			} else {
				// eg. "$k, $v := range .""
				s.vars[len(s.vars)-2].typ = keyType
				s.vars[len(s.vars)-1].typ = elemType
			}
		}
	}

	_ = s.walk(elemType, r.List)

	s.vars = s.vars[:mark]
}

func (s *Checker) walkTemplate(dot types.Type, t *parse.TemplateNode) {
	tree := s.treeSet[t.Name]
	if tree == nil {
		if s.AllowUndefinedTemplates {
			s.debugf(t, "skip: template %q not defined", t.Name)
		} else {
			s.errorf(t, "template %q not defined", t.Name)
		}
		return
	}

	if _, ok := s.visited[tree]; ok {
		return
	}

	s.visited[tree] = false
	dot = s.checkPipeline(dot, t.Pipe)
	newState := *s
	newState.vars = []variable{{"$", dot}}
	newState.errors = nil
	newState.walk(dot, tree.Root)
	s.errors = append(s.errors, newState.errors...)
	s.visited[tree] = true
}

func (s *Checker) push(name string, typ types.Type) {
	s.vars = append(s.vars, variable{name: name, typ: typ})
}

func (s *Checker) setVar(name string, typ types.Type) {
	for i := len(s.vars) - 1; i >= 0; i-- {
		if s.vars[i].name == name {
			s.vars[i].typ = typ
			return
		}
	}

	s.TODO(nil, "setVar: %s", name)
}

func (s *Checker) checkPipeline(dot types.Type, pipe *parse.PipeNode) (final types.Type) {
	if pipe == nil {
		return
	}

	for _, cmd := range pipe.Cmds {
		final = s.checkCommand(dot, cmd, final)
	}

	for _, variable := range pipe.Decl {
		if pipe.IsAssign {
			s.setVar(variable.Ident[0], final)
		} else {
			s.push(variable.Ident[0], final)
		}
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
	case *parse.StringNode:
		return types.Typ[types.String]
	case *parse.NumberNode:
		return types.Typ[types.UntypedInt]
	case *parse.NilNode:
		return types.Typ[types.UntypedNil]
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
	name := node.Ident

	argTypes := []types.Type{}

	// args[0] is the function name/node
	for _, arg := range args[1:] {
		typ := s.checkArg(dot, arg)
		if typ == nil {
			return nil
		}
		argTypes = append(argTypes, typ)
	}
	if final != nil {
		argTypes = append(argTypes, final)
	}

	if checkBuiltin, ok := builtinFuncs[name]; ok {
		if checkBuiltin == nil {
			s.TODO(cmd, "checkFunction: builtin %q", name)
			return nil
		}

		typ, err := checkBuiltin(dot, argTypes)
		if err != nil {
			s.errorf(cmd, "function %s: %s", name, err)
			return nil
		}

		return typ
	}

	if fun, ok := s.funcMap[name]; ok {
		return fun.Results().At(0).Type()
	}

	if s.AllowUndefinedFuncs {
		s.debugf(cmd, "skip: function %q not found", name)
	} else {
		s.errorf(cmd, "function %q not found", name)
	}
	return nil
}

func (s *Checker) checkCall(dot types.Type, fun *types.Func, node parse.Node, name string, args []parse.Node, final types.Type) types.Type {
	argTypes := []types.Type{}

	if len(args) > 0 {
		for _, arg := range args[1:] {
			typ := s.checkArg(dot, arg)
			if typ == nil {
				return nil
			}
			argTypes = append(argTypes, typ)
		}
		if final != nil {
			argTypes = append(argTypes, final)
		}
	}

	// TODO: check arg
	_ = argTypes

	results := fun.Type().(*types.Signature).Results()
	switch results.Len() {
	case 1:
		return results.At(0).Type()

	case 2:
		if results.At(1).Type() != types.Universe.Lookup("error").Type() {
			s.errorf(node, "function %s: second return value must be error", name)
			return nil
		}
		return results.At(0).Type()

	default:
		s.errorf(node, "function %s: must return 1 or 2 values", name)
		return nil
	}
}

func lookupMethod(typ types.Type, name string) *types.Func {
	switch typ := typ.(type) {
	case *types.Named:
		for i := 0; i < typ.NumMethods(); i++ {
			meth := typ.Method(i)
			if meth.Name() == name {
				return meth
			}
		}
		return lookupMethod(typ.Underlying(), name)

	case *types.Interface:
		for i := 0; i < typ.NumMethods(); i++ {
			meth := typ.Method(i)
			if meth.Name() == name {
				return meth
			}
		}
	}

	return nil
}

func (s *Checker) checkField(dot types.Type, fieldName string, node parse.Node, args []parse.Node, final types.Type, receiver types.Type) types.Type {
	if receiver == nil {
		return nil
	}

	// TODO: check method

	origReceiver := receiver
	hasArgs := len(args) > 1 || final != nil

	obj, _, _ := types.LookupFieldOrMethod(receiver, false, nil, fieldName)
	if obj != nil {
		if meth, ok := obj.(*types.Func); ok {
			return s.checkCall(dot, meth, node, fieldName, args, final)
		} else {
			if hasArgs {
				s.errorf(node, "field %q does not take any arguments", fieldName)
			}
			return obj.Type()
		}
	}

	if meth := lookupMethod(receiver, fieldName); meth != nil {
		return s.checkCall(dot, meth, node, fieldName, args, final)
	}

	receiver = peelType(receiver)

	switch receiver := receiver.(type) {
	case *types.Map:
		return valueTypeOf(receiver)
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
	case *parse.NumberNode:
		return types.Typ[types.UntypedInt]
	case *parse.BoolNode:
		return types.Typ[types.UntypedBool]
	}

	s.TODO(n, "checkArg: %q (%T)", n, n)

	return dot
}

func (s *Checker) TODO(node parse.Node, format string, args ...any) {
	s.debugf(node, "TODO: "+format, args...)
}

func (s *Checker) debugf(node parse.Node, format string, args ...any) {
	if !s.Verbose {
		return
	}
	if node == nil {
		log.Printf("debug: "+format, args...)
	} else {
		loc, context := s.diagContext(node)
		log.Printf("%s: debug: in %s: "+format, append([]any{loc, context}, args...)...)
	}
}

func (s *Checker) errorf(node parse.Node, format string, args ...interface{}) {
	s.errors = append(s.errors, TypeCheckError{
		Node:    node,
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

func (s *Checker) diagContext(node parse.Node) (loc, context string) {
	return (*parse.Tree).ErrorContext(nil, node)
}

func (s *Checker) FormatError(err error) string {
	if te, ok := err.(TypeCheckError); ok {
		loc, context := s.diagContext(te.Node)
		return fmt.Sprintf("%s: in %s: %s", loc, context, te.Message)
	} else {
		return err.Error()
	}
}

func (s *Checker) ParseFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	return s.Parse(filename, f)
}

func (s *Checker) Parse(name string, r io.Reader) error {
	tree := parse.New(name)
	tree.Mode = parse.ParseComments | parse.SkipFuncCheck

	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	treeSet := map[string]*parse.Tree{}
	_, err = tree.Parse(string(content), "", "", treeSet)
	if err != nil {
		return err
	}

	if s.treeSet == nil {
		s.treeSet = map[string]*parse.Tree{}
	}
	for name, tree := range treeSet {
		s.treeSet[name] = tree
	}

	return nil
}

func (s *Checker) Check(entryPoint string) error {
	s.visited = map[*parse.Tree]bool{}

	tree := s.treeSet[entryPoint]
	if tree == nil {
		return fmt.Errorf("entry point %q not found", entryPoint)
	}

	s.vars = []variable{
		{name: "$", typ: nil},
	}

	if s.FuncMapVar != "" {
		m, err := s.loadFuncMap(s.FuncMapVar)
		if err != nil {
			return err
		}
		s.funcMap = m
	}

	var typ types.Type
	if s.DotType != "" {
		var err error
		typ, err = s.setDotType(s.DotType)
		if err != nil {
			return err
		}
	}
	s.walk(typ, tree.Root)

	return errors.Join(s.errors...)
}
