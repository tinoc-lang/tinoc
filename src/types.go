package src

import "fmt"

// === Type System ===
//
// This file defines Sema's internal representation of Tinoc types
// (distinct from the AST's TypeExpr, which is just syntax). Sema resolves
// every TypeExpr it encounters down to a *Type, and Codegen consumes those
// resolved *Type values (never TypeExpr) when it needs to know what C type
// to emit.
//
// Scope for this pass: primitives (integers, floats, bool, char, void,
// str) plus pointer types (`^T`), since that's what var/const/static
// var/static const/fn need end-to-end. Compound types (struct/enum/union),
// optionals, error unions, arrays/slices, and generics are recognized
// syntactically elsewhere in the AST but are not yet resolved by Sema;
// referencing them here produces a clear "not yet supported" diagnostic
// rather than a silent wrong answer.

// TypeKind categorizes a resolved Type.
type TypeKind int

const (
	KindInvalid TypeKind = iota
	KindVoid
	KindBool
	KindChar
	KindInt
	KindFloat
	KindStr
	KindPointer
	KindUnknown // valid but not yet resolvable by this pass (struct/enum/etc)
)

// Type is Sema's resolved representation of a Tinoc type.
type Type struct {
	Kind TypeKind
	Name string // canonical Tinoc spelling, e.g. "i32", "str", "bool"

	// Integer-specific.
	IntBits   int // 8, 16, 32, 64, 128; 0 for usize/isize (platform width)
	IntSigned bool
	IsSize    bool // true for usize/isize

	// Float-specific.
	FloatBits int // 32, 64, 128

	// Pointer-specific.
	Elem *Type
}

func (t *Type) String() string {
	if t == nil {
		return "<nil>"
	}
	if t.Kind == KindPointer {
		return "^" + t.Elem.String()
	}
	return t.Name
}

// CType returns the C99 type spelling to emit for this Type, per the
// mapping table in syntax.md (Tinoc Type -> C Equivalent), using the
// aliases tinoc.h defines (u8, i32, f64, str, ...) rather than the raw
// stdint.h names, since generated code includes tinoc.h.
func (t *Type) CType() string {
	if t == nil {
		return "void"
	}
	switch t.Kind {
	case KindVoid:
		return "void"
	case KindBool:
		return "bool"
	case KindChar:
		return "char32"
	case KindStr:
		return "str"
	case KindInt, KindFloat:
		return t.Name // tinoc.h defines u8/i32/f32/usize/... 1:1 with Tinoc's own names
	case KindPointer:
		return t.Elem.CType() + "*"
	default:
		return t.Name
	}
}

// Predefined primitive types, shared/interned so type comparisons can use
// simple pointer or field equality without re-allocating.
var (
	typeVoid = &Type{Kind: KindVoid, Name: "void"}
	typeBool = &Type{Kind: KindBool, Name: "bool"}
	typeChar = &Type{Kind: KindChar, Name: "char"}
	typeStr  = &Type{Kind: KindStr, Name: "str"}

	typeU8    = &Type{Kind: KindInt, Name: "u8", IntBits: 8, IntSigned: false}
	typeU16   = &Type{Kind: KindInt, Name: "u16", IntBits: 16, IntSigned: false}
	typeU32   = &Type{Kind: KindInt, Name: "u32", IntBits: 32, IntSigned: false}
	typeU64   = &Type{Kind: KindInt, Name: "u64", IntBits: 64, IntSigned: false}
	typeU128  = &Type{Kind: KindInt, Name: "u128", IntBits: 128, IntSigned: false}
	typeUsize = &Type{Kind: KindInt, Name: "usize", IntSigned: false, IsSize: true}

	typeI8    = &Type{Kind: KindInt, Name: "i8", IntBits: 8, IntSigned: true}
	typeI16   = &Type{Kind: KindInt, Name: "i16", IntBits: 16, IntSigned: true}
	typeI32   = &Type{Kind: KindInt, Name: "i32", IntBits: 32, IntSigned: true}
	typeI64   = &Type{Kind: KindInt, Name: "i64", IntBits: 64, IntSigned: true}
	typeI128  = &Type{Kind: KindInt, Name: "i128", IntBits: 128, IntSigned: true}
	typeIsize = &Type{Kind: KindInt, Name: "isize", IntSigned: true, IsSize: true}

	typeF32  = &Type{Kind: KindFloat, Name: "f32", FloatBits: 32}
	typeF64  = &Type{Kind: KindFloat, Name: "f64", FloatBits: 64}
	typeF128 = &Type{Kind: KindFloat, Name: "f128", FloatBits: 128}
)

// primitiveTypes maps every primitive type name recognized in type
// position to its resolved Type.
var primitiveTypes = map[string]*Type{
	"void": typeVoid,
	"bool": typeBool,
	"char": typeChar,
	"str":  typeStr,

	"u8": typeU8, "u16": typeU16, "u32": typeU32, "u64": typeU64, "u128": typeU128, "usize": typeUsize,
	"i8": typeI8, "i16": typeI16, "i32": typeI32, "i64": typeI64, "i128": typeI128, "isize": typeIsize,

	"f32": typeF32, "f64": typeF64, "f128": typeF128,
}

// isIntegerType reports whether t is one of the built-in integer types.
func (t *Type) isInteger() bool { return t != nil && t.Kind == KindInt }

// isFloatType reports whether t is one of the built-in float types.
func (t *Type) isFloat() bool { return t != nil && t.Kind == KindFloat }

// isNumeric reports whether t supports arithmetic operators (+, -, *, /, %).
func (t *Type) isNumeric() bool { return t.isInteger() || t.isFloat() }

// rank gives a coarse "size class" for integer types so integer-literal
// inference (syntax.md: "defaults to i32... but if literal's size is more
// than i32, so it will go for i64..i128") can pick the smallest type that
// fits.
func intRankForBits(bits int) *Type {
	switch {
	case bits <= 32:
		return typeI32
	case bits <= 64:
		return typeI64
	default:
		return typeI128
	}
}

// typesEqual reports whether two resolved types are identical. Pointers
// compare structurally (element types must also be equal).
func typesEqual(a, b *Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindPointer:
		return typesEqual(a.Elem, b.Elem)
	case KindInt:
		return a.Name == b.Name
	case KindFloat:
		return a.Name == b.Name
	default:
		return a.Kind == b.Kind
	}
}

// assignable reports whether a value of type src can be assigned/bound to
// a location of type dst without an explicit cast. Tinoc's var/const
// explicit-type form (`var x i32 = <expr>`) requires the initializer's
// type to match; the one flexibility built in here is untyped integer/
// float literals, which adapt to the declared type the same way Go's
// untyped constants do (see Sema's literal handling in sema.go).
func assignable(dst, src *Type) bool {
	if dst == nil || src == nil {
		return true // avoid cascading errors when one side already failed
	}
	return typesEqual(dst, src)
}

// resolveTypeExpr converts a parsed TypeExpr into a resolved *Type. Returns
// nil and records a diagnostic when the type is not (yet) supported by
// this pass (structs, enums, unions, arrays, slices, optionals, error
// unions, generics) or refers to an unknown name.
func (s *Sema) resolveTypeExpr(te TypeExpr) *Type {
	if te == nil {
		return nil
	}
	switch t := te.(type) {
	case *NamedType:
		if prim, ok := primitiveTypes[t.Name]; ok {
			return prim
		}
		// Not a known primitive: likely a struct/enum/union name, which
		// this pass doesn't resolve yet. Treated as KindUnknown (valid,
		// opaque) rather than an error so var/const/fn involving
		// user-defined types don't hard-fail sema wholesale -- codegen
		// will surface a clear "unsupported" diagnostic if it's actually
		// asked to generate code for one.
		return &Type{Kind: KindUnknown, Name: t.Name}

	case *PointerType:
		elem := s.resolveTypeExpr(t.Elem)
		if elem == nil {
			return nil
		}
		return &Type{Kind: KindPointer, Name: "^" + elem.Name, Elem: elem}

	case *GenericType:
		s.diags.Error("sema", 0, 0, "generic types are not yet supported (%s)", t.String())
		return nil
	case *OptionalType:
		s.diags.Error("sema", 0, 0, "optional types are not yet supported (%s)", t.String())
		return nil
	case *ErrorUnionType:
		s.diags.Error("sema", 0, 0, "error union types are not yet supported (%s)", t.String())
		return nil
	case *ArrayType:
		s.diags.Error("sema", 0, 0, "array/slice types are not yet supported (%s)", t.String())
		return nil
	default:
		s.diags.Error("sema", 0, 0, "unsupported type expression: %s", te.String())
		return nil
	}
}

// describeMismatch renders a "cannot use X (type A) as type B" style
// message, matching Go's own phrasing for the same class of error.
func describeMismatch(exprDesc string, got, want *Type) string {
	return fmt.Sprintf("cannot use %s (type %s) as type %s", exprDesc, got.String(), want.String())
}
