cmake_minimum_required(VERSION 3.0.0)
project(quark-codegen VERSION 0.1.0 LANGUAGES C CXX)

include(CTest)
enable_testing()

add_library(quark-codegen SHARED codegen.cpp)

set(CPACK_PROJECT_NAME ${PROJECT_NAME})
set(CPACK_PROJECT_VERSION ${PROJECT_VERSION})

find_package(PythonLibs REQUIRED)
include_directories(${PYTHON_INCLUDE_DIRS})
target_link_libraries(quark-codegen ${PYTHON_LIBRARIES})
