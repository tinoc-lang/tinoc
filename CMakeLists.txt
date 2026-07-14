cmake_minimum_required(VERSION 3.28)

project(tinoc
    VERSION 0.1.0
    LANGUAGES CXX
)

# Force Clang. Reject other compilers early instead of failing deep in build.
if(NOT CMAKE_CXX_COMPILER_ID STREQUAL "Clang" AND NOT CMAKE_CXX_COMPILER_ID STREQUAL "AppleClang")
    message(FATAL_ERROR "Tinoc requires Clang. Detected: ${CMAKE_CXX_COMPILER_ID}")
endif()

if(CMAKE_CXX_COMPILER_VERSION VERSION_LESS 17.0)
    message(FATAL_ERROR "Tinoc requires Clang 17 or newer. Detected: ${CMAKE_CXX_COMPILER_VERSION}")
endif()

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Debug)
endif()

set(CMAKE_EXPORT_COMPILE_COMMANDS ON)
set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)

# Warnings treated as errors. Catch bugs at compile time, not runtime.
add_library(tinoc_warnings INTERFACE)
target_compile_options(tinoc_warnings INTERFACE
    -Wall
    -Wextra
    -Wpedantic
    -Werror
    -Wshadow
    -Wconversion
    -Wsign-conversion
    -Wold-style-cast
    -Wnon-virtual-dtor
    -Wcast-align
    -Wunused
    -Woverloaded-virtual
    -Wnull-dereference
    -Wdouble-promotion
    -Wformat=2
    -Wimplicit-fallthrough
    -Wswitch-enum
)

# Hardening flags. Catch memory bugs at compile time where possible, abort loudly at runtime otherwise.
add_library(tinoc_hardening INTERFACE)
target_compile_definitions(tinoc_hardening INTERFACE
    _GLIBCXX_ASSERTIONS
    $<$<CONFIG:Release>:_FORTIFY_SOURCE=2>
)
target_compile_options(tinoc_hardening INTERFACE
    -fstack-protector-strong
    -fstack-clash-protection
)

# Sanitizers for debug builds. Catches UB, leaks, overflows before they ship.
option(TINOC_ENABLE_SANITIZERS "Enable ASan/UBSan in debug builds" ON)

add_library(tinoc_sanitizers INTERFACE)
if(TINOC_ENABLE_SANITIZERS AND CMAKE_BUILD_TYPE STREQUAL "Debug")
    target_compile_options(tinoc_sanitizers INTERFACE
        -fsanitize=address,undefined
        -fno-sanitize-recover=all
        -fno-omit-frame-pointer
    )
    target_link_options(tinoc_sanitizers INTERFACE
        -fsanitize=address,undefined
    )
endif()

# Build type specific flags
set(CMAKE_CXX_FLAGS_DEBUG "-Og -g3")
set(CMAKE_CXX_FLAGS_RELEASE "-O2 -DNDEBUG")
set(CMAKE_CXX_FLAGS_RELWITHDEBINFO "-O2 -g -DNDEBUG")

option(TINOC_BUILD_TESTS "Build unit tests" ON)

add_subdirectory(src)

if(TINOC_BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()
