package templatetypes

import (
	"fmt"
	"go/types"
)

// funcChecker is a function that checks the arguments of a function.
// args must be well-typed (i.e. non-nil).
type funcChecker func(dot types.Type, args []types.Type) (types.Type, error)

var builtinFuncs = map[string]funcChecker{
	"and":      stubBuiltinFunc(types.Typ[types.Bool]),
	"call":     checkBuiltinCall,
	"html":     stubBuiltinFunc(types.Typ[types.String]),
	"index":    checkBuiltinIndex,
	"slice":    checkBuiltinSlice,
	"js":       stubBuiltinFunc(types.Typ[types.String]),
	"len":      checkBuiltinLen,
	"not":      stubBuiltinFunc(types.Typ[types.Bool]),
	"or":       stubBuiltinFunc(types.Typ[types.Bool]),
	"print":    stubBuiltinFunc(types.Typ[types.String]),
	"printf":   stubBuiltinFunc(types.Typ[types.String]),
	"println":  stubBuiltinFunc(types.Typ[types.String]),
	"urlquery": stubBuiltinFunc(types.Typ[types.String]),

	// Comparisons
	"eq": stubBuiltinFunc(types.Typ[types.Bool]), // ==
	"ge": stubBuiltinFunc(types.Typ[types.Bool]), // >=
	"gt": stubBuiltinFunc(types.Typ[types.Bool]), // >
	"le": stubBuiltinFunc(types.Typ[types.Bool]), // <=
	"lt": stubBuiltinFunc(types.Typ[types.Bool]), // <
	"ne": stubBuiltinFunc(types.Typ[types.Bool]), // !=
}

func stubBuiltinFunc(fixedType types.Type) func(dot types.Type, args []types.Type) (types.Type, error) {
	return func(dot types.Type, args []types.Type) (types.Type, error) {
		return fixedType, nil
	}
}

func checkBuiltinCall(dot types.Type, args []types.Type) (types.Type, error) {
	// FXIME: check types
	fn, ok := args[0].(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("expected function type, got %s", args[0])
	}
	return fn.Results().At(0).Type(), nil
}

func checkBuiltinSlice(dot types.Type, args []types.Type) (types.Type, error) {
	// FIXME: check types
	return args[0], nil
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
