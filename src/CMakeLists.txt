# Add new files to this list as they're created. No globbing —
# CMake must see every file explicitly to reconfigure correctly.
set(TINOC_SOURCES
    main.cpp
    # lexer.cpp
    # parser.cpp
    # ast.cpp
    # codegen.cpp
)

add_executable(tinoc ${TINOC_SOURCES})

target_include_directories(tinoc PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}
)

target_link_libraries(tinoc PRIVATE
    tinoc_warnings
    tinoc_hardening
    tinoc_sanitizers
)
