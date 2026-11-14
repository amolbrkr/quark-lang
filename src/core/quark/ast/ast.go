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
	FunctionCallNode
	ArgumentsNode
	IdentifierNode
	LiteralNode
	AssignNode // target = value: [target, value]; the target is an identifier, MemberNode or IndexNode
	MemberNode // object.name: [object, IdentifierNode name] (a dict key, struct field or method name)
	UnaryNode  // -x, !x: [operand]
	BinaryNode // left op right (arithmetic, comparison, and, or): [left, right]
	PairNode   // key: value in a dict literal: [key, value]
	FnDeclNode // fn name(...) -> body: [IdentifierNode name, LambdaNode]; Token is 'fn'
	IfStatementNode
	WhenStatementNode
	PatternNode
	ForLoopNode
	WhileLoopNode
	LambdaNode
	TernaryNode
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
	// QueryNode is a table query, `t | e -> verb(...) | verb(...)`
	// (specs/tables_v0_1_spec.md §4): [source, IdentifierNode row name,
	// QueryStepNode...]. Token is the `|` that starts it.
	QueryNode
	// QueryStepNode is one verb of a query: Token is the verb's name
	// (filter, mutate, ...), Children its arguments: expressions,
	// ColumnDefNodes (mutate, summarize) or SortKeyNodes (sort).
	QueryStepNode
	// ColumnDefNode is `name = value` in mutate or summarize: Token is the
	// column name, Children [value]; in summarize the value is an
	// AggregateNode.
	ColumnDefNode
	// AggregateNode is a summarize aggregate, `sum(e.x)` or `count()`:
	// Token is its name, Children its arguments. Aggregate names are syntax
	// here, never resolved as functions.
	AggregateNode
	// SortKeyNode is a sort key: Children [key]; Descending is set for
	// desc(key).
	SortKeyNode
)

var nodeTypeNames = map[NodeType]string{
	CompilationUnitNode:   "CompilationUnit",
	BlockNode:             "Block",
	FunctionCallNode:      "FunctionCall",
	ArgumentsNode:         "Arguments",
	IdentifierNode:        "Identifier",
	LiteralNode:           "Literal",
	AssignNode:            "Assign",
	MemberNode:            "Member",
	UnaryNode:             "Unary",
	BinaryNode:            "Binary",
	PairNode:              "Pair",
	FnDeclNode:            "FnDecl",
	IfStatementNode:       "IfStatement",
	WhenStatementNode:     "WhenStatement",
	PatternNode:           "Pattern",
	ForLoopNode:           "ForLoop",
	WhileLoopNode:         "WhileLoop",
	LambdaNode:            "Lambda",
	TernaryNode:           "Ternary",
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
	QueryNode:             "Query",
	QueryStepNode:         "QueryStep",
	ColumnDefNode:         "ColumnDef",
	AggregateNode:         "Aggregate",
	SortKeyNode:           "SortKey",
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

	// Piped marks a FunctionCallNode the parser made from a pipe: `x | f(a)`
	// is parsed as `f(x, a)`, so the piped input is the first argument.
	// Only messages use it ("including piped input").
	Piped bool

	// Descending marks a SortKeyNode written desc(key).
	Descending bool

	// SourceFile is the display name of the file the node was parsed from, set
	// by the loader for nodes spliced in from imported modules (empty for the
	// root file). Lowering uses it for runtime error locations.
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

// IsFnDecl reports whether n is a well-formed `fn name(...)` declaration. A
// declaration is fixed (never reassigned) and hoisted to the start of its
// scope; a plain `name = fn(...)` assignment is neither.
func IsFnDecl(n *TreeNode) bool {
	return n != nil && n.NodeType == FnDeclNode && len(n.Children) == 2 &&
		n.Children[0] != nil && n.Children[0].NodeType == IdentifierNode &&
		n.Children[1] != nil && n.Children[1].NodeType == LambdaNode
}

// Op is the operator token of a UnaryNode or BinaryNode (token.ILLEGAL
// otherwise).
func (n *TreeNode) Op() token.TokenType {
	if n == nil || n.Token == nil || (n.NodeType != UnaryNode && n.NodeType != BinaryNode) {
		return token.ILLEGAL
	}
	return n.Token.Type
}

// IsNegatedNumber reports whether n is a unary minus on a number literal.
func IsNegatedNumber(n *TreeNode) bool {
	if n.Op() != token.MINUS || len(n.Children) != 1 {
		return false
	}
	c := n.Children[0]
	return c != nil && c.NodeType == LiteralNode && c.Token != nil && (c.Token.Type == token.INT || c.Token.Type == token.FLOAT)
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
