package types

import (
	"fmt"
	"quark/ast"
	"quark/builtins"
	"quark/diagnostics"
	"quark/ir"
	"quark/names"
)

type builtinSignature struct {
	Type    *FunctionType
	MinArgs int
	MaxArgs int
}

type paramSpec struct {
	name         string
	nameNode     *ast.TreeNode
	typeNode     *ast.TreeNode
	defaultValue *ast.TreeNode
}

// Analyzer performs semantic analysis on the AST
type Analyzer struct {
	currentScope    *Scope
	errors          []diagnostics.Diagnostic
	builtins        map[string]*builtinSignature
	methods         map[builtins.TypeKey]map[string]*builtinSignature // receiver type -> method name -> sig
	callPlans       map[*ast.TreeNode]*ir.CallPlan
	returnValidated map[*ast.TreeNode]bool
	nodeTypes       map[*ast.TreeNode]Type // Inferred type of every expression node
	// paramUsages collects call shapes of the parameters of the functions
	// being analyzed (analyzer_callshapes.go).
	paramUsages     map[*Symbol]*paramUsage
	paramOwnerSeq   int
	loopDepth       int
	pendingFuncName string
	// externFns maps extern fn registrations: free function name or "type.method" key
	// → prototype CallPlan used to build per-call-site plans with DispatchExtern.
	externFns map[string]*ir.CallPlan
	// externSources holds resolved absolute paths from extern 'path' directives.
	externSources []string
	// nativeFns maps the C++ name (FixedFn.CName) of each fully annotated,
	// non-capturing declared function to its prototype CallPlan with
	// DispatchNative and populated NativeParamTypes/NativeReturnType. Filled
	// by finalizeFixedFns.
	nativeFns map[string]*ir.CallPlan
	// structTypes maps struct name → StructType for user-defined struct types.
	structTypes map[string]*StructType
	// builtinSyms holds the symbol each builtin (prelude or extern fn) was
	// declared with. Calls and captures are resolved by name, so a name that
	// looks up to a different symbol is a local (parameter, loop variable)
	// shadowing it.
	builtinSyms map[string]*Symbol

	// Fixed `fn` declarations (analyzer_fixed.go).
	rootScope     *Scope
	curList       *ast.TreeNode // statement list being analyzed, and the index in it
	curIdx        int
	fixedBySym    map[*Symbol]*FixedFn
	fixedByLambda map[*ast.TreeNode]*FixedFn
	fixedList     []*FixedFn
	fixedCalls    []fixedCall
	fixedRefs     map[*ast.TreeNode]*FixedFn  // identifier nodes naming a declaration
	captureSyms   map[*ast.TreeNode][]*Symbol // lambda -> symbol of each captured name
	fnScopes      []*fnScope
	symDefs       map[*Symbol]symDef
	hoistScopes   map[*ast.TreeNode]*HoistScope

	// names maps the loader's internal names back to the names users write;
	// every diagnostic goes through it.
	names names.Table

	// Symbols (finalizeSymbols): every declared symbol in declaration order,
	// and the symbol each resolved identifier node refers to.
	symbols []*Symbol
	refs    map[*ast.TreeNode]*Symbol
}

func NewAnalyzer() *Analyzer {
	globalScope := NewScope(nil)
	builtinSyms := make(map[string]*Symbol)

	builtinSigs := make(map[string]*builtinSignature)
	methodSigs := make(map[builtins.TypeKey]map[string]*builtinSignature)
	for _, spec := range builtins.Catalog() {
		paramTypes := make([]Type, 0, len(spec.ParamTypes))
		for _, key := range spec.ParamTypes {
			paramTypes = append(paramTypes, mapBuiltinTypeKey(key))
		}
		funcType := &FunctionType{ParamTypes: paramTypes, ReturnType: mapBuiltinTypeKey(spec.ReturnType)}
		sig := &builtinSignature{Type: funcType, MinArgs: spec.MinArgs, MaxArgs: spec.MaxArgs}

		if spec.ReceiverType == "" {
			if _, exists := builtinSigs[spec.Name]; !exists {
				builtinSyms[spec.Name] = globalScope.Define(spec.Name, funcType, false)
				builtinSigs[spec.Name] = sig
			}
		} else {
			if methodSigs[spec.ReceiverType] == nil {
				methodSigs[spec.ReceiverType] = make(map[string]*builtinSignature)
			}
			if _, exists := methodSigs[spec.ReceiverType][spec.Name]; !exists {
				methodSigs[spec.ReceiverType][spec.Name] = sig
			}
		}
	}

	return &Analyzer{
		currentScope:    globalScope,
		errors:          make([]diagnostics.Diagnostic, 0),
		builtins:        builtinSigs,
		methods:         methodSigs,
		callPlans:       make(map[*ast.TreeNode]*ir.CallPlan),
		returnValidated: make(map[*ast.TreeNode]bool),
		nodeTypes:       make(map[*ast.TreeNode]Type),
		paramUsages:     make(map[*Symbol]*paramUsage),
		externFns:       make(map[string]*ir.CallPlan),
		externSources:   make([]string, 0),
		nativeFns:       make(map[string]*ir.CallPlan),
		structTypes:     make(map[string]*StructType),
		builtinSyms:     builtinSyms,
		rootScope:       globalScope,
		fixedBySym:      make(map[*Symbol]*FixedFn),
		fixedByLambda:   make(map[*ast.TreeNode]*FixedFn),
		fixedRefs:       make(map[*ast.TreeNode]*FixedFn),
		captureSyms:     make(map[*ast.TreeNode][]*Symbol),
		symDefs:         make(map[*Symbol]symDef),
		hoistScopes:     make(map[*ast.TreeNode]*HoistScope),
		refs:            make(map[*ast.TreeNode]*Symbol),
	}
}

// declareSym defines name in the current scope as a symbol of the given kind,
// declared (in generated C++) at node at. store is the type every value it
// holds is known to have (nil for any value).
func (a *Analyzer) declareSym(name string, typ Type, mutable bool, kind SymKind, store Type, at *ast.TreeNode) *Symbol {
	sym := a.currentScope.Define(name, typ, mutable)
	sym.ID = len(a.symbols) + 1
	sym.Kind = kind
	sym.StoreType = store
	sym.Decl = at
	a.symbols = append(a.symbols, sym)
	if at != nil {
		a.refs[at] = sym
	}
	return sym
}

// refersToBuiltin reports whether name, read in scope, is the builtin or
// extern fn of that name, not a local binding that shadows it.
func (a *Analyzer) refersToBuiltin(scope *Scope, name string) bool {
	if _, ok := a.builtins[name]; !ok {
		return false
	}
	sym := scope.Lookup(name)
	return sym == nil || sym == a.builtinSyms[name]
}

func (a *Analyzer) HasErrors() bool {
	for _, d := range a.errors {
		if d.Severity == "" || d.Severity == diagnostics.SeverityError {
			return true
		}
	}
	return false
}

func (a *Analyzer) Diagnostics() []diagnostics.Diagnostic {
	out := make([]diagnostics.Diagnostic, len(a.errors))
	copy(out, a.errors)
	return out
}

// SetNames passes the loader's name table (modules v0.2), used to show
// internal names as users write them.
func (a *Analyzer) SetNames(t names.Table) {
	a.names = t
}

// display is the user-facing text of a message built from internal names.
func (a *Analyzer) display(msg string) string {
	return a.names.DisplayText(msg)
}

// nodeLocation is where a diagnostic about node is reported: its line and
// column, and its file when it came from an imported file.
func nodeLocation(node *ast.TreeNode) *diagnostics.Location {
	if node == nil || node.Token == nil {
		return nil
	}
	return &diagnostics.Location{File: node.SourceFile, Line: node.Token.Line, Column: node.Token.Column}
}

func (a *Analyzer) addError(format string, args ...interface{}) {
	a.errors = append(a.errors, diagnostics.Diagnostic{
		Code:     "QK-CHECK-001",
		Stage:    diagnostics.StageCheck,
		Severity: diagnostics.SeverityError,
		Message:  a.display(fmt.Sprintf(format, args...)),
	})
}

func (a *Analyzer) errorAt(node *ast.TreeNode, format string, args ...interface{}) {
	msg := a.display(fmt.Sprintf(format, args...))
	loc := nodeLocation(node)
	a.errors = append(a.errors, diagnostics.Diagnostic{
		Code:     "QK-CHECK-001",
		Stage:    diagnostics.StageCheck,
		Severity: diagnostics.SeverityError,
		Message:  msg,
		Location: loc,
	})
}

func (a *Analyzer) warnAt(node *ast.TreeNode, format string, args ...interface{}) {
	msg := a.display(fmt.Sprintf(format, args...))
	loc := nodeLocation(node)
	a.errors = append(a.errors, diagnostics.Diagnostic{
		Code:     "QK-CHECK-002",
		Stage:    diagnostics.StageCheck,
		Severity: diagnostics.SeverityWarning,
		Message:  msg,
		Location: loc,
	})
}

func (a *Analyzer) pushScope() {
	a.currentScope = NewScope(a.currentScope)
}

func (a *Analyzer) popScope() {
	if a.currentScope == nil || a.currentScope.Parent == nil {
		panic("internal compiler error: popScope called at root scope")
	}
	a.currentScope = a.currentScope.Parent
}

// Analyze performs semantic analysis on the AST
func (a *Analyzer) Analyze(node *ast.TreeNode) Type {
	if node == nil {
		return TypeVoid
	}
	t := a.analyze(node)
	a.nodeTypes[node] = t
	return t
}

func (a *Analyzer) analyze(node *ast.TreeNode) Type {
	switch node.NodeType {
	case ast.CompilationUnitNode:
		return a.analyzeCompilationUnit(node)
	case ast.BlockNode:
		return a.analyzeBlock(node)
	case ast.FunctionCallNode:
		return a.analyzeFunctionCall(node)
	case ast.IfStatementNode:
		return a.analyzeIfStatement(node)
	case ast.WhenStatementNode:
		return a.analyzeWhenStatement(node)
	case ast.ForLoopNode:
		return a.analyzeForLoop(node)
	case ast.WhileLoopNode:
		return a.analyzeWhileLoop(node)
	case ast.IdentifierNode:
		return a.analyzeIdentifier(node)
	case ast.LiteralNode:
		return a.analyzeLiteral(node)
	case ast.AssignNode:
		return a.analyzeAssign(node)
	case ast.FnDeclNode:
		return a.analyzeFnDecl(node)
	case ast.MemberNode:
		return a.analyzeMember(node)
	case ast.UnaryNode:
		return a.analyzeUnary(node)
	case ast.BinaryNode:
		return a.analyzeBinary(node)
	case ast.TernaryNode:
		return a.analyzeTernary(node)
	case ast.ListNode:
		return a.analyzeList(node)
	case ast.VectorNode:
		return a.analyzeVector(node)
	case ast.DictNode:
		return a.analyzeDict(node)
	case ast.IndexNode:
		return a.analyzeIndex(node)
	case ast.ResultNode:
		return a.analyzeResult(node)
	case ast.VarDeclNode:
		return a.analyzeVarDecl(node)
	case ast.ModuleNode:
		return a.analyzeModule(node)
	case ast.InclNode:
		a.errorAt(node, "internal compiler error: 'incl' reached the analyzer (the loader removes every incl)")
		return TypeVoid
	case ast.BreakNode:
		if a.loopDepth == 0 {
			a.errorAt(node, "[C-SCOPE] 'break' must be inside a for or while loop")
		}
		return TypeVoid
	case ast.ContinueNode:
		if a.loopDepth == 0 {
			a.errorAt(node, "[C-SCOPE] 'continue' must be inside a for or while loop")
		}
		return TypeVoid
	case ast.LambdaNode:
		return a.analyzeLambda(node)
	case ast.ExternHeaderNode, ast.ExternLinkNode, ast.ExternIncludePathNode:
		return TypeVoid // Recorded by the loader; nothing to analyze
	case ast.ExternFnNode:
		return a.analyzeExternFn(node)
	case ast.StructDefNode:
		return a.analyzeStructDef(node)
	case ast.StructLiteralNode:
		return a.analyzeStructLiteral(node)
	default:
		return TypeAny
	}
}
