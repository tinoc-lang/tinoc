package src

import _ "embed"

// RuntimeHeader is the contents of tinoc.h, the small C99 runtime header
// that defines Tinoc's primitive type aliases (u8, i32, f32, str, ...) and
// supporting helpers. It is embedded directly into the tinoc binary so
// `build`/`run` can write it out next to generated C code without
// depending on any external file being present on disk.
//
//go:embed runtime/tinoc.h
var RuntimeHeader string
