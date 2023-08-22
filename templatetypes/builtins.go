package templatetypes

import (
	"fmt"
	"go/types"
)

// funcChecker is a function that checks the arguments of a function.
// args must be well-typed (i.e. non-nil).
type funcChecker func(dot types.Type, args []types.Type) (types.Type, error)

var builtinFuncs = map[string]funcChecker{
	"and":      nil, // checkBuiltinAnd,
	"call":     nil, // checkBuiltinCall,
	"html":     nil, // checkBuiltinHTMLEscaper,
	"index":    checkBuiltinIndex,
	"slice":    nil, // checkBuiltinSlice,
	"js":       nil, // checkBuiltinJSEscaper,
	"len":      checkBuiltinLen,
	"not":      nil, // checkBuiltinNot,
	"or":       nil, // checkBuiltinOr,
	"print":    nil, // checkBuiltinPrint,
	"printf":   nil, // checkBuiltinPrintf,
	"println":  nil, // checkBuiltinPrintln,
	"urlquery": nil, // checkBuiltinURLQueryEscaper,

	// Comparisons
	"eq": nil, // checkBuiltinEq, // ==
	"ge": nil, // checkBuiltinGe, // >=
	"gt": nil, // checkBuiltinGt, // >
	"le": nil, // checkBuiltinLe, // <=
	"lt": nil, // checkBuiltinLt, // <
	"ne": nil, // checkBuiltinNe, // !=
}

func checkBuiltinIndex(dot types.Type, args []types.Type) (types.Type, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("too few arguments")
	}

	item := args[0]
	for _, index := range args[1:] {
		t := indexTypeOf(item)
		if t == nil {
			return nil, fmt.Errorf("cannot index %s", item)
		}
		if !types.AssignableTo(index, t) {
			return nil, fmt.Errorf("index %s is not assignable to %s", index, t)
		}
		item = valueTypeOf(item)
	}

	return item, nil
}

func checkBuiltinLen(dot types.Type, args []types.Type) (types.Type, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 argument, got %d", len(args))
	}

	arg := args[0]
	switch arg := arg.(type) {
	case *types.Basic:
		if arg.Kind() == types.String {
			return types.Typ[types.Int], nil
		}
	case *types.Slice, *types.Array, *types.Map, *types.Chan:
		return types.Typ[types.Int], nil
	}

	return nil, fmt.Errorf("invalid argument type %s", arg)
}

func indexTypeOf(typ types.Type) types.Type {
	switch typ := typ.(type) {
	case *types.Map:
		return typ.Key()
	case *types.Slice:
		return types.Typ[types.UntypedInt]
	case *types.Array:
		return types.Typ[types.UntypedInt]
	case *types.Basic:
		if typ.Kind() == types.String {
			return types.Typ[types.UntypedInt]
		}
	}

	return nil
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
