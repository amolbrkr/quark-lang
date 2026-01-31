#pragma once

#include <pybind11/pybind11.h>
#include <pybind11/stl.h>
#include "../include/ast.h"
#include "../include/codegen.h"

class PyTreeToNativeRepr
{
public:
	static TreeNode genNativeTreeRepr(pybind11::object tree);
	static void consumePyTree(const pybind11::object& tree);
};