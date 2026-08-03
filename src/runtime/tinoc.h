#ifndef TINOC_H
#define TINOC_H

#include <stdint.h>
#include <stdbool.h>
#include <stddef.h>
#include <limits.h>
#include <math.h>

/* Primitive Integer Types */
typedef uint8_t   u8;
typedef uint16_t  u16;
typedef uint32_t  u32;
typedef uint64_t  u64;
typedef size_t    usize;

typedef int8_t    i8;
typedef int16_t   i16;
typedef int32_t   i32;
typedef int64_t   i64;
typedef ptrdiff_t isize;

#ifdef __SIZEOF_INT128__
typedef __uint128_t u128;
typedef __int128_t  i128;
#endif

/* Floating-Point & Character Types */
typedef float     f32;
typedef double    f64;

#if defined(__SIZEOF_FLOAT128__) || defined(__FLOAT128__)
typedef __float128 f128;
#else
typedef long double f128;
#endif

typedef uint32_t  char32;

/* String Type */
typedef struct {
    const char* data;
    size_t len;
} str;

static inline str tinoc_str_lit(const char* s, size_t len) {
    str result = { s, len };
    return result;
}

/* Slice Macro & Typedefs */
#define tinoc_slice(T) struct { T* ptr; size_t len; }

typedef tinoc_slice(u8)   slice_u8;
typedef tinoc_slice(i32)  slice_i32;
typedef tinoc_slice(f32)  slice_f32;
typedef tinoc_slice(str)  slice_str;

/* Optional Type Generators (Named Typedefs for C Compatibility) */
#define TINOC_DEFINE_OPTIONAL(T) \
    typedef struct { T value; bool has_value; } opt_##T

#define tinoc_optional(T) opt_##T
#define tinoc_null(T) ((opt_##T){ .has_value = false })
#define tinoc_some(T, val) ((opt_##T){ .value = (val), .has_value = true })

/* Pre-defined Common Optionals */
TINOC_DEFINE_OPTIONAL(u8);
TINOC_DEFINE_OPTIONAL(i32);
TINOC_DEFINE_OPTIONAL(f32);
TINOC_DEFINE_OPTIONAL(str);

/* Error Union Generators (Named Typedefs for C Compatibility) */
#define TINOC_DEFINE_RESULT(T, E) \
    typedef struct { T value; E err; bool is_err; } res_##T_##E

#define tinoc_result(T, E) res_##T_##E
#define tinoc_ok(T, E, val) ((res_##T_##E){ .value = (val), .is_err = false })
#define tinoc_err(T, E, error) ((res_##T_##E){ .err = (error), .is_err = true })

/* Wrapping Arithmetic Operators (+%, -%, *%) */
#define tinoc_wrap_add(a, b) ((__typeof__(a))((__typeof__(a))(a) + (__typeof__(a))(b)))
#define tinoc_wrap_sub(a, b) ((__typeof__(a))((__typeof__(a))(a) - (__typeof__(a))(b)))
#define tinoc_wrap_mul(a, b) ((__typeof__(a))((__typeof__(a))(a) * (__typeof__(a))(b)))

/* Saturating Addition (+|) */
static inline u8 tinoc_sat_add_u8(u8 a, u8 b) {
    u8 res;
    return __builtin_add_overflow(a, b, &res) ? UINT8_MAX : res;
}

static inline u16 tinoc_sat_add_u16(u16 a, u16 b) {
    u16 res;
    return __builtin_add_overflow(a, b, &res) ? UINT16_MAX : res;
}

static inline u32 tinoc_sat_add_u32(u32 a, u32 b) {
    u32 res;
    return __builtin_add_overflow(a, b, &res) ? UINT32_MAX : res;
}

static inline u64 tinoc_sat_add_u64(u64 a, u64 b) {
    u64 res;
    return __builtin_add_overflow(a, b, &res) ? UINT64_MAX : res;
}

