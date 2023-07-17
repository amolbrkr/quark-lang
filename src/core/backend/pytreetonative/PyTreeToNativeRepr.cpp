#include "pytreetonative.h"

PYBIND11_MODULE(pytreetonative, m)
{
    m.def("initCodegen", &PyTreeToNativeRepr::consumePyTree, "Takes in a pybind11::object tree and converts it to native C++ representation");
};

TreeNode PyTreeToNativeRepr::genNativeTreeRepr(pybind11::object tree)
{
    TreeNode node;
    node.type = static_cast<NodeType>(std::stoi(pybind11::str(tree.attr("type").attr("value"))));
    if (!tree.attr("tok").is_none())
    {
        node.tok = Token{
            pybind11::str(tree.attr("tok").attr("type")),
            pybind11::str(tree.attr("tok").attr("value")),
            std::stoi(pybind11::str(tree.attr("tok").attr("lineno"))),
            std::stoi(pybind11::str(tree.attr("tok").attr("pos"))) };
    }

    for (pybind11::handle child : tree.attr("children"))
    {
        node.children.push_back(genNativeTreeRepr(pybind11::reinterpret_borrow<pybind11::object>(child)));
    }

    return node;
};

void PyTreeToNativeRepr::consumePyTree(const pybind11::object& tree)
{
    QuarkCodegen cg;
    cg.begin(PyTreeToNativeRepr::genNativeTreeRepr(tree));
};