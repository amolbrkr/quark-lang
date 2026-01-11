"""
Semantic Analyzer for Quark Language

Performs:
- Symbol table management (tracking variables, functions, parameters)
- Type inference and checking
- Semantic error detection (undefined symbols, type mismatches, etc.)
"""

from .helper_types import NodeType, TreeNode
from typing import Dict, List, Optional, Any
from dataclasses import dataclass


@dataclass
class Symbol:
    """Represents a symbol in the symbol table"""
    name: str
    symbol_type: str  # 'variable', 'function', 'parameter'
    data_type: str    # 'number', 'str', 'bool', 'list', 'dict', 'function', 'object', 'unknown'
    node: TreeNode = None  # Reference to AST node

    def __str__(self):
        return f"{self.name}: {self.data_type} ({self.symbol_type})"


class Scope:
    """Represents a lexical scope"""
    def __init__(self, name: str, parent: Optional['Scope'] = None):
        self.name = name
        self.parent = parent
        self.symbols: Dict[str, Symbol] = {}

    def define(self, symbol: Symbol) -> None:
        """Define a symbol in this scope"""
        self.symbols[symbol.name] = symbol

    def lookup(self, name: str, recursive: bool = True) -> Optional[Symbol]:
        """Look up a symbol in this scope or parent scopes"""
        if name in self.symbols:
            return self.symbols[name]
        if recursive and self.parent:
            return self.parent.lookup(name, recursive=True)
        return None

    def __str__(self):
        return f"Scope({self.name}): {list(self.symbols.keys())}"


class SemanticError:
    """Represents a semantic error"""
    def __init__(self, message: str, node: TreeNode = None):
        self.message = message
        self.node = node
        self.line = node.tok.lineno if node and node.tok else None

    def __str__(self):
        if self.line:
            return f"Line {self.line}: {self.message}"
        return self.message


