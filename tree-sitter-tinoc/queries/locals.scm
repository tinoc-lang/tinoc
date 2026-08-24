;=============================================================================
; Locals for TinocLang
;=============================================================================

; Scopes
(source_file) @scope
(block) @scope
(function_declaration) @scope
(struct_declaration) @scope
(enum_declaration) @scope
(union_declaration) @scope
(module_block_declaration) @scope
(if_statement) @scope
(while_statement) @scope
(for_statement) @scope
(switch_statement) @scope

; Definitions
(function_declaration
  name: (identifier) @definition.function)

(struct_declaration
  name: (identifier) @definition.type)

(enum_declaration
  name: (identifier) @definition.type)

(union_declaration
  name: (identifier) @definition.type)

(alias_declaration
  name: (identifier) @definition.type)

(variable_declaration
  name: (identifier) @definition.var)

(const_declaration
  name: (identifier) @definition.const)

(parameter
  name: (identifier) @definition.parameter)

(struct_field_declaration
  name: (identifier) @definition.field)

(union_field_declaration
  name: (identifier) @definition.field)

(enum_variant
  name: (identifier) @definition.enum)

(module_declaration
  name: (identifier) @definition.namespace)

(module_block_declaration
  name: (dotted_identifier) @definition.namespace)

; References
(identifier) @reference

(type_cast_expression
  type: (identifier) @reference)

(import_declaration
  path: (import_path) @reference)
