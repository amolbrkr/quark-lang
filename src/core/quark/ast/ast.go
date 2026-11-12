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
	VectorNode
	DictNode
	IndexNode
	ResultNode
	ResultPatternNode
	ParameterNode
	TypeNode
	VarDeclNode
	ModuleNode
	InclNode
	BreakNode
	ContinueNode
	ExternHeaderNode      // extern 'path/to/glue.hpp'
	ExternFnNode          // extern fn name(...) type as 'symbol'
	StructDefNode         // struct Name: fields...
	StructFieldNode       // field: Type [= default]
	StructLiteralNode     // Name { field: expr, ... }
	ExternLinkNode        // extern link 'name' (link a native library)
	ExternIncludePathNode // extern include_path 'dir' (C++ include search path)
	// QualifiedNameNode is `a::b` or `a::b::c` in expression position.
	// Token.Literal holds the whole path ("io::exists"); the loader resolves
	// it and turns the node into an IdentifierNode with the internal name.
	QualifiedNameNode
)

var nodeTypeNames = map[NodeType]string{
	CompilationUnitNode:   "CompilationUnit",
	BlockNode:             "Block",
	StatementNode:         "Statement",
	ExpressionNode:        "Expression",
	ConditionNode:         "Condition",
	FunctionNode:          "Function",
	FunctionCallNode:      "FunctionCall",
	ArgumentsNode:         "Arguments",
	IdentifierNode:        "Identifier",
	LiteralNode:           "Literal",
	OperatorNode:          "Operator",
	IfStatementNode:       "IfStatement",
	WhenStatementNode:     "WhenStatement",
	PatternNode:           "Pattern",
	ForLoopNode:           "ForLoop",
	WhileLoopNode:         "WhileLoop",
	LambdaNode:            "Lambda",
	TernaryNode:           "Ternary",
	PipeNode:              "Pipe",
	ListNode:              "List",
	VectorNode:            "Vector",
	DictNode:              "Dict",
	IndexNode:             "Index",
	ResultNode:            "Result",
	ResultPatternNode:     "ResultPattern",
	ParameterNode:         "Parameter",
	TypeNode:              "Type",
	VarDeclNode:           "VarDecl",
	ModuleNode:            "Module",
	InclNode:              "Incl",
	BreakNode:             "Break",
	ContinueNode:          "Continue",
	ExternHeaderNode:      "ExternHeader",
	ExternFnNode:          "ExternFn",
	StructDefNode:         "StructDef",
	StructFieldNode:       "StructField",
	StructLiteralNode:     "StructLiteral",
	ExternLinkNode:        "ExternLink",
	ExternIncludePathNode: "ExternIncludePath",
	QualifiedNameNode:     "QualifiedName",
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
	NodeType     NodeType
	Token        *token.Token
	Children     []*TreeNode
	ReturnType   *TreeNode // For LambdaNode: optional return type annotation (TypeNode)
	DefaultValue *TreeNode // For ParameterNode: optional default value expression

	// ExternFnNode fields:
	// Token.Literal = function name (e.g. "sqrt" or "list.shuffle")
	// Children = ParameterNodes (same as function params)
	// ReturnType = TypeNode for the return type (reusing existing field)
	ExternSymbol   string // The 'as' symbol (C++ function name)
	ExternReceiver string // The receiver type prefix (e.g. "list", "vec", "str") — empty for free functions

	// FnDecl marks the `name = fn(...)` node a `fn name(...)` declaration is
	// parsed into. A declaration is fixed (never reassigned) and hoisted to
	// the start of its scope; a plain assignment of a lambda is neither.
	FnDecl bool

	// SourceFile is the display name of the file the node was parsed from, set
	// by the loader for nodes spliced in from imported modules (empty for the
	// root file). Codegen uses it for runtime error locations.
	SourceFile string
}

// SetSourceFile sets SourceFile on n and every node below it.
func SetSourceFile(n *TreeNode, file string) {
	if n == nil {
		return
	}
	n.SourceFile = file
	for _, c := range n.Children {
		SetSourceFile(c, file)
	}
	SetSourceFile(n.ReturnType, file)
	SetSourceFile(n.DefaultValue, file)
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
	PrecTernary               // if-else
	PrecOr                    // or
	PrecAnd                   // and
	PrecEquality              // == !=
	PrecComparison            // < <= > >=
	PrecTerm                  // + -
	PrecFactor                // * / %
	PrecUnary                 // ! - (prefix; their operand is parsed at PrecExponent)
	PrecExponent              // **
	PrecAccess                // . [] ()
)

var precedenceNames = map[Precedence]string{
	PrecLowest:     "Lowest",
	PrecAssignment: "Assignment",
	PrecPipe:       "Pipe",
	PrecTernary:    "Ternary",
	PrecOr:         "Or",
	PrecAnd:        "And",
	PrecEquality:   "Equality",
	PrecComparison: "Comparison",
	PrecTerm:       "Term",
	PrecFactor:     "Factor",
	PrecExponent:   "Exponent",
	PrecUnary:      "Unary",
	PrecAccess:     "Access",
}

func (p Precedence) String() string {
	if name, ok := precedenceNames[p]; ok {
		return name
	}
	return "Unknown"
}