class SemanticAnalyzer:
    """Semantic analyzer with type inference and error checking"""

    def __init__(self):
        self.global_scope = Scope("global")
        self.current_scope = self.global_scope
        self.errors: List[SemanticError] = []

        # Built-in functions
        self._register_builtins()

    def _register_builtins(self):
        """Register built-in functions in global scope"""
        builtins = [
            Symbol('print', 'function', 'function'),
            Symbol('len', 'function', 'function'),
            Symbol('range', 'function', 'function'),
            Symbol('str', 'function', 'function'),
            Symbol('int', 'function', 'function'),
            Symbol('float', 'function', 'function'),
            Symbol('bool', 'function', 'function'),
            Symbol('list', 'function', 'function'),
            Symbol('dict', 'function', 'function'),
        ]
        for builtin in builtins:
            self.global_scope.define(builtin)

    def error(self, message: str, node: TreeNode = None):
        """Record a semantic error"""
        self.errors.append(SemanticError(message, node))

    def enter_scope(self, name: str):
        """Enter a new scope"""
        new_scope = Scope(name, parent=self.current_scope)
        self.current_scope = new_scope
        return new_scope

    def exit_scope(self):
        """Exit current scope and return to parent"""
        if self.current_scope.parent:
            self.current_scope = self.current_scope.parent

    def analyze(self, ast: TreeNode) -> bool:
        """Analyze the AST and return True if no errors"""
        self.visit(ast)
        return len(self.errors) == 0

    def visit(self, node: TreeNode) -> str:
        """
        Visit a node and return its inferred type.
        Dispatches to specific visitor methods based on node type.
        """
        if node is None:
            return 'unknown'

        method_name = f'visit_{node.type.name}'
        visitor = getattr(self, method_name, self.generic_visit)
        return visitor(node)

    def generic_visit(self, node: TreeNode) -> str:
        """Default visitor for nodes without specific handlers"""
        for child in node.children:
            self.visit(child)
        return 'unknown'

    # ==================== Statement Visitors ====================

    def visit_CompilationUnit(self, node: TreeNode) -> str:
        """Visit compilation unit (top-level)"""
        for child in node.children:
            self.visit(child)
        return 'unknown'

    def visit_Block(self, node: TreeNode) -> str:
        """Visit a block of statements"""
        for child in node.children:
            self.visit(child)
        return 'unknown'

    def visit_Function(self, node: TreeNode) -> str:
        """Visit function definition"""
        # Function structure: [name_node, params_node, body_node]
        name_node = node.children[0]
        params_node = node.children[1]
        body_node = node.children[2]

        func_name = name_node.tok.value

        # Register function in current scope
        func_symbol = Symbol(func_name, 'function', 'function', node)
        self.current_scope.define(func_symbol)

        # Enter function scope
        self.enter_scope(f"function:{func_name}")

        # Register parameters
        for param in params_node.children:
            param_name = param.tok.value
            param_symbol = Symbol(param_name, 'parameter', 'unknown', param)
            self.current_scope.define(param_symbol)

        # Visit function body
        self.visit(body_node)

        # Exit function scope
        self.exit_scope()

        return 'function'

    def visit_IfStatement(self, node: TreeNode) -> str:
        """Visit if statement"""
        # Structure: [condition, then_block, elseif_blocks..., else_block]
        condition = node.children[0]

        # Check condition
        self.visit(condition)

        # Visit all branches
        for branch in node.children[1:]:
            if branch.type == NodeType.IfStatement:
                # This is an elseif
                self.visit(branch)
            else:
                # This is a block (then or else)
                self.visit(branch)

        return 'unknown'

    def visit_WhenStatement(self, node: TreeNode) -> str:
        """Visit when statement (pattern matching)"""
        # Structure: [match_expr, pattern1, pattern2, ...]
        match_expr = node.children[0]
        self.visit(match_expr)

        # Visit all patterns
        for pattern in node.children[1:]:
            self.visit(pattern)

        return 'unknown'

    def visit_Pattern(self, node: TreeNode) -> str:
        """Visit pattern in when statement"""
        # Structure: [pattern_expr(s)..., result_expr]
        # Visit all children (patterns and result)
        for child in node.children:
            self.visit(child)
        return 'unknown'

    def visit_ForLoop(self, node: TreeNode) -> str:
        """Visit for loop"""
        # Structure: [loop_var, iterable, body]
        loop_var = node.children[0]
        iterable = node.children[1]
        body = node.children[2]

        # Check iterable
        self.visit(iterable)

        # Enter loop scope
        self.enter_scope("for")

        # Register loop variable
        var_name = loop_var.tok.value
        var_symbol = Symbol(var_name, 'variable', 'unknown', loop_var)
        self.current_scope.define(var_symbol)

        # Visit body
        self.visit(body)

        # Exit loop scope
        self.exit_scope()

        return 'unknown'

    def visit_WhileLoop(self, node: TreeNode) -> str:
        """Visit while loop"""
        # Structure: [condition, body]
        condition = node.children[0]
        body = node.children[1]

        self.visit(condition)

        # Enter loop scope
        self.enter_scope("while")
        self.visit(body)
        self.exit_scope()

        return 'unknown'

    # ==================== Expression Visitors ====================

    def visit_Operator(self, node: TreeNode) -> str:
        """Visit operator node and infer result type"""
        op = node.tok.value

        # Visit children and get their types
        if len(node.children) == 1:
            # Unary operator
            operand_type = self.visit(node.children[0])

            if op in ['!', '~']:
                return 'bool'
            elif op == '-':
                return operand_type if operand_type == 'number' else 'unknown'

        elif len(node.children) == 2:
            # Binary operator

            # Assignment - handle specially (don't visit left side)
            if op == '=':
                left_node = node.children[0]
                right_type = self.visit(node.children[1])  # Only visit right side

                if left_node.type == NodeType.Identifier:
                    var_name = left_node.tok.value
                    # Define or update variable
                    symbol = self.current_scope.lookup(var_name, recursive=False)
                    if symbol:
                        # Update type
                        symbol.data_type = right_type
                    else:
                        # Define new variable
                        var_symbol = Symbol(var_name, 'variable', right_type, left_node)
                        self.current_scope.define(var_symbol)
                return right_type

            # All other binary operators - visit both sides
            left_type = self.visit(node.children[0])
            right_type = self.visit(node.children[1])

            # Arithmetic operators
            if op in ['+', '-', '*', '/', '%', '**']:
                # Special case: string concatenation
                if op == '+' and left_type == 'str' and right_type == 'str':
                    return 'str'

                # Numeric operations
                if left_type == 'number' and right_type == 'number':
                    return 'number'
                elif left_type in ['number', 'unknown'] or right_type in ['number', 'unknown']:
                    return 'number'  # Assume numeric for unknown types
                else:
                    self.error(f"Cannot perform '{op}' on {left_type} and {right_type}", node)
                    return 'unknown'

            # Comparison operators
            elif op in ['<', '<=', '>', '>=', '==', '!=']:
                return 'bool'

            # Logical operators
            elif op in ['and', 'or', '&']:
                return 'bool'

            # Range operator
            elif op == '..':
                return 'list'  # Range produces a list

            # Member access
            elif op == '.':
                return 'unknown'  # Type depends on member

            # Comma operator
            elif op == ',':
                return right_type  # Return type of last element

        return 'unknown'

    def visit_Identifier(self, node: TreeNode) -> str:
        """Visit identifier and check if it's defined"""
        name = node.tok.value

        # Special case: wildcard in pattern matching
        if name == '_':
            return 'unknown'

        symbol = self.current_scope.lookup(name)
        if symbol:
            return symbol.data_type
        else:
            self.error(f"Undefined identifier '{name}'", node)
            return 'unknown'

    def visit_Literal(self, node: TreeNode) -> str:
        """Visit literal and return its type"""
        tok_type = node.tok.type

        if tok_type == 'INT':
            return 'number'
        elif tok_type == 'FLOAT':
            return 'number'
        elif tok_type == 'STR':
            return 'str'
        elif tok_type == 'LBRACE':
            # List literal
            for child in node.children:
                self.visit(child)
            return 'list'
        elif tok_type == 'BLOCKSTART':
            # Dict literal
            for child in node.children:
                self.visit(child)
            return 'dict'

        return 'unknown'

    def visit_FunctionCall(self, node: TreeNode) -> str:
        """Visit function call and check arguments"""
        # Structure: [func_identifier, arguments_node]
        func_node = node.children[0]
        args_node = node.children[1]

        # Get function name
        if func_node.type == NodeType.Identifier:
            func_name = func_node.tok.value

            # Check if function is defined
            symbol = self.current_scope.lookup(func_name)
            if not symbol:
                self.error(f"Undefined function '{func_name}'", func_node)
            elif symbol.symbol_type != 'function':
                self.error(f"'{func_name}' is not a function", func_node)
        else:
            # Complex expression as function (e.g., member access)
            self.visit(func_node)

        # Visit arguments
        self.visit(args_node)

        return 'unknown'  # Return type depends on function

    def visit_Arguments(self, node: TreeNode) -> str:
        """Visit function arguments"""
        for arg in node.children:
            self.visit(arg)
        return 'unknown'

    def visit_Ternary(self, node: TreeNode) -> str:
        """Visit ternary expression"""
        # Structure: [condition, value_if_true, value_if_false]
        self.visit(node.children[0])  # condition
        true_type = self.visit(node.children[1])
        false_type = self.visit(node.children[2])

        # Result type is the common type of branches
        if true_type == false_type:
            return true_type
        return 'unknown'

    def visit_Pipe(self, node: TreeNode) -> str:
        """Visit pipe expression"""
        # Structure: [left_expr, right_expr]
        # For now, just visit both sides
        self.visit(node.children[0])
        return self.visit(node.children[1])

    def visit_Expression(self, node: TreeNode) -> str:
        """Visit expression statement"""
        if node.children:
            return self.visit(node.children[0])
        return 'unknown'

    def visit_Statement(self, node: TreeNode) -> str:
        """Visit statement"""
        if node.children:
            return self.visit(node.children[0])
        return 'unknown'

    def print_errors(self):
        """Print all semantic errors"""
        if not self.errors:
            print("OK No semantic errors")
            return

        print(f"\nERROR {len(self.errors)} semantic error(s) found:\n")
        for error in self.errors:
            print(f"  {error}")

    def print_symbol_table(self, scope: Scope = None, indent: int = 0):
        """Print symbol table for debugging"""
        if scope is None:
            scope = self.global_scope

        print("  " * indent + str(scope))
        for symbol in scope.symbols.values():
            print("  " * (indent + 1) + f"- {symbol}")
