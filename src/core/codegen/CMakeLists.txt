cmake_minimum_required(VERSION 3.0.0)
project(c_codegen VERSION 0.1.0 LANGUAGES C CXX)

find_package(pybind11)
pybind11_add_module(c_codegen codegen.cpp)