static inline i8 tinoc_sat_add_i8(i8 a, i8 b) {
    i8 res;
    if (__builtin_add_overflow(a, b, &res)) return (b > 0) ? INT8_MAX : INT8_MIN;
    return res;
}

static inline i16 tinoc_sat_add_i16(i16 a, i16 b) {
    i16 res;
    if (__builtin_add_overflow(a, b, &res)) return (b > 0) ? INT16_MAX : INT16_MIN;
    return res;
}

static inline i32 tinoc_sat_add_i32(i32 a, i32 b) {
    i32 res;
    if (__builtin_add_overflow(a, b, &res)) return (b > 0) ? INT32_MAX : INT32_MIN;
    return res;
}

static inline i64 tinoc_sat_add_i64(i64 a, i64 b) {
    i64 res;
    if (__builtin_add_overflow(a, b, &res)) return (b > 0) ? INT64_MAX : INT64_MIN;
    return res;
}

/* Saturating Subtraction (-|) */
static inline u8 tinoc_sat_sub_u8(u8 a, u8 b) {
    u8 res;
    return __builtin_sub_overflow(a, b, &res) ? 0 : res;
}

static inline u16 tinoc_sat_sub_u16(u16 a, u16 b) {
    u16 res;
    return __builtin_sub_overflow(a, b, &res) ? 0 : res;
}

static inline u32 tinoc_sat_sub_u32(u32 a, u32 b) {
    u32 res;
    return __builtin_sub_overflow(a, b, &res) ? 0 : res;
}

static inline u64 tinoc_sat_sub_u64(u64 a, u64 b) {
    u64 res;
    return __builtin_sub_overflow(a, b, &res) ? 0 : res;
}

static inline i8 tinoc_sat_sub_i8(i8 a, i8 b) {
    i8 res;
    if (__builtin_sub_overflow(a, b, &res)) return (b < 0) ? INT8_MAX : INT8_MIN;
    return res;
}

static inline i16 tinoc_sat_sub_i16(i16 a, i16 b) {
    i16 res;
    if (__builtin_sub_overflow(a, b, &res)) return (b < 0) ? INT16_MAX : INT16_MIN;
    return res;
}

static inline i32 tinoc_sat_sub_i32(i32 a, i32 b) {
    i32 res;
    if (__builtin_sub_overflow(a, b, &res)) return (b < 0) ? INT32_MAX : INT32_MIN;
    return res;
}

static inline i64 tinoc_sat_sub_i64(i64 a, i64 b) {
    i64 res;
    if (__builtin_sub_overflow(a, b, &res)) return (b < 0) ? INT64_MAX : INT64_MIN;
    return res;
}

/* Generic Saturating Dispatch Macros (C11) */
#if defined(__STDC_VERSION__) && __STDC_VERSION__ >= 201112L
#define tinoc_sat_add(a, b) _Generic((a), \
    u8:  tinoc_sat_add_u8(a, b),   i8:  tinoc_sat_add_i8(a, b),  \
    u16: tinoc_sat_add_u16(a, b),  i16: tinoc_sat_add_i16(a, b), \
    u32: tinoc_sat_add_u32(a, b),  i32: tinoc_sat_add_i32(a, b), \
    u64: tinoc_sat_add_u64(a, b),  i64: tinoc_sat_add_i64(a, b)  \
)

#define tinoc_sat_sub(a, b) _Generic((a), \
    u8:  tinoc_sat_sub_u8(a, b),   i8:  tinoc_sat_sub_i8(a, b),  \
    u16: tinoc_sat_sub_u16(a, b),  i16: tinoc_sat_sub_i16(a, b), \
    u32: tinoc_sat_sub_u32(a, b),  i32: tinoc_sat_sub_i32(a, b), \
    u64: tinoc_sat_sub_u64(a, b),  i64: tinoc_sat_sub_i64(a, b)  \
)
#endif

#endif
