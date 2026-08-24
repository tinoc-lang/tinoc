;=============================================================================
; Tags for TinocLang (code navigation / outline)
;=============================================================================

(function_declaration
  name: (identifier) @name
  parameters: (_) @parameters
  return_type: (_)? @return
  body: (_)? @body) @definition.function

(struct_declaration
  name: (identifier) @name
  body: (_)? @body) @definition.type

(enum_declaration
  name: (identifier) @name
  body: (_)? @body) @definition.type

(union_declaration
  name: (identifier) @name
  body: (_)? @body) @definition.type

(alias_declaration
  name: (identifier) @name) @definition.type

(variable_declaration
  name: (identifier) @name) @definition.variable

(const_declaration
  name: (identifier) @name) @definition.constant

(module_declaration
  name: (identifier) @name) @definition.module

(module_block_declaration
  name: (identifier) @name) @definition.module

(test_declaration
  name: (string) @name) @definition.test

(extern_fn_declaration
  name: (identifier) @name
  parameters: (_) @parameters
  return_type: (_)? @return) @definition.function

(struct_field_declaration
  name: (identifier) @name) @definition.field

(union_field_declaration
  name: (identifier) @name) @definition.field

(enum_variant
  name: (identifier) @name) @definition.enum

(call_expression
  function: (identifier) @name) @reference.call

(member_expression
  property: (identifier) @name) @reference
