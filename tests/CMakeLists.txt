find_package(GTest QUIET)

if(NOT GTest_FOUND)
    include(FetchContent)
    FetchContent_Declare(
        googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG v1.15.2
    )
    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)
endif()

# Add compiler/transpiler unit source files here once split out of main.cpp.
set(TINOC_TEST_SOURCES
    test_main.cpp
)

add_executable(tinoc_tests ${TINOC_TEST_SOURCES})

target_include_directories(tinoc_tests PRIVATE
    ${CMAKE_SOURCE_DIR}/src
)

target_link_libraries(tinoc_tests PRIVATE
    tinoc_warnings
    tinoc_hardening
    tinoc_sanitizers
    GTest::gtest_main
)

include(GoogleTest)
gtest_discover_tests(tinoc_tests)
