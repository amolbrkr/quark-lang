cmake_minimum_required(VERSION 3.13.0)

project(pytreetonative VERSION 0.1.0 LANGUAGES CXX)

list(APPEND CMAKE_PREFIX_PATH C:/Users/Amol/AppData/Roaming/Python/Python310/site-packages/pybind11/share/cmake/pybind11)

find_package(pybind11 REQUIRED)
pybind11_add_module(pytreetonative PyTreeToNativeRepr.cpp)