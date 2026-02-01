package ast

import (
	"fmt"
	"quark/token"
	"strings"
)

type NodeType int

const (
	CompilationUnitNode NodeType = iota
	BlockNode
	StatementNode
	ExpressionNode
	ConditionNode
	FunctionNode
	FunctionCallNode
	ArgumentsNode
	IdentifierNode
	LiteralNode
	OperatorNode
	IfStatementNode
	WhenStatementNode
	PatternNode
	ForLoopNode
	WhileLoopNode
	LambdaNode
	TernaryNode
	PipeNode
	ListNode
	DictNode
	IndexNode
	ModuleNode
	UseNode
)

var nodeTypeNames = map[NodeType]string{
	CompilationUnitNode: "CompilationUnit",
	BlockNode:           "Block",
	StatementNode:       "Statement",
	ExpressionNode:      "Expression",
	ConditionNode:       "Condition",
	FunctionNode:        "Function",
	FunctionCallNode:    "FunctionCall",
	ArgumentsNode:       "Arguments",
	IdentifierNode:      "Identifier",
	LiteralNode:         "Literal",
	OperatorNode:        "Operator",
	IfStatementNode:     "IfStatement",
	WhenStatementNode:   "WhenStatement",
	PatternNode:         "Pattern",
	ForLoopNode:         "ForLoop",
	WhileLoopNode:       "WhileLoop",
	LambdaNode:          "Lambda",
	TernaryNode:         "Ternary",
	PipeNode:            "Pipe",
	ListNode:            "List",
	DictNode:            "Dict",
	IndexNode:           "Index",
	ModuleNode:          "Module",
	UseNode:             "Use",
}

func (n NodeType) String() string {
	if name, ok := nodeTypeNames[n]; ok {
		return name
	}
	return "Unknown"
}

// Node is the interface all AST nodes implement
type Node interface {
	TokenLiteral() string
	String() string
	Type() NodeType
}

// TreeNode is the main AST node structure (mirrors Python implementation)
type TreeNode struct {
	NodeType NodeType
	Token    *token.Token
	Children []*TreeNode
}

func NewNode(nodeType NodeType, tok *token.Token) *TreeNode {
	return &TreeNode{
		NodeType: nodeType,
		Token:    tok,
		Children: make([]*TreeNode, 0),
	}
}

func (n *TreeNode) Type() NodeType {
	return n.NodeType
}

func (n *TreeNode) TokenLiteral() string {
	if n.Token != nil {
		return n.Token.Literal
	}
	return ""
}

func (n *TreeNode) String() string {
	if n.Token != nil && n.Token.Literal != "" {
		return fmt.Sprintf("%s[%s]", n.NodeType.String(), n.Token.Literal)
	}
	return n.NodeType.String()
}

func (n *TreeNode) AddChild(child *TreeNode) {
	n.Children = append(n.Children, child)
}

func (n *TreeNode) AddChildren(children ...*TreeNode) {
	n.Children = append(n.Children, children...)
}

// Print outputs the AST tree structure
func (n *TreeNode) Print(level int) {
	indent := strings.Repeat("  ", level)
	fmt.Printf("%s%s\n", indent, n.String())
	for _, child := range n.Children {
		child.Print(level + 1)
	}
}

// PrintTree is a convenience function for printing from root
func (n *TreeNode) PrintTree() {
	n.Print(0)
}

// Precedence levels for expression parsing (mirrors Python Precedence class)
type Precedence int

const (
	PrecLowest     Precedence = iota
	PrecAssignment            // =
	PrecPipe                  // |
	PrecComma                 // ,
	PrecTernary               // if-else
	PrecOr                    // or
	PrecAnd                   // and
	PrecBitwiseAnd            // &
	PrecEquality              // == !=
	PrecComparison            // < <= > >=
	PrecRange                 // ..
	PrecTerm                  // + -
	PrecFactor                // * / %
	PrecExponent              // **
	PrecUnary                 // ! ~ -
	PrecApplication           // function application (space)
	PrecAccess                // . [] ()
)

var precedenceNames = map[Precedence]string{
	PrecLowest:     "Lowest",
	PrecAssignment: "Assignment",
	PrecPipe:       "Pipe",
	PrecComma:      "Comma",
	PrecTernary:    "Ternary",
	PrecOr:         "Or",
	PrecAnd:        "And",
	PrecBitwiseAnd: "BitwiseAnd",
	PrecEquality:   "Equality",
	PrecComparison: "Comparison",
	PrecRange:      "Range",
	PrecTerm:       "Term",
	PrecFactor:     "Factor",
	PrecExponent:   "Exponent",
	PrecUnary:      "Unary",
	PrecApplication: "Application",
	PrecAccess:     "Access",
}

func (p Precedence) String() string {
	if name, ok := precedenceNames[p]; ok {
		return name
	}
	return "Unknown"
}
