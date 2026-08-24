/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

"use strict";

module.exports = grammar({
  name: "tinoc",

  extras: ($) => [/\s+/, $.line_comment, $.block_comment],

  conflicts: ($) => [
    // type vs expression overlap
    [$._type, $._expression],

    // deref ^ vs binary XOR ^
    [$.deref_expression, $.binary_expression],
    // range .. vs binary ops / postfix deref
    [$.range_expression, $.binary_expression, $.deref_expression],
    // unary vs deref vs binary
    [$.unary_expression, $.deref_expression, $.binary_expression],
    // address-of & vs deref ^ vs binary
    [$.address_of_expression, $.deref_expression, $.binary_expression],
    // type_list (trailing comma ambiguity)
    [$.type_list],
    // generic_type_args vs parenthesized_type
    [$.generic_type_args, $.parenthesized_type],
    // extern params: C-style vs native Tinoc type
    [$.c_type, $._type],
    // generic call `foo:T(...)` vs generic type followed by parenthesized expr
    [$.generic_type, $._expression],
    [$._type, $.generic_type],
    [$.generic_type, $.parenthesized_type],
  ],

  rules: {
    source_file: ($) => repeat($._top_level_item),

    _top_level_item: ($) =>
      choice(
        $.module_declaration,
        $.module_block_declaration,
        $.import_declaration,
        $.importc_declaration,
        $.run_declaration,
        $.partial_declaration,
        $.function_declaration,
        $.struct_declaration,
        $.enum_declaration,
        $.union_declaration,
        $.variable_declaration,
        $.const_declaration,
        $.alias_declaration,
        $.extern_fn_declaration,
        $.test_declaration,
      ),

    // ========================================================================
    // Comments
    // ========================================================================

    line_comment: (_) => token(seq("//", /[^\n]*/)),
    block_comment: (_) =>
      token(seq("/*", /[^*]*\*+([^/*][^*]*\*+)*/, "*/")),

    // ========================================================================
    // Identifiers
    // ========================================================================

    identifier: ($) => /[a-zA-Z_][a-zA-Z0-9_]*/,

    // ========================================================================
    // Module System — all end with ;
    // ========================================================================

    module_declaration: ($) =>
      prec.right(seq("module", field("name", $.identifier), ";")),

    module_block_declaration: ($) =>
      seq(
        "module",
        field("name", $.dotted_identifier),
        field("body", seq("{", repeat($._top_level_item), "}")),
      ),

    import_declaration: ($) =>
      seq(
        "#import",
        field(
          "path",
          choice(
            $.import_path,
            $.import_wildcard,
            $.import_list,
            $.import_string,
          ),
        ),
        optional(seq("as", field("alias", $.identifier))),
        ";",
      ),

    import_path: ($) =>
      prec.left(1, seq(
        $.identifier,
        repeat(seq(".", $.identifier)),
      )),

    dotted_identifier: ($) =>
      prec.left(1, seq(
        $.identifier,
        repeat(seq(".", $.identifier)),
      )),

    import_wildcard: ($) => prec(2, seq(
      $.identifier,
      repeat(seq(".", $.identifier)),
      ".*",
    )),

    import_list: ($) => seq(
      $.identifier,
      repeat(seq(".", $.identifier)),
      ".",
      "{",
      commaSep(
        seq($.identifier, optional(seq("as", $.identifier))),
      ),
      "}",
    ),

    import_string: ($) =>
      prec.right(seq($.string, optional(seq("as", $.identifier)))),

    importc_declaration: ($) =>
      prec.right(
        seq(
          "#importc",
          repeat1($.string),
          optional(seq("as", field("alias", $.identifier))),
          ";",
        ),
      ),

    run_declaration: ($) =>
      prec.right(seq("#run", $._expression, ";")),

    partial_declaration: (_) => prec.right(seq("#partial", ";")),

    // ========================================================================
    // Functions
    // ========================================================================

    function_declaration: ($) =>
      prec.right(
        seq(
          optional("pub"),
          optional("static"),
          "fn",
          field("name", $.identifier),
          optional($.generic_type_args),
          field("parameters", $.parameter_list),
          optional(field("return_type", $._type)),
          choice(field("body", $.block), ";"),
        ),
      ),

    extern_fn_declaration: ($) =>
      prec.right(
        seq(
          "extern",
          '"C"',
          "fn",
          field("name", $.identifier),
          optional(seq(".", $.field_name)),
          field("parameters", $.extern_parameter_list),
          optional(field("return_type", $._type)),
          ";",
        ),
      ),

    extern_parameter_list: ($) =>
      seq(
        "(",
        commaSep1($.extern_parameter),
        optional(seq(",", "...")),
        ")",
      ),

    // Extern declarations accept C-style types (`fmt *const char`) or
    // native Tinoc types (`fmt ^char`)
    extern_parameter: ($) =>
      prec.left(seq(
        field("name", $.identifier),
        field("type", alias(choice($.c_type, $._type), $.extern_type)),
      )),

    c_type: ($) =>
      prec.right(seq(
        repeat("*"),
        repeat(choice("const", "volatile", "signed", "unsigned", "struct", "enum")),
        field("base", $.identifier),
        repeat(choice(
          "*",
          "const",
          "volatile",
          "signed",
          "unsigned",
          "struct",
          "enum",
        )),
      )),

    test_declaration: ($) =>
      seq("test", field("name", $.string), field("body", $.block)),

    parameter_list: ($) => seq("(", optional(commaSep($.parameter)), ")"),

    parameter: ($) =>
      prec.left(
        seq(field("name", $.identifier), field("type", $._type)),
      ),

    // ========================================================================
    // Generic Type Arguments
    // ========================================================================

    generic_type_args: ($) =>
      seq(
        ":",
        choice($.type_list, seq("(", commaSep1($._type), ")")),
      ),

    type_list: ($) => commaSep1($._type),

    // ========================================================================
    // Types
    // ========================================================================

    _type: ($) =>
      choice(
        $.primitive_type,
        $.pointer_type,
        $.optional_type,
        $.error_union_type,
        $.array_type,
        $.slice_type,
        $.generic_type,
        $.identifier,
        $.parenthesized_type,
      ),

    primitive_type: ($) =>
      token(
        choice(
          "u8", "u16", "u32", "u64", "u128", "usize",
          "i8", "i16", "i32", "i64", "i128", "isize",
          "f32", "f64", "f128",
          "bool", "char", "void", "str",
        ),
      ),

    pointer_type: ($) => prec(2, seq("^", $._type)),

    optional_type: ($) => prec(2, seq("?", $._type)),

    // Prefix form: !T (error inferred)
    // Infix form: T1!T2 (documented return type, e.g. `f32!string`)
    // Binds tighter than pointer/array/optional constructors
    error_union_type: ($) =>
      prec.right(3, choice(
        seq("!", $._type),
        seq($._type, "!", $._type),
      )),

    array_type: ($) =>
      prec(1, seq(
        "[",
        choice($.integer, "_", seq($.integer, ":", $.integer)),
        "]",
        $._type,
      )),

    slice_type: ($) => prec(2, seq("[]", $._type)),

    generic_type: ($) =>
      seq(
        field("name", $.identifier),
        ":",
        field("type_args", choice(
          $.type_list,
          seq("(", commaSep1($._type), ")"),
        )),
      ),

    parenthesized_type: ($) => seq("(", $._type, ")"),

    // ========================================================================
    // Structs
    // ========================================================================

    struct_declaration: ($) =>
      seq(
        optional("pub"),
        "struct",
        field("name", $.identifier),
        optional($.generic_type_args),
        field("body", $.struct_body),
      ),

    struct_body: ($) =>
      seq(
        "{",
        repeat(
          choice(
            $.struct_field_declaration,
            $.function_declaration,
            $.const_declaration,
            $.variable_declaration,
          ),
        ),
        "}",
      ),

    struct_field_declaration: ($) =>
      seq(field("name", $.identifier), field("type", $._type), ";"),

    // ========================================================================
    // Enums
    // ========================================================================

    enum_declaration: ($) =>
      seq(
        optional("pub"),
        "enum",
        field("name", $.identifier),
        optional($.generic_type_args),
        field("body", $.enum_body),
      ),

    enum_body: ($) =>
      seq(
        "{",
        commaSep(
          choice(
            $.enum_variant,
            $.function_declaration,
            $.const_declaration,
          ),
        ),
        optional(","),
        "}",
      ),

    enum_variant: ($) =>
      seq(
        field("name", $.identifier),
        optional(field("payload", $.enum_payload)),
      ),

    enum_payload: ($) => seq("(", commaSep1($._type), ")"),

    // ========================================================================
    // Unions
    // ========================================================================

    union_declaration: ($) =>
      seq(
        optional("pub"),
        "union",
        field("name", $.identifier),
        optional($.generic_type_args),
        field("body", $.union_body),
      ),

    union_body: ($) =>
      seq(
        "{",
        repeat(
          choice(
            $.union_field_declaration,
            $.function_declaration,
            $.const_declaration,
          ),
        ),
        "}",
      ),

    union_field_declaration: ($) =>
      prec.right(
        seq(field("name", $.identifier), field("type", $._type), ";"),
      ),

    // ========================================================================
    // Variables & Constants — all end with ;
    // ========================================================================

    variable_declaration: ($) =>
      seq(
        "var",
        field("name", $.identifier),
        optional(field("type", $._type)),
        optional(seq("=", field("value", $._expression))),
        ";",
      ),

    const_declaration: ($) =>
      prec.right(
        seq(
          optional("pub"),
          "const",
          field("name", $.identifier),
          optional(field("type", $._type)),
          optional(seq("=", field("value", $._expression))),
          ";",
        ),
      ),

    // ========================================================================
    // Alias — ends with ;
    // ========================================================================

    alias_declaration: ($) =>
      seq(
        optional("pub"),
        "alias",
        field("name", $.identifier),
        optional($.generic_type_args),
        "=",
        field("type", $._type),
        ";",
      ),

    // ========================================================================
    // Block & Statements — semicolons required
    // ========================================================================

    block: ($) => seq("{", repeat($._statement), "}"),

    _statement: ($) =>
      prec(1, choice(
        $.variable_declaration,
        $.const_declaration,
        $.return_statement,
        $.break_statement,
        $.continue_statement,
        $.if_statement,
        $.while_statement,
        $.for_statement,
        $.switch_statement,
        $.expression_statement,
      )),

    expression_statement: ($) =>
      prec(1, seq($._expression, ";")),

    return_statement: ($) =>
      prec.right(seq("return", optional($._expression), ";")),

    break_statement: (_) => prec.right(seq("break", ";")),

    continue_statement: (_) => prec.right(seq("continue", ";")),

    // ========================================================================
    // Control Flow
    // ========================================================================

    if_statement: ($) =>
      prec.right(seq(
        "if",
        field("condition", $._expression),
        field("consequence", $.block),
        optional(seq(
          "else",
          field("alternative", choice($.block, $.if_statement)),
        )),
      )),

    while_statement: ($) =>
      seq(
        "while",
        field("condition", $._expression),
        field("body", $.block),
      ),

    // High precedence so the capture pipes `|i|` are not eaten by binary `|`
    for_statement: ($) =>
      prec(8, seq(
        "for",
        field("iterable", choice($.range_expression, $._expression)),
        "|",
        field("variable", $.identifier),
        "|",
        field("body", $.block),
      )),

    switch_statement: ($) =>
      prec.right(seq(
        "switch",
        field("value", $._expression),
        "{",
        repeat($.switch_arm),
        "}",
      )),

    switch_arm: ($) =>
      prec(-1, seq(
        field("pattern", choice($._expression, "_")),
        "=>",
        field("body", $.block),
        optional(","),
      )),

    // ========================================================================
    // Expressions
    // ========================================================================

    _expression: ($) =>
      choice(
        $.identifier,
        $.primitive_type,
        $.integer,
        $.float,
        $.string,
        $.char_literal,
        $.true,
        $.false,
        $.null,
        $.unary_expression,
        $.binary_expression,
        $.call_expression,
        $.index_expression,
        $.member_expression,
        $.deref_expression,
        $.parenthesized_expression,
        $.struct_literal,
        $.array_literal,
        $.optional_unwrap,
        $.type_cast_expression,
        $.address_of_expression,
        $.catch_expression,
        $.range_expression,
      ),

    true: (_) => token("true"),
    false: (_) => token("false"),
    null: (_) => token("null"),

    unary_expression: ($) =>
      prec(15, seq(
        field("operator", choice("-", "-%", "-|", "!", "~")),
        field("argument", $._expression),
      )),

    binary_expression: ($) => {
      // Precedence from syntax.md (highest to lowest):
      //   * / % ** *% *|
      //   ||
      //   + - ++ +% -% +| -|
      //   << >> <<|
      //   &
      //   ^
      //   |
      //   orelse catch
      //   == != < > <= >=
      //   and or
      //   = *= **= *%= *|= /= %= += +%= +|= -= -%= -|= <<= <<|= >>= &= ^= |=
      const table = [
        ["right", 1,  "="],
        ["right", 2,  "**=", "*%=", "*|=", ">>=", "<<=", "<<|=",
                       "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=",
                       "+%=", "+|=", "-%=", "-|="],
        ["left",  3,  "or"],
        ["left",  4,  "and"],
        ["left",  5,  "==", "!=", "<", ">", "<=", ">="],
        ["left",  6,  "orelse"],
        ["left",  7,  "|"],
        ["left",  8,  "^"],
        ["left",  9,  "&"],
        ["left",  10, "<<", ">>", "<<|"],
        ["left",  11, "+", "-", "++", "+%", "-%", "+|", "-|"],
        ["left",  12, "||"],
        ["left",  13, "*", "/", "%", "**", "*%", "*|"],
      ];

      return choice(
        ...table.map(([assoc, prec_level, ...ops]) => {
          const rule = seq(
            field("left", $._expression),
            field("operator", ops.length === 1 ? ops[0] : choice(...ops)),
            field("right", $._expression),
          );
          return prec[assoc](prec_level, rule);
        }),
      );
    },

    // `a catch b` / `a catch |err| b` — defaulting error unwrap
    catch_expression: ($) =>
      prec.left(6, seq(
        field("value", $._expression),
        "catch",
        optional(seq(
          "|",
          field("error", $.identifier),
          "|",
        )),
        field("fallback", $._expression),
      )),

    call_expression: ($) =>
      prec(16, seq(
        field("function", $._expression),
        optional(seq(":", $.type_list)),
        field("arguments", $.argument_list),
      )),

    argument_list: ($) => seq("(", optional(commaSep($._expression)), ")"),

    index_expression: ($) =>
      prec(16, seq($._expression, "[", $._expression, "]")),

    member_expression: ($) =>
      prec.left(17, seq(
        field("object", $._expression),
        ".",
        field("property", $.identifier),
      )),

    deref_expression: ($) => prec(16, seq($._expression, "^")),

    address_of_expression: ($) => prec(15, seq("&", $._expression)),

    parenthesized_expression: ($) => seq("(", $._expression, ")"),

    struct_literal: ($) =>
      prec(1, seq(
        field("type", $._type),
        "{",
        optional(commaSep($.struct_literal_field)),
        "}",
      )),

    struct_literal_field: ($) =>
      prec.right(seq(
        ".",
        field("name", $.identifier),
        "=",
        field("value", $._expression),
      )),

    array_literal: ($) =>
      seq("[", optional(commaSep($._expression)), "]"),

    optional_unwrap: ($) => prec(16, seq($._expression, "?")),

    type_cast_expression: ($) =>
      prec(16, seq($._expression, "as", field("type", $._type))),

    // Above binary `|` (7) so the capture pipes are not eaten by bitwise-or
    range_expression: ($) =>
      prec.left(9, seq(
        field("start", $._expression),
        "..",
        field("end", $._expression),
      )),

    // ========================================================================
    // Literals
    // ========================================================================

    integer: (_) =>
      token(choice(
        /0[xX][0-9a-fA-F][0-9a-fA-F_]*/,
        /0[oO][0-7][0-7_]*/,
        /0[bB][01][01_]*/,
        /0|[1-9][0-9_]*/,
      )),

    float: (_) =>
      token(choice(
        /[0-9][0-9_]*\.[0-9][0-9_]*[eE][+-]?[0-9][0-9_]*/,
        /[0-9][0-9_]*\.[0-9][0-9_]*/,
        /[0-9][0-9_]*[eE][+-]?[0-9][0-9_]*/,
        /0[xX][0-9a-fA-F][0-9a-fA-F_]*\.[0-9a-fA-F][0-9a-fA-F_]*[pP][+-]?[0-9][0-9_]*/,
        /0[xX][0-9a-fA-F][0-9a-fA-F_]*\.[0-9a-fA-F][0-9a-fA-F_]*/,
        /0[xX][0-9a-fA-F][0-9a-fA-F_]*[pP][+-]?[0-9][0-9_]*/,
      )),

    string: (_) =>
      token(seq(
        '"',
        repeat(choice(
          /[^"\\\n]/,
          /\\[\\abfnrtv0'"]/,
          /\\x[0-9a-fA-F]{2}/,
          /\\u\{[0-9a-fA-F]+\}/,
        )),
        '"',
      )),

    char_literal: (_) =>
      token(seq(
        "'",
        choice(
          /[^'\\\n]/,
          /\\[\\abfnrtv0'"]/,
          /\\x[0-9a-fA-F]{2}/,
          /\\u\{[0-9a-fA-F]+\}/,
        ),
        "'",
      )),

    // ========================================================================
    // Helpers
    // ========================================================================

    field_name: ($) => $.identifier,
  },
});

function commaSep(rule) {
  return seq(rule, repeat(seq(",", rule)), optional(","));
}

function commaSep1(rule) {
  return seq(rule, repeat(seq(",", rule)));
}
