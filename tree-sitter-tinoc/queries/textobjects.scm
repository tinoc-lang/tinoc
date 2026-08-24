; Textobject queries for Helix (v-select / expand-selection support)

(function_declaration
  body: (block) @function.inside) @function.around

(struct_declaration
  body: (struct_body) @class.inside) @class.around

(enum_declaration
  body: (enum_body) @class.inside) @class.around

(union_declaration
  body: (union_body) @class.inside) @class.around

(struct_field_declaration) @field.inside

(parameter) @parameter.inside

(argument_list
  (_) @parameter.inside)

(line_comment) @comment.inside

(block_comment) @comment.inside
