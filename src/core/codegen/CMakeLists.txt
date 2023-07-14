cmake_minimum_required(VERSION 3.0.0)

SET (CMAKE_C_COMPILER "MSVC")
SET (CMAKE_CXX_COMPILER "MSVC")

project(quark_codegen VERSION 0.1.0 LANGUAGES C CXX)

find_package(pybind11 REQUIRED)
pybind11_add_module(quark_codegen codegen.cpp)
