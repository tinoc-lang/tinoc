#include <stdio.h>
#include <assert.h>
#include "tinoc.h"

typedef enum { ERR_NONE = 0, ERR_NOT_FOUND = 404 } MyError;

/* Define the result struct for (i32, MyError) */
TINOC_DEFINE_RESULT(i32, MyError);

void test_primitives_and_string(void) {
    u8  a = 255;
    i32 b = -42;
    f32 c = 3.14159f;
    
    assert(a == 255);
    assert(b == -42);

    str s = tinoc_str_lit("Hello, Tinoc!", 13);
    assert(s.len == 13);
    
    printf("[PASS] Primitives & Strings (%.*s)\n", (int)s.len, s.data);
}

void test_optionals(void) {
    tinoc_optional(i32) some_val = tinoc_some(i32, 100);
    assert(some_val.has_value == true);
    assert(some_val.value == 100);

    tinoc_optional(i32) null_val = tinoc_null(i32);
    assert(null_val.has_value == false);

    printf("[PASS] Optionals\n");
}

void test_error_unions(void) {
    tinoc_result(i32, MyError) ok_res = tinoc_ok(i32, MyError, 200);
    assert(ok_res.is_err == false);
    assert(ok_res.value == 200);

    tinoc_result(i32, MyError) err_res = tinoc_err(i32, MyError, ERR_NOT_FOUND);
    assert(err_res.is_err == true);
    assert(err_res.err == ERR_NOT_FOUND);

    printf("[PASS] Error Unions\n");
}

void test_slices(void) {
    i32 raw_arr[] = { 10, 20, 30, 40 };
    slice_i32 slice = { .ptr = raw_arr, .len = 4 };

    assert(slice.len == 4);
    assert(slice.ptr[0] == 10);
    assert(slice.ptr[3] == 40);

    printf("[PASS] Slices\n");
}

void test_wrapping_arithmetic(void) {
    u8 x = 250;
    u8 y = 10;

    u8 wrapped_add = tinoc_wrap_add(x, y);
    assert(wrapped_add == 4);

    u8 a = 5;
    u8 b = 10;
    u8 wrapped_sub = tinoc_wrap_sub(a, b);
    assert(wrapped_sub == 251);

    printf("[PASS] Wrapping Arithmetic (+%%, -%%)\n");
}

void test_saturating_arithmetic(void) {
    u8 u1 = 250;
    u8 u2 = 10;
    u8 sat_u8_add = tinoc_sat_add_u8(u1, u2);
    assert(sat_u8_add == 255);

    u8 sat_u8_sub = tinoc_sat_sub_u8(5, 10);
    assert(sat_u8_sub == 0);

    i32 i1 = INT32_MAX - 5;
    i32 i2 = 100;
    i32 sat_i32_add = tinoc_sat_add_i32(i1, i2);
    assert(sat_i32_add == INT32_MAX);

#if defined(__STDC_VERSION__) && __STDC_VERSION__ >= 201112L
    u8 gen_u8_add = tinoc_sat_add(u1, u2);
    assert(gen_u8_add == 255);

    u8 gen_u8_sub = tinoc_sat_sub((u8)5, (u8)10);
    assert(gen_u8_sub == 0);
#endif

    printf("[PASS] Saturating Arithmetic (+|, -|)\n");
}

int main(void) {
    printf("=== Running Tinoc.h Test Suite ===\n");

    test_primitives_and_string();
    test_optionals();
    test_error_unions();
    test_slices();
    test_wrapping_arithmetic();
    test_saturating_arithmetic();

    printf("===================================\n");
    printf("All Tinoc.h tests passed successfully!\n");
    return 0;
}
