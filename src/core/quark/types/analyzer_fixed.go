package types

import (
	"fmt"
	"sort"

	"quark/ast"
	"quark/ir"
)

// Fixed function declarations (modules v0.2 spec, rules 17-23).
//
// `fn f(...)` is parsed as `f = fn(...)` with FnDecl set. A declaration is a
// fixed binding: declared once per scope, never reassigned, never shadowing a
// visible name, and hoisted, so every declaration of a scope exists before
// the scope's first statement runs.
//
// How a declared function is called is decided once the whole program has
// been analyzed (finalizeFixedFns), because a call can precede the callee's
// body. A function that captures nothing (directly, or by referring to a
// function that captures) compiles to a plain C++ function, called directly
// (DispatchDirect, or DispatchNative when fully annotated) and never captured
// by closures. One that captures stays a closure held in its fixed binding.

// FixedFn is one `fn` declaration.
type FixedFn struct {
	Name   string        // declared name (after the loader's module renaming)
	Decl   *ast.TreeNode // the `name = fn(...)` node
	Lambda *ast.TreeNode
	Line   int
	Type   *FunctionType // the analyzed type once its body has been analyzed

	// Set by finalizeFixedFns:
	CName    string       // generated C++ function: quark_<CName>
	Captures bool         // needs a closure (and a binding holding it)
	Native   *ir.CallPlan // non-nil when compiled with a native C++ signature

	sym      *Symbol
	scope    *Scope
	topLevel bool // declared in the root scope or a module body
}

// HoistScope is what codegen emits at the start of a scope that declares
// functions: the variables its capturing functions capture (as unassigned
// cells, since the closures are created before those variables are
// assigned), then the closures of its capturing functions.
type HoistScope struct {
	Fns   []*FixedFn // every declaration of the scope, in source order
	Cells []string   // variables of the scope captured by its capturing functions
}

type fixedCall struct {
	plan *ir.CallPlan
	fn   *FixedFn
}

// fnScope is a statement list that declares functions.
type fnScope struct {
	node  *ast.TreeNode // CompilationUnit, module body or block
	scope *Scope
	stmts []*ast.TreeNode
	fns   []*FixedFn
}

// symDef is where a variable was first assigned in its scope.
type symDef struct {
	stmt int // index of the statement in the scope's statement list
	line int
}

func isFnDecl(n *ast.TreeNode) bool {
	return n != nil && n.FnDecl && len(n.Children) == 2 && n.Children[0] != nil &&
		n.Children[0].NodeType == ast.IdentifierNode && n.Children[1] != nil && n.Children[1].NodeType == ast.LambdaNode
}

func nodeLine(n *ast.TreeNode) int {
	if n != nil && n.Token != nil {
		return n.Token.Line
	}
	return 0
}

// enterStatement records that statement idx of the current scope is being
// analyzed, so first assignments can be ordered against calls (rule 23).
func (a *Analyzer) enterStatement(idx int) {
	a.scopeStmt[a.currentScope] = idx
}

// recordDef notes the first assignment of a variable defined in the current scope.
func (a *Analyzer) recordDef(sym *Symbol, at *ast.TreeNode) {
	if sym == nil {
		return
	}
	if _, seen := a.symDefs[sym]; seen {
		return
	}
	a.symDefs[sym] = symDef{stmt: a.scopeStmt[a.currentScope], line: nodeLine(at)}
}

// declareFixedFns declares every `fn` of a statement list in the current
// scope, before any of the statements is analyzed (hoisting, rule 22), and
// reports duplicate (rule 18) and shadowing (rule 20) declarations.
func (a *Analyzer) declareFixedFns(scopeNode *ast.TreeNode, stmts []*ast.TreeNode) {
	sc := &fnScope{node: scopeNode, scope: a.currentScope, stmts: stmts}
	for _, stmt := range stmts {
		if !isFnDecl(stmt) {
			continue
		}
		nameNode := stmt.Children[0]
		name := nameNode.TokenLiteral()
		if name == "" {
			continue
		}
		if a.refersToBuiltin(a.currentScope, name) {
			if _, isExtern := a.externFns[name]; isExtern {
				a.errorAt(nameNode, "'%s' is an extern fn and can't be redefined or assigned; choose another name", name)
			} else {
				a.errorAt(nameNode, "'%s' is a builtin function and can't be redefined or assigned; choose another name", name)
			}
			continue
		}
		if prev := a.currentScope.LookupLocal(name); prev != nil {
			if f := a.fixedBySym[prev]; f != nil {
				a.errorAt(nameNode, "'%s' is already defined on line %d", name, f.Line)
			} else {
				a.errorAt(nameNode, "'%s' is already defined in this scope", name)
			}
			continue
		}
		// Still declared after this error, so uses in this scope don't
		// produce follow-on errors against the outer name.
		if a.currentScope.Parent != nil {
			if outer := a.currentScope.Parent.Lookup(name); outer != nil {
				if f := a.fixedBySym[outer]; f != nil {
					a.errorAt(nameNode, "'%s' is already a function in an enclosing scope (line %d); choose another name", name, f.Line)
				} else {
					a.errorAt(nameNode, "'%s' is already defined in an enclosing scope; choose another name for this function", name)
				}
			}
		}
		ft := functionTypeFromLambdaNodeWithStructs(stmt.Children[1], a.structTypes)
		sym := a.currentScope.Define(name, ft, false)
		f := &FixedFn{
			Name:     name,
			Decl:     stmt,
			Lambda:   stmt.Children[1],
			Line:     nodeLine(nameNode),
			Type:     ft,
			sym:      sym,
			scope:    a.currentScope,
			topLevel: a.currentScope == a.rootScope || a.moduleScopes[a.currentScope],
		}
		a.fixedBySym[sym] = f
		a.fixedByLambda[f.Lambda] = f
		a.fixedList = append(a.fixedList, f)
		sc.fns = append(sc.fns, f)
	}
	if len(sc.fns) > 0 {
		a.fnScopes = append(a.fnScopes, sc)
	}
}

// analyzeFnDecl analyzes the body of a declaration hoisted by declareFixedFns.
func (a *Analyzer) analyzeFnDecl(node *ast.TreeNode) Type {
	nameNode, lambda := node.Children[0], node.Children[1]
	a.pendingFuncName = nameNode.TokenLiteral()
	t := a.Analyze(lambda)
	a.pendingFuncName = ""
	if f := a.fixedByLambda[lambda]; f != nil {
		if ft, ok := t.(*FunctionType); ok {
			f.Type = ft
			f.sym.Type = ft
		}
	}
	return t
}

// fixedFnOf returns the declaration sym was declared by, if any.
func (a *Analyzer) fixedFnOf(sym *Symbol) *FixedFn {
	if sym == nil {
		return nil
	}
	return a.fixedBySym[sym]
}

// deferFixedDispatch records a call to a declared function; its dispatch is
// set by finalizeFixedFns.
func (a *Analyzer) deferFixedDispatch(plan *ir.CallPlan, sym *Symbol) {
	if f := a.fixedFnOf(sym); f != nil && plan != nil {
		a.fixedCalls = append(a.fixedCalls, fixedCall{plan: plan, fn: f})
	}
}

// reassignedDeclError reports an assignment to a declared function (rule 19).
func (a *Analyzer) reassignedDeclError(target *ast.TreeNode, f *FixedFn) {
	a.errorAt(target, "'%s' is a function declaration (line %d) and can't be reassigned; use a variable: g = fn ...", displayName(f.Name), f.Line)
}

// finalizeFixedFns runs once the whole program has been analyzed.
func (a *Analyzer) finalizeFixedFns() {
	// 1. Which declarations need a closure: least fixpoint of "captures a
	//    variable, or refers to a declaration that needs one".
	for changed := true; changed; {
		changed = false
		for _, f := range a.fixedList {
			if f.Captures {
				continue
			}
			for _, s := range a.captureSyms[f.Lambda] {
				if g := a.fixedBySym[s]; g == nil || g.Captures {
					f.Captures = true
					changed = true
					break
				}
			}
		}
	}

	// 2. C++ names and native signatures.
	seq := 0
	for _, f := range a.fixedList {
		if f.topLevel && !f.Captures {
			f.CName = f.Name
		} else {
			seq++
			f.CName = fmt.Sprintf("%s__%d", f.Name, seq)
		}
		if !f.Captures && isFullyAnnotated(f.Type) {
			nativeParams := make([]string, len(f.Type.ParamTypes))
			for i, pt := range f.Type.ParamTypes {
				nativeParams[i] = quarkTypeToNativeCType(pt)
			}
			f.Native = &ir.CallPlan{
				Kind:             ir.CallFunctionValue,
				CalleeName:       f.Name,
				MinArity:         len(f.Type.ParamTypes),
				MaxArity:         len(f.Type.ParamTypes),
				Dispatch:         ir.DispatchNative,
				RuntimeSymbol:    "quark_" + f.CName,
				NativeParamTypes: nativeParams,
				NativeReturnType: quarkTypeToNativeCType(f.Type.AnnotatedReturnType),
			}
			a.nativeFns[f.CName] = f.Native
		}
	}

	// 3. Definite assignment (rule 23) and hoisted cells, from the raw captures.
	a.checkDefiniteAssignment()

	// 4. Closures don't capture functions that need no closure; those are
	//    referred to by their C++ name.
	for lambda, syms := range a.captureSyms {
		names := a.captures[lambda]
		keptNames := make([]string, 0, len(names))
		keptSyms := make([]*Symbol, 0, len(syms))
		for i, s := range syms {
			if g := a.fixedBySym[s]; g != nil && !g.Captures {
				continue
			}
			keptNames = append(keptNames, names[i])
			keptSyms = append(keptSyms, s)
		}
		if len(keptNames) == 0 {
			delete(a.captures, lambda)
		} else {
			a.captures[lambda] = keptNames
		}
		a.captureSyms[lambda] = keptSyms
	}

	// 5. Calls and references.
	for _, c := range a.fixedCalls {
		f := c.fn
		switch {
		case f.Native != nil:
			c.plan.Dispatch = ir.DispatchNative
			c.plan.RuntimeSymbol = f.Native.RuntimeSymbol
			c.plan.NativeParamTypes = f.Native.NativeParamTypes
			c.plan.NativeReturnType = f.Native.NativeReturnType
		case !f.Captures:
			c.plan.Dispatch = ir.DispatchDirect
			c.plan.RuntimeSymbol = "quark_" + f.CName
		default:
			c.plan.Dispatch = ir.DispatchClosure
			c.plan.RuntimeSymbol = ""
		}
	}
	for node, f := range a.fixedRefs {
		if f.Captures {
			delete(a.fixedRefs, node)
		}
	}
}

// checkDefiniteAssignment implements rule 23 for every scope that declares
// functions, and records the cells codegen must create unassigned.
//
// A declared function's closure exists from the start of its scope, so a
// call to it can run before a variable it captures is assigned. Direct calls
// (and chains of direct calls through declarations of the same scope) at or
// before the variable's first assignment are rejected here; anything else
// reads the variable through a cell that starts unassigned.
func (a *Analyzer) checkDefiniteAssignment() {
	hoisted := map[*Symbol]bool{}
	for _, sc := range a.fnScopes {
		inScope := map[*FixedFn]bool{}
		for _, f := range sc.fns {
			inScope[f] = true
		}
		isScopeVar := func(s *Symbol) bool {
			return s != nil && sc.scope.Symbols[s.Name] == s && a.fixedBySym[s] == nil
		}
		// Variables of the scope each declaration uses, directly or through
		// calls to other declarations of the scope.
		needs := map[*FixedFn]map[*Symbol]*FixedFn{} // var -> function that captures it
		callees := map[*FixedFn][]*FixedFn{}
		for _, f := range sc.fns {
			needs[f] = map[*Symbol]*FixedFn{}
			for _, s := range a.captureSyms[f.Lambda] {
				if isScopeVar(s) {
					needs[f][s] = f
				}
			}
			a.walkFixedCalls(f.Lambda, true, func(_ *ast.TreeNode, g *FixedFn) {
				if inScope[g] && g != f {
					callees[f] = append(callees[f], g)
				}
			})
		}
		for changed := true; changed; {
			changed = false
			for _, f := range sc.fns {
				for _, g := range callees[f] {
					for s, via := range needs[g] {
						if _, ok := needs[f][s]; !ok {
							needs[f][s] = via
							changed = true
						}
					}
				}
			}
		}
		for i, stmt := range sc.stmts {
			if isFnDecl(stmt) {
				continue
			}
			a.walkFixedCalls(stmt, false, func(call *ast.TreeNode, f *FixedFn) {
				if !inScope[f] {
					return
				}
				vars := make([]*Symbol, 0, len(needs[f]))
				for s := range needs[f] {
					vars = append(vars, s)
				}
				sort.Slice(vars, func(x, y int) bool {
					if a.symDefs[vars[x]].line != a.symDefs[vars[y]].line {
						return a.symDefs[vars[x]].line < a.symDefs[vars[y]].line
					}
					return vars[x].Name < vars[y].Name
				})
				for _, s := range vars {
					via := needs[f][s]
					d, ok := a.symDefs[s]
					if !ok || d.stmt < i {
						continue
					}
					at := call.Children[0]
					if via == f {
						a.errorAt(at, "'%s' uses '%s', which is assigned on line %d, after this call", displayName(f.Name), s.Name, d.line)
					} else {
						a.errorAt(at, "'%s' uses '%s' (through '%s'), which is assigned on line %d, after this call", displayName(f.Name), s.Name, displayName(via.Name), d.line)
					}
				}
			})
		}

		// Cells the scope's closures capture before they can be assigned. The
		// top level and module bodies all run in main, where their closures
		// are created together before the first statement, so variables of
		// any of those scopes count.
		hoistVar := isScopeVar
		if a.isMainScope(sc.scope) {
			hoistVar = func(s *Symbol) bool {
				if s == nil || a.fixedBySym[s] != nil {
					return false
				}
				if a.rootScope.Symbols[s.Name] == s {
					return true
				}
				for ms := range a.moduleScopes {
					if ms.Symbols[s.Name] == s {
						return true
					}
				}
				return false
			}
		}
		hs := &HoistScope{Fns: sc.fns}
		seen := map[string]bool{}
		for _, f := range sc.fns {
			if !f.Captures {
				continue
			}
			for _, s := range a.captureSyms[f.Lambda] {
				if hoistVar(s) && !seen[s.Name] {
					seen[s.Name] = true
					hs.Cells = append(hs.Cells, s.Name)
					hoisted[s] = true
				}
			}
		}
		a.hoistScopes[sc.node] = hs
	}
	for lambda, syms := range a.captureSyms {
		for _, s := range syms {
			if hoisted[s] {
				if a.checkedCaptures[lambda] == nil {
					a.checkedCaptures[lambda] = map[string]bool{}
				}
				a.checkedCaptures[lambda][s.Name] = true
			}
		}
	}
}

// isMainScope reports whether a scope's statements run directly in main:
// the top level and module bodies.
func (a *Analyzer) isMainScope(s *Scope) bool {
	return s == a.rootScope || a.moduleScopes[s]
}

// walkFixedCalls calls visit for every call of a declared function under n.
// Lambda bodies are entered only when intoLambdas is set (a function's body
// may call through nested lambdas while it runs; a statement's lambdas run
// later, if at all).
func (a *Analyzer) walkFixedCalls(n *ast.TreeNode, intoLambdas bool, visit func(call *ast.TreeNode, f *FixedFn)) {
	if n == nil {
		return
	}
	if n.NodeType == ast.LambdaNode && !intoLambdas {
		return
	}
	if n.NodeType == ast.FunctionCallNode && len(n.Children) > 0 {
		if f := a.fixedRefs[n.Children[0]]; f != nil {
			visit(n, f)
		}
	}
	for _, c := range n.Children {
		a.walkFixedCalls(c, intoLambdas, visit)
	}
}
