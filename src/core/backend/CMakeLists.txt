cmake_minimum_required(VERSION 3.13.0)

set(CMAKE_CXX_COMPILER "CLANG")
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

project(quark_backend VERSION 0.1.0 LANGUAGES CXX)

find_package(LLVM REQUIRED CONFIG)
include_directories(include)
add_library(quark_backend QuarkCodegen.cpp)
add_subdirectory(pytreetonative)
target_link_libraries(pytreetonative PUBLIC quark_backend)
