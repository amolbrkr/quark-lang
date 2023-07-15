#include <pybind11/pybind11.h>
#include <pybind11/stl.h>
#include "ast.h"

namespace py = pybind11;

TreeNode genNativeTreeRepr(py::object& tree)
{
	TreeNode node;
	node.type = static_cast<NodeType>(std::stoi(tree.attr("type").attr("value").str()));
	if (!tree.attr("tok").is_none()) {
		node.tok = Token{
			tree.attr("tok").attr("type").str(),
			tree.attr("tok").attr("value").str(),
			std::stoi(tree.attr("tok").attr("lineno").str()),
			std::stoi(tree.attr("tok").attr("pos").str())
		};
	}

	for (py::handle child : tree.attr("children")) {
		node.children.push_back(genNativeTreeRepr(py::reinterpret_borrow<py::object>(child)));
	}

	return node;
}

void consumePyTree(py::object& tree) {
	printTree(genNativeTreeRepr(tree));
}

PYBIND11_MODULE(quark_codegen, m)
{
	m.def("initCodegen", &consumePyTree, "This funtion takes in a py::object tree and converts it to native C++ tree structure");
}