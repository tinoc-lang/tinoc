;=============================================================================
; Highlights for TinocLang
;=============================================================================

;-----------------------------------------------------------------------------
; Keywords
;-----------------------------------------------------------------------------

[
  "var"
  "const"
  "fn"
  "struct"
  "enum"
  "union"
  "if"
  "else"
  "switch"
  "for"
  "while"
  "break"
  "continue"
  "return"
  "pub"
  "module"
  "alias"
  "extern"
  "and"
  "or"
  "orelse"
  "catch"
  "as"
] @keyword

;-----------------------------------------------------------------------------
; Preprocessor
;-----------------------------------------------------------------------------

[
  "#import"
  "#importc"
  "#run"
  "#partial"
] @keyword.directive

;-----------------------------------------------------------------------------
; Control Flow
;-----------------------------------------------------------------------------

(if_statement
  "if" @keyword.control.conditional)

(if_statement
  "else" @keyword.control.conditional)

(while_statement
  "while" @keyword.control.repeat)

(for_statement
  "for" @keyword.control.repeat)

(switch_statement
  "switch" @keyword.control.repeat)

(break_statement
  "break" @keyword.control.flow)

(continue_statement
  "continue" @keyword.control.flow)

(return_statement
  "return" @keyword.control.return)

;-----------------------------------------------------------------------------
; Types
;-----------------------------------------------------------------------------

(primitive_type) @type.builtin

(pointer_type
  "^" @type)

(optional_type
  "?" @type)

(array_type
  "[" @punctuation.bracket)

(slice_type
  "[]" @punctuation.bracket)

(struct_declaration
  "struct" @keyword.type)

(enum_declaration
  "enum" @keyword.type)

(union_declaration
  "union" @keyword.type)

(alias_declaration
  "alias" @keyword.type)

;-----------------------------------------------------------------------------
; Functions
;-----------------------------------------------------------------------------

(function_declaration
  "fn" @keyword.function)

(extern_fn_declaration
  "fn" @keyword.function)

(test_declaration
  "test" @keyword.function)

(function_declaration
  name: (identifier) @function)

(extern_fn_declaration
  name: (identifier) @function)

(call_expression
  function: (identifier) @function.call)

(call_expression
  function: (member_expression
    property: (identifier) @function.method))

;-----------------------------------------------------------------------------
; Parameters
;-----------------------------------------------------------------------------

(parameter
  name: (identifier) @variable.parameter)

(parameter
  type: (_) @type)

;-----------------------------------------------------------------------------
; Variables
;-----------------------------------------------------------------------------

(variable_declaration
  name: (identifier) @variable)

(const_declaration
  name: (identifier) @variable.builtin)

;-----------------------------------------------------------------------------
; Properties / Fields
;-----------------------------------------------------------------------------

(member_expression
  property: (identifier) @property)

(struct_field_declaration
  name: (identifier) @property)

(struct_literal_field
  name: (identifier) @property)

(union_field_declaration
  name: (identifier) @property)

;-----------------------------------------------------------------------------
; Enum Variants
;-----------------------------------------------------------------------------

(enum_variant
  name: (identifier) @constructor)

;-----------------------------------------------------------------------------
; Module Names
;-----------------------------------------------------------------------------

(module_declaration
  name: (identifier) @namespace)

(module_block_declaration
  name: (dotted_identifier) @namespace)

(import_declaration
  path: (import_path) @namespace)

(import_declaration
  alias: (identifier) @namespace)

(importc_declaration
  alias: (identifier) @namespace)

;-----------------------------------------------------------------------------
; Operators
;-----------------------------------------------------------------------------

[
  "+"
  "-"
  "*"
  "/"
  "%"
  "="
  "=="
  "!="
  "<"
  ">"
  "<="
  ">="
  "<<"
  ">>"
  "<<|"
  "&"
  "^"
  "|"
  "||"
  "+="
  "-="
  "*="
  "/="
  "%="
  "&="
  "|="
  "^="
  "+%"
  "-%"
  "*%"
  "+|"
  "-|"
  "*|"
  ".."
  "..."
  "?"
  "~"
  "**"
  "++"
  "<<|"
  "<<="
  ">>="
  "<<|="
  "**="
  "*%="
  "*|="
  "+%="
  "-%="
  "+|="
  "-|="
  "*|="
] @operator

;-----------------------------------------------------------------------------
; Punctuation
;-----------------------------------------------------------------------------

[
  "("
  ")"
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

[
  ","
  "."
  ":"
  ";"
  "=>"
  "^"
] @punctuation.delimiter

;-----------------------------------------------------------------------------
; Literals
;-----------------------------------------------------------------------------

(integer) @number

(float) @number.float

(string) @string

(char_literal) @character

(true) @boolean

(false) @boolean

(null) @constant.builtin

;-----------------------------------------------------------------------------
; Comments
;-----------------------------------------------------------------------------

(line_comment) @comment

(block_comment) @comment

;-----------------------------------------------------------------------------
; Attributes / Directives (for Helix / other editors)
;-----------------------------------------------------------------------------

(partial_declaration) @punctuation.special

;-----------------------------------------------------------------------------
; Test declarations
;-----------------------------------------------------------------------------

(test_declaration
  name: (string) @label)

;-----------------------------------------------------------------------------
; Generic parameters
;-----------------------------------------------------------------------------

(generic_type_args
  ":" @punctuation.special)

;-----------------------------------------------------------------------------
; Error / Special
;-----------------------------------------------------------------------------

(error_union_type
  "!" @type)

(enum_payload
  "(" @punctuation.bracket)

(enum_payload
  ")" @punctuation.bracket)
