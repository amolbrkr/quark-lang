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
    char *type;
    char *value;
    int lineNo;
    int pos;
};

struct TreeNode
{
    NodeType type;
    Token tok;
    std::vector<TreeNode *> children;
};

namespace py = pybind11;

void processTree(TreeNode *root)
{
    std::cout << root->type << std::endl;
}

void objTest(py::object &o)
{
    for (const py::handle &child : o.attr("children"))
    {
        std::cout << child.str() << std::endl;
    }
}

PYBIND11_MODULE(c_codegen, m)
{
    py::enum_<NodeType>(m, "NodeType")
        .value("CompilationUnit", NodeType::CompilationUnit)
        .value("Block", NodeType::Block)
        .value("Statement", NodeType::Statement)
        .value("Expression", NodeType::Expression)
        .value("Condition", NodeType::Condition)
        .value("Function", NodeType::Function)
        .value("FunctionCall", NodeType::FunctionCall)
        .value("Arguments", NodeType::Arguments)
        .value("Identifier", NodeType::Identifier)
        .value("Literal", NodeType::Literal)
        .value("Operator", NodeType::Operator)
        .export_values();

    // py::class_<std::vector<TreeNode *>>(m, "NodeList")
    //     .def(py::init<>())
    //     .def(py::vectorize_convert<std::vector<TreeNode *>, py::array_t<TreeNode *>>());

    py::class_<TreeNode>(m, "TreeNode")
        .def(py::init<>())
        .def_readwrite("type", &TreeNode::type)
        .def_readwrite("tok", &TreeNode::tok)
        .def_readwrite("children", &TreeNode::children);

    m.def("objTest", &objTest, "Test func");
    m.def("processTree", &processTree, "A function that receives a tree");
}