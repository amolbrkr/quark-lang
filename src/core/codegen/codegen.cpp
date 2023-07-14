#include <iostream>
#include <pybind11/pybind11.h>
#include <pybind11/stl.h>

enum NodeType
{
	CompilationUnit,
	Block,
	Statement,
	Expression,
	Condition,
	Function,
	FunctionCall,
	Arguments,
	Identifier,
	Literal,
	Operator,
};

struct Token
{
	std::string type;
	std::string value;
	int lineNo;
	int pos;
};

struct TreeNode
{
	NodeType type;
	Token tok;
	std::vector<TreeNode> children;
};

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

	for (auto child : tree.attr("children")) {
		node.children.push_back(genNativeTreeRepr(py::reinterpret_borrow<py::object>(child)));
	}

	return node;
}

void printTree(TreeNode root, int level) {
	for (int i = 0; i < level; i++) std::cout << "\t";
	std::cout << root.type << '\n';
	for (TreeNode child : root.children) printTree(child, level + 1);
}

void consumePyTree(py::object& tree) {
	printTree(genNativeTreeRepr(tree), 0);
}

PYBIND11_MODULE(quark_codegen, m)
{
	m.def("initCodegen", &consumePyTree, "This funtion takes in a py::object tree and converts it to native C++ tree structure");
}