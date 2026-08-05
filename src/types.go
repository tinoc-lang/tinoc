package src

import (
	"fmt"
	"strings"
)

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
		// A type imported from a C header (#importc), e.g. FILE, size_t,
		// or a user struct tag: use the header-derived type.
		if ct, ok := s.cTypes[t.Name]; ok {
			return ct
		}
		// Not a known primitive: likely a struct/enum/union name, which
		// this pass doesn't resolve yet. Treated as KindUnknown (valid,
		// opaque) rather than an error so var/const/fn involving
		// user-defined types don't hard-fail sema wholesale -- codegen
		// will surface a clear "unsupported" diagnostic if it's actually
		// asked to generate code for one.
		return &Type{Kind: KindUnknown, Name: t.Name}

	case *CQualType:
		// const/volatile/restrict only matters for C prototype spelling;
		// the type checker resolves through it.
		return s.resolveTypeExpr(t.Elem)

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

// === C Type Mapping ===
//
// cTypeFromQualType translates a C type string (as produced by clang's
// JSON AST `qualType` fields or gcc's -aux-info output, e.g.
// "const char *", "unsigned long", "FILE *", "size_t") into a resolved
// Tinoc *Type. Primitives map onto Tinoc's own primitives where a
// sensible equivalent exists; anything else (FILE, struct tags, glibc's
// internal typedefs, ...) becomes an opaque KindUnknown type whose Name is
// the original C spelling, so codegen (which never re-emits these types —
// the C header provides them) still yields valid C via Type.CType().

// cTypeBase is the word-level mapping from a lowercase, space-joined,
// qualifier-stripped C base type to a Tinoc primitive. Longs map to 64-bit
// (the LP64 data model used by every mainstream Linux/macOS target).
func cTypeBase(words string) *Type {
	switch words {
	case "void":
		return typeVoid
	case "bool", "_Bool":
		return typeBool
	case "char", "signed char":
		return typeI8
	case "unsigned char":
		return typeU8
	case "short", "short int", "signed short", "signed short int":
		return typeI16
	case "unsigned short", "unsigned short int":
		return typeU16
	case "int", "signed", "signed int":
		return typeI32
	case "unsigned", "unsigned int":
		return typeU32
	case "long", "long int", "signed long", "long long", "long long int",
		"signed long long", "signed long long int":
		return typeI64
	case "unsigned long", "unsigned long int", "unsigned long long", "unsigned long long int":
		return typeU64
	case "float":
		return typeF32
	case "double":
		return typeF64
	case "long double":
		return typeF128
	case "size_t":
		return typeUsize
	case "ssize_t", "ptrdiff_t", "intptr_t", "intmax_t", "int_least64_t", "int_fast64_t":
		return typeIsize
	case "uintptr_t", "uintmax_t", "uint_least64_t", "uint_fast64_t":
		return typeUsize
	case "wchar_t", "char32_t":
		return typeU32
	case "char16_t":
		return typeU16
	case "int8_t", "int_least8_t", "int_fast8_t":
		return typeI8
	case "uint8_t", "uint_least8_t", "uint_fast8_t":
		return typeU8
	case "int16_t", "int_least16_t", "int_fast16_t":
		return typeI16
	case "uint16_t", "uint_least16_t", "uint_fast16_t":
		return typeU16
	case "int32_t", "int_least32_t", "int_fast32_t":
		return typeI32
	case "uint32_t", "uint_least32_t", "uint_fast32_t":
		return typeU32
	case "__int128":
		return typeI128
	case "unsigned __int128":
		return typeU128
	default:
		// Unknown (FILE, struct stat, va_list, off_t, DIR, glibc internals,
		// user structs, ...): opaque, passed straight through to C codegen
		// where the included header defines it.
		return nil
	}
}

// cTypeFromQualType maps a C type spelling to a Tinoc *Type. It strips
// qualifiers (const/volatile/restrict/__restrict/__restrict__/inline), and
// handles multi-level pointers. Function-pointer types ("void (*)(int)")
// and anything unparseable degrade to an opaque pointer/unknown rather
// than failing the whole import.
func cTypeFromQualType(qt string) *Type {
	qt = strings.TrimSpace(qt)
	if qt == "" {
		return &Type{Kind: KindUnknown, Name: "void"}
	}

	// Function-pointer type: keep it as an opaque pointer. Codegen never
	// re-emits imported signatures (the header provides them), so this is
	// only ever used for type checking, where exactness here is
	// unimportant.
	if strings.Contains(qt, "(*") || strings.Contains(qt, "( *") || strings.Contains(qt, "*) (") || strings.Contains(qt, "*)(") {
		return &Type{Kind: KindPointer, Name: "^fn", Elem: &Type{Kind: KindUnknown, Name: "fn"}}
	}

	// Split into words and peel pointer stars / qualifiers off the end.
	// Stars are normalized onto their own tokens first so forms like
	// "char *restrict", "int *const", and "FILE **restrict" (clang's
	// JSON qualType spells these with the star glued to the qualifier)
	// all tokenize uniformly as ["char", "*", "restrict"] instead of
	// degrading to an opaque unknown type.
	qt = strings.ReplaceAll(qt, "*", " * ")
	tokens := strings.Fields(qt)
	depth := 0
	for len(tokens) > 0 {
		switch tokens[len(tokens)-1] {
		case "*":
			depth++
			tokens = tokens[:len(tokens)-1]
		case "const", "volatile", "restrict", "__restrict", "__restrict__", "__const", "inline", "_Atomic", "__attribute__":
			tokens = tokens[:len(tokens)-1]
		default:
			goto base
		}
	}
base:
	// Strip leading qualifiers too ("const char *" -> "char").
	for len(tokens) > 0 {
		switch strings.ToLower(tokens[0]) {
		case "const", "volatile", "restrict", "__restrict", "__restrict__", "__const", "inline", "_atomic":
			tokens = tokens[1:]
		default:
			goto mapped
		}
	}
mapped:

	var base *Type
	if len(tokens) > 0 {
		base = cTypeBase(strings.ToLower(strings.Join(tokens, " ")))
	}
	if base == nil {
		base = &Type{Kind: KindUnknown, Name: strings.Join(tokens, " ")}
	}

	t := base
	for i := 0; i < depth; i++ {
		t = &Type{Kind: KindPointer, Name: "^" + t.Name, Elem: t}
	}
	return t
}

// isCStringPtr reports whether t is a C string parameter type (pointer to
// char), which Tinoc's `str` values implicitly convert to.
func isCStringPtr(t *Type) bool {
	if t == nil || t.Kind != KindPointer {
		return false
	}
	if t.Elem == nil {
		return false
	}
	return t.Elem.Name == "i8" || t.Elem.Name == "char" || t.Elem.Kind == KindChar
}

// cDeclTypeSpelling renders a TypeExpr back into C syntax for the
// prototypes codegen emits for `extern "C" fn` declarations (e.g.
// `*const char` -> "const char *", `i32` -> "i32", `^void` -> "void *").
func cDeclTypeSpelling(te TypeExpr) string {
	if te == nil {
		return "void"
	}
	switch t := te.(type) {
	case *CQualType:
		return t.Qual + " " + cDeclTypeSpelling(t.Elem)
	case *PointerType:
		return cDeclTypeSpelling(t.Elem) + "*"
	case *NamedType:
		// Tinoc primitives pass through their own names (i32/u8/f64/...
		// are aliases tinoc.h defines, so they're valid C); anything else
		// is emitted verbatim so user-written C names (FILE, size_t, ...)
		// survive.
		return t.Name
	default:
		return t.String()
	}
}

// cParamListSpelling renders the parameter list of an extern "C" fn into C
// prototype text, appending `, ...` when variadic. Returns "void" for an
// empty non-variadic list, matching C's (void) idiom.
func cParamListSpelling(ecs *ExternCFuncStatement) string {
	var parts []string
	for _, p := range ecs.Params {
		if p == nil || p.Name == nil {
			continue
		}
		parts = append(parts, cDeclTypeSpelling(p.Type)+" "+p.Name.Value)
	}
	if ecs.Variadic {
		parts = append(parts, "...")
	}
	if len(parts) == 0 {
		return "void"
	}
	return strings.Join(parts, ", ")
}
