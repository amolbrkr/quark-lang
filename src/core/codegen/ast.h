#pragma once

#include <iostream>

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

inline std::string nodeTypeString(NodeType type) {
	std::string vals[] = {
		"CompilationUnit",
		"Block",
		"Statement",
		"Expression",
		"Condition",
		"Function",
		"FunctionCall",
		"Arguments",
		"Identifier",
		"Literal",
		"Operator",
	};
	return vals[type];
}

inline void printTree(TreeNode& root, int level = 0)
{
	for (int i = 0; i < level; i++) std::cout << "\t";
	std::cout << nodeTypeString(root.type) + "[" + root.tok.value + "]\n";
	for (TreeNode child : root.children) printTree(child, level + 1);
}