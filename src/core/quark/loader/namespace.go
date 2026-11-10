package loader

import (
	"strings"

	"quark/ast"
)

// MemberSep joins a module name and a member name into the member's internal
// name: member `open` of module `io` is `io__open`, emitted as quark_io__open.
const MemberSep = "__"

// MemberName is the internal name of member `member` of module `module`.
func MemberName(module, member string) string {
	return module + MemberSep + member
}

// DisplayMember turns an internal member name back into the name the user
// wrote (io__open -> open), for messages.
func DisplayMember(module, member string) string {
	return strings.TrimPrefix(member, module+MemberSep)
}

// namespaceModules gives every module member a name of its own, so that
// members of different modules, and user definitions, can't collide in the
// single scope all spliced files share. It runs once on the fully spliced tree:
//
//   - definitions at the top level of a module body, and references to them
//     from inside that module, become `module__name`;
//   - `alias.member` keeps its form, with member renamed, so the analyzer
//     resolves it through the module's symbols;
//   - bare names imported by `use module` (no alias) are renamed where they
//     are used.
//
// Parameters, loop variables and ok/err bindings shadow members as usual.
func (ml *ModuleLoader) namespaceModules(root *ast.TreeNode) {
	if root == nil {
		return
	}
	members := map[string]map[string]bool{}
	for _, n := range root.Children {
		if n == nil || n.NodeType != ast.ModuleNode || len(n.Children) < 2 || n.Children[1] == nil {
			continue
		}
		set := map[string]bool{}
		for _, stmt := range n.Children[1].Children {
			if name := definedName(stmt); name != "" {
				set[name] = true
			}
		}
		members[n.Children[0].TokenLiteral()] = set
	}
	if len(members) == 0 {
		return
	}

	// Aliases and bare imports come from `use` statements at the top level
	// (imports inside other files are spliced there too).
	aliases := map[string]string{} // alias -> module
	bare := map[string]string{}    // member -> module, from `use module`
	bareUseLine := map[string]int{}
	for _, n := range root.Children {
		if n == nil || n.NodeType != ast.UseNode || len(n.Children) == 0 {
			continue
		}
		module := n.Children[0].TokenLiteral()
		set, ok := members[module]
		if !ok {
			continue
		}
		if len(n.Children) >= 2 && n.Children[1] != nil {
			aliases[n.Children[1].TokenLiteral()] = module
			continue
		}
		aliases[module] = module
		for name := range set {
			if prev, seen := bare[name]; seen && prev != module {
				ml.addErrorAt(lineOf(n), 0, "symbol '%s' from module '%s' conflicts with symbol '%s' from module '%s'; import one of them with an alias", name, module, name, prev)
				continue
			}
			bare[name] = module
			bareUseLine[name] = lineOf(n)
		}
	}
	// A top-level definition with the same name as a bare import is ambiguous.
	for _, n := range root.Children {
		if name := definedName(n); name != "" {
			if module, clash := bare[name]; clash {
				ml.addErrorAt(lineOf(n), 0, "symbol '%s' from module '%s' (imported on line %d) conflicts with this definition; import the module with an alias", name, module, bareUseLine[name])
			}
		}
	}

	r := &memberRenamer{members: members, aliases: aliases, bare: bare}
	for _, n := range root.Children {
		r.rewrite(n, "", nil)
	}
}

// definedName is the name a top-level statement defines: a function, a
// function binding or any variable assignment.
func definedName(stmt *ast.TreeNode) string {
	if stmt == nil {
		return ""
	}
	switch {
	case stmt.NodeType == ast.FunctionNode && len(stmt.Children) > 0 && stmt.Children[0] != nil:
		return stmt.Children[0].TokenLiteral()
	case stmt.NodeType == ast.OperatorNode && stmt.Token != nil && stmt.Token.Literal == "=" &&
		len(stmt.Children) == 2 && stmt.Children[0] != nil && stmt.Children[0].NodeType == ast.IdentifierNode:
		return stmt.Children[0].TokenLiteral()
	}
	return ""
}

func lineOf(n *ast.TreeNode) int {
	if n != nil && n.Token != nil {
		return n.Token.Line
	}
	return 0
}

type memberRenamer struct {
	members map[string]map[string]bool
	aliases map[string]string
	bare    map[string]string
}

// resolve returns the internal name for a bare identifier, if it names a
// member: a sibling inside module `module`, or a bare import.
func (r *memberRenamer) resolve(name, module string, shadow map[string]bool) (string, bool) {
	if shadow[name] {
		return "", false
	}
	if module != "" && r.members[module][name] {
		return MemberName(module, name), true
	}
	if m, ok := r.bare[name]; ok {
		return MemberName(m, name), true
	}
	return "", false
}

func renameToken(n *ast.TreeNode, name string) {
	if n.Token == nil {
		return
	}
	tok := *n.Token
	tok.Literal = name
	n.Token = &tok
}

func withShadow(shadow map[string]bool, names ...string) map[string]bool {
	out := make(map[string]bool, len(shadow)+len(names))
	for k, v := range shadow {
		out[k] = v
	}
	for _, n := range names {
		if n != "" {
			out[n] = true
		}
	}
	return out
}

func paramNames(args *ast.TreeNode) []string {
	if args == nil {
		return nil
	}
	var names []string
	for _, p := range args.Children {
		if p == nil {
			continue
		}
		if p.NodeType == ast.ParameterNode && len(p.Children) > 0 {
			names = append(names, p.Children[0].TokenLiteral())
		} else {
			names = append(names, p.TokenLiteral())
		}
	}
	return names
}

func (r *memberRenamer) rewrite(n *ast.TreeNode, module string, shadow map[string]bool) {
	if n == nil {
		return
	}
	switch n.NodeType {
	case ast.ModuleNode:
		if len(n.Children) >= 2 {
			r.rewrite(n.Children[1], n.Children[0].TokenLiteral(), nil)
		}
		return
	case ast.UseNode, ast.StructDefNode, ast.TypeNode, ast.ExternFnNode, ast.ExternHeaderNode,
		ast.ExternLinkNode, ast.ExternIncludePathNode, ast.ParameterNode:
		return
	case ast.IdentifierNode:
		if name, ok := r.resolve(n.TokenLiteral(), module, shadow); ok {
			renameToken(n, name)
		}
		return
	case ast.StructLiteralNode:
		// Field names (first child of each field) aren't variables.
		for _, f := range n.Children {
			if f != nil && f.NodeType == ast.StructFieldNode {
				for _, c := range f.Children[min(1, len(f.Children)):] {
					r.rewrite(c, module, shadow)
				}
				continue
			}
			r.rewrite(f, module, shadow)
		}
		return
	case ast.FunctionNode:
		// name, params, body
		if len(n.Children) > 0 {
			r.rewrite(n.Children[0], module, shadow)
		}
		if len(n.Children) >= 3 {
			r.rewrite(n.Children[2], module, withShadow(shadow, paramNames(n.Children[1])...))
		}
		return
	case ast.LambdaNode:
		// params, body
		if len(n.Children) >= 2 {
			r.rewrite(n.Children[1], module, withShadow(shadow, paramNames(n.Children[0])...))
		}
		return
	case ast.ForLoopNode:
		// var, iterable, body
		if len(n.Children) >= 3 {
			r.rewrite(n.Children[1], module, shadow)
			r.rewrite(n.Children[2], module, withShadow(shadow, n.Children[0].TokenLiteral()))
			return
		}
	case ast.PatternNode:
		armShadow := shadow
		for _, c := range n.Children {
			if c != nil && c.NodeType == ast.ResultPatternNode && len(c.Children) > 0 {
				armShadow = withShadow(shadow, c.Children[0].TokenLiteral())
			}
		}
		for _, c := range n.Children {
			if c != nil && c.NodeType == ast.ResultPatternNode {
				continue
			}
			r.rewrite(c, module, armShadow)
		}
		return
	case ast.OperatorNode:
		if n.Token != nil && n.Token.Literal == "." && len(n.Children) == 2 {
			left, right := n.Children[0], n.Children[1]
			if left != nil && left.NodeType == ast.IdentifierNode && right != nil && right.NodeType == ast.IdentifierNode {
				if m, isAlias := r.aliases[left.TokenLiteral()]; isAlias && !shadow[left.TokenLiteral()] {
					if r.members[m][right.TokenLiteral()] {
						renameToken(right, MemberName(m, right.TokenLiteral()))
					}
					return
				}
			}
			// Only the left side is an expression; the right is a key or method name.
			r.rewrite(left, module, shadow)
			return
		}
	}
	for _, c := range n.Children {
		r.rewrite(c, module, shadow)
	}
}
