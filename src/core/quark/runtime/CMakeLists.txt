cmake_minimum_required(VERSION 3.14)
project(quark_runtime_tests CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Fetch Catch2 for testing
include(FetchContent)
FetchContent_Declare(
    Catch2
    GIT_REPOSITORY https://github.com/catchorg/Catch2.git
    GIT_TAG v3.5.0
)
FetchContent_MakeAvailable(Catch2)

# Include directory for runtime headers
include_directories(include)

# Test executable
add_executable(runtime_tests
    tests/test_main.cpp
    tests/test_value.cpp
    tests/test_arithmetic.cpp
    tests/test_comparison.cpp
    tests/test_logical.cpp
    tests/test_string.cpp
    tests/test_list.cpp
    tests/test_builtins.cpp
)

target_link_libraries(runtime_tests PRIVATE Catch2::Catch2WithMain)

# Link math library on Unix
if(UNIX)
    target_link_libraries(runtime_tests PRIVATE m)
endif()

# Enable testing
include(CTest)
include(Catch)
catch_discover_tests(runtime_tests)
