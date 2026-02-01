package codegen

import (
	"fmt"
	"quark/ast"
	"quark/token"
	"strings"
)

// Generator generates C code from an AST
type Generator struct {
	output        strings.Builder
	indentLevel   int
	functions     []string           // Function definitions (generated separately)
	lambdas       []*ast.TreeNode    // Lambda expressions to generate
	lambdaNames   map[*ast.TreeNode]string // Maps lambda nodes to their generated names
	tempCounter   int
	lambdaCounter int
	inFunction    bool
	currentFunc   string
}

func New() *Generator {
	return &Generator{
		functions:   make([]string, 0),
		lambdas:     make([]*ast.TreeNode, 0),
		lambdaNames: make(map[*ast.TreeNode]string),
		tempCounter: 0,
	}
}

func (g *Generator) indent() string {
	return strings.Repeat("    ", g.indentLevel)
}

func (g *Generator) emit(format string, args ...interface{}) {
	g.output.WriteString(fmt.Sprintf(format, args...))
}

func (g *Generator) emitLine(format string, args ...interface{}) {
	g.output.WriteString(g.indent())
	g.output.WriteString(fmt.Sprintf(format, args...))
	g.output.WriteString("\n")
}

func (g *Generator) newTemp() string {
	g.tempCounter++
	return fmt.Sprintf("_t%d", g.tempCounter)
}

func (g *Generator) newLambda() string {
	g.lambdaCounter++
	return fmt.Sprintf("_lambda%d", g.lambdaCounter)
}

// Generate produces C code from the AST
func (g *Generator) Generate(node *ast.TreeNode) string {
	// Generate header
	g.emit(`#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdarg.h>
#include <math.h>
#include <ctype.h>

// Quark runtime types
typedef struct {
    enum { VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL, VAL_LIST, VAL_FUNC } type;
    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        struct { void** items; int len; int cap; } list_val;
        void* func_val;
    } data;
} QValue;

// Function pointer types for different arities
typedef QValue (*QFunc0)();
typedef QValue (*QFunc1)(QValue);
typedef QValue (*QFunc2)(QValue, QValue);
typedef QValue (*QFunc3)(QValue, QValue, QValue);
typedef QValue (*QFunc4)(QValue, QValue, QValue, QValue);

// Runtime functions
QValue qv_int(long long v) { QValue q; q.type = VAL_INT; q.data.int_val = v; return q; }
QValue qv_float(double v) { QValue q; q.type = VAL_FLOAT; q.data.float_val = v; return q; }
QValue qv_string(const char* v) { QValue q; q.type = VAL_STRING; q.data.string_val = strdup(v); return q; }
QValue qv_bool(bool v) { QValue q; q.type = VAL_BOOL; q.data.bool_val = v; return q; }
QValue qv_null() { QValue q; q.type = VAL_NULL; return q; }

// Function value constructor
QValue qv_func(void* f) { QValue q; q.type = VAL_FUNC; q.data.func_val = f; return q; }

// List operations
QValue qv_list(int initial_cap) {
    QValue q;
    q.type = VAL_LIST;
    q.data.list_val.cap = initial_cap > 0 ? initial_cap : 8;
    q.data.list_val.len = 0;
    q.data.list_val.items = malloc(sizeof(QValue) * q.data.list_val.cap);
    return q;
}

QValue qv_list_from(int count, ...) {
    QValue q = qv_list(count > 0 ? count : 8);
    va_list args;
    va_start(args, count);
    for (int i = 0; i < count; i++) {
        QValue* items = (QValue*)q.data.list_val.items;
        items[i] = va_arg(args, QValue);
    }
    q.data.list_val.len = count;
    va_end(args);
    return q;
}

void q_list_grow(QValue* list) {
    if (list->type != VAL_LIST) return;
    int new_cap = list->data.list_val.cap * 2;
    list->data.list_val.items = realloc(list->data.list_val.items, sizeof(QValue) * new_cap);
    list->data.list_val.cap = new_cap;
}

QValue q_push(QValue list, QValue item) {
    if (list.type != VAL_LIST) return qv_null();
    if (list.data.list_val.len >= list.data.list_val.cap) {
        q_list_grow(&list);
    }
    QValue* items = (QValue*)list.data.list_val.items;
    items[list.data.list_val.len] = item;
    list.data.list_val.len++;
    return list;
}

QValue q_pop(QValue list) {
    if (list.type != VAL_LIST || list.data.list_val.len == 0) return qv_null();
    QValue* items = (QValue*)list.data.list_val.items;
    list.data.list_val.len--;
    return items[list.data.list_val.len];
}

QValue q_get(QValue list, QValue index) {
    if (list.type != VAL_LIST) return qv_null();
    int idx = (int)index.data.int_val;
    if (idx < 0) idx = list.data.list_val.len + idx;
    if (idx < 0 || idx >= list.data.list_val.len) return qv_null();
    QValue* items = (QValue*)list.data.list_val.items;
    return items[idx];
}

QValue q_set(QValue list, QValue index, QValue value) {
    if (list.type != VAL_LIST) return qv_null();
    int idx = (int)index.data.int_val;
    if (idx < 0) idx = list.data.list_val.len + idx;
    if (idx < 0 || idx >= list.data.list_val.len) return qv_null();
    QValue* items = (QValue*)list.data.list_val.items;
    items[idx] = value;
    return value;
}

// Call function value with different arities
QValue q_call0(QValue f) {
    if (f.type != VAL_FUNC) return qv_null();
    return ((QFunc0)f.data.func_val)();
}

QValue q_call1(QValue f, QValue a) {
    if (f.type != VAL_FUNC) return qv_null();
    return ((QFunc1)f.data.func_val)(a);
}

QValue q_call2(QValue f, QValue a, QValue b) {
    if (f.type != VAL_FUNC) return qv_null();
    return ((QFunc2)f.data.func_val)(a, b);
}

QValue q_call3(QValue f, QValue a, QValue b, QValue c) {
    if (f.type != VAL_FUNC) return qv_null();
    return ((QFunc3)f.data.func_val)(a, b, c);
}

QValue q_call4(QValue f, QValue a, QValue b, QValue c, QValue d) {
    if (f.type != VAL_FUNC) return qv_null();
    return ((QFunc4)f.data.func_val)(a, b, c, d);
}

void print_qvalue(QValue v) {
    switch (v.type) {
        case VAL_INT: printf("%%lld", v.data.int_val); break;
        case VAL_FLOAT: printf("%%g", v.data.float_val); break;
        case VAL_STRING: printf("%%s", v.data.string_val); break;
        case VAL_BOOL: printf(v.data.bool_val ? "true" : "false"); break;
        case VAL_NULL: printf("null"); break;
        default: printf("<value>"); break;
    }
}

QValue q_print(QValue v) { print_qvalue(v); return qv_null(); }
QValue q_println(QValue v) { print_qvalue(v); printf("\n"); return qv_null(); }

// Arithmetic operations
QValue q_add(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av + bv);
    }
    return qv_int(a.data.int_val + b.data.int_val);
}

QValue q_sub(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av - bv);
    }
    return qv_int(a.data.int_val - b.data.int_val);
}

QValue q_mul(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av * bv);
    }
    return qv_int(a.data.int_val * b.data.int_val);
}

QValue q_div(QValue a, QValue b) {
    double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
    double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
    return qv_float(av / bv);
}

QValue q_mod(QValue a, QValue b) {
    return qv_int(a.data.int_val %% b.data.int_val);
}

QValue q_pow(QValue a, QValue b) {
    double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
    double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
    double result = 1;
    for (int i = 0; i < (int)bv; i++) result *= av;
    return a.type == VAL_FLOAT || b.type == VAL_FLOAT ? qv_float(result) : qv_int((long long)result);
}

QValue q_neg(QValue a) {
    if (a.type == VAL_FLOAT) return qv_float(-a.data.float_val);
    return qv_int(-a.data.int_val);
}

// Comparison operations
QValue q_lt(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av < bv);
    }
    return qv_bool(a.data.int_val < b.data.int_val);
}

QValue q_lte(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av <= bv);
    }
    return qv_bool(a.data.int_val <= b.data.int_val);
}

QValue q_gt(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av > bv);
    }
    return qv_bool(a.data.int_val > b.data.int_val);
}

QValue q_gte(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av >= bv);
    }
    return qv_bool(a.data.int_val >= b.data.int_val);
}

QValue q_eq(QValue a, QValue b) {
    if (a.type != b.type) return qv_bool(false);
    switch (a.type) {
        case VAL_INT: return qv_bool(a.data.int_val == b.data.int_val);
        case VAL_FLOAT: return qv_bool(a.data.float_val == b.data.float_val);
        case VAL_BOOL: return qv_bool(a.data.bool_val == b.data.bool_val);
        case VAL_STRING: return qv_bool(strcmp(a.data.string_val, b.data.string_val) == 0);
        case VAL_NULL: return qv_bool(true);
        default: return qv_bool(false);
    }
}

QValue q_neq(QValue a, QValue b) {
    return qv_bool(!q_eq(a, b).data.bool_val);
}

// Logical operations
QValue q_and(QValue a, QValue b) {
    bool av = a.type == VAL_BOOL ? a.data.bool_val : (a.type == VAL_INT ? a.data.int_val != 0 : true);
    bool bv = b.type == VAL_BOOL ? b.data.bool_val : (b.type == VAL_INT ? b.data.int_val != 0 : true);
    return qv_bool(av && bv);
}

QValue q_or(QValue a, QValue b) {
    bool av = a.type == VAL_BOOL ? a.data.bool_val : (a.type == VAL_INT ? a.data.int_val != 0 : true);
    bool bv = b.type == VAL_BOOL ? b.data.bool_val : (b.type == VAL_INT ? b.data.int_val != 0 : true);
    return qv_bool(av || bv);
}

QValue q_not(QValue a) {
    bool av = a.type == VAL_BOOL ? a.data.bool_val : (a.type == VAL_INT ? a.data.int_val != 0 : true);
    return qv_bool(!av);
}

// Truthiness check
bool q_truthy(QValue v) {
    switch (v.type) {
        case VAL_BOOL: return v.data.bool_val;
        case VAL_INT: return v.data.int_val != 0;
        case VAL_FLOAT: return v.data.float_val != 0.0;
        case VAL_STRING: return v.data.string_val != NULL && strlen(v.data.string_val) > 0;
        case VAL_NULL: return false;
        default: return true;
    }
}

// Built-in functions
QValue q_len(QValue v) {
    switch (v.type) {
        case VAL_STRING: return qv_int((long long)strlen(v.data.string_val));
        case VAL_LIST: return qv_int(v.data.list_val.len);
        default: return qv_int(0);
    }
}

QValue q_input() {
    char buffer[4096];
    if (fgets(buffer, sizeof(buffer), stdin) != NULL) {
        // Remove trailing newline
        size_t len = strlen(buffer);
        if (len > 0 && buffer[len-1] == '\n') {
            buffer[len-1] = '\0';
        }
        return qv_string(buffer);
    }
    return qv_string("");
}

QValue q_str(QValue v) {
    char buffer[256];
    switch (v.type) {
        case VAL_INT:
            snprintf(buffer, sizeof(buffer), "%%lld", v.data.int_val);
            return qv_string(buffer);
        case VAL_FLOAT:
            snprintf(buffer, sizeof(buffer), "%%g", v.data.float_val);
            return qv_string(buffer);
        case VAL_BOOL:
            return qv_string(v.data.bool_val ? "true" : "false");
        case VAL_STRING:
            return v;
        case VAL_NULL:
            return qv_string("null");
        default:
            return qv_string("<value>");
    }
}

QValue q_int(QValue v) {
    switch (v.type) {
        case VAL_INT: return v;
        case VAL_FLOAT: return qv_int((long long)v.data.float_val);
        case VAL_BOOL: return qv_int(v.data.bool_val ? 1 : 0);
        case VAL_STRING: return qv_int(atoll(v.data.string_val));
        default: return qv_int(0);
    }
}

QValue q_float(QValue v) {
    switch (v.type) {
        case VAL_INT: return qv_float((double)v.data.int_val);
        case VAL_FLOAT: return v;
        case VAL_BOOL: return qv_float(v.data.bool_val ? 1.0 : 0.0);
        case VAL_STRING: return qv_float(atof(v.data.string_val));
        default: return qv_float(0.0);
    }
}

QValue q_bool(QValue v) {
    return qv_bool(q_truthy(v));
}

// Math module functions
QValue q_abs(QValue v) {
    if (v.type == VAL_FLOAT) return qv_float(fabs(v.data.float_val));
    return qv_int(llabs(v.data.int_val));
}

QValue q_min(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av < bv ? av : bv);
    }
    return qv_int(a.data.int_val < b.data.int_val ? a.data.int_val : b.data.int_val);
}

QValue q_max(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av > bv ? av : bv);
    }
    return qv_int(a.data.int_val > b.data.int_val ? a.data.int_val : b.data.int_val);
}

QValue q_sqrt(QValue v) {
    double val = v.type == VAL_FLOAT ? v.data.float_val : (double)v.data.int_val;
    return qv_float(sqrt(val));
}

QValue q_floor(QValue v) {
    if (v.type == VAL_INT) return v;
    return qv_int((long long)floor(v.data.float_val));
}

QValue q_ceil(QValue v) {
    if (v.type == VAL_INT) return v;
    return qv_int((long long)ceil(v.data.float_val));
}

QValue q_round(QValue v) {
    if (v.type == VAL_INT) return v;
    return qv_int((long long)round(v.data.float_val));
}

// String module functions
QValue q_upper(QValue v) {
    if (v.type != VAL_STRING) return qv_string("");
    char* result = strdup(v.data.string_val);
    for (int i = 0; result[i]; i++) result[i] = toupper(result[i]);
    QValue q = qv_string(result);
    free(result);
    return q;
}

QValue q_lower(QValue v) {
    if (v.type != VAL_STRING) return qv_string("");
    char* result = strdup(v.data.string_val);
    for (int i = 0; result[i]; i++) result[i] = tolower(result[i]);
    QValue q = qv_string(result);
    free(result);
    return q;
}

QValue q_trim(QValue v) {
    if (v.type != VAL_STRING) return qv_string("");
    const char* start = v.data.string_val;
    while (*start && isspace(*start)) start++;
    if (*start == '\0') return qv_string("");
    const char* end = v.data.string_val + strlen(v.data.string_val) - 1;
    while (end > start && isspace(*end)) end--;
    size_t len = end - start + 1;
    char* result = malloc(len + 1);
    strncpy(result, start, len);
    result[len] = '\0';
    QValue q = qv_string(result);
    free(result);
    return q;
}

QValue q_contains(QValue str, QValue sub) {
    if (str.type != VAL_STRING || sub.type != VAL_STRING) return qv_bool(false);
    return qv_bool(strstr(str.data.string_val, sub.data.string_val) != NULL);
}

QValue q_startswith(QValue str, QValue prefix) {
    if (str.type != VAL_STRING || prefix.type != VAL_STRING) return qv_bool(false);
    size_t plen = strlen(prefix.data.string_val);
    return qv_bool(strncmp(str.data.string_val, prefix.data.string_val, plen) == 0);
}

QValue q_endswith(QValue str, QValue suffix) {
    if (str.type != VAL_STRING || suffix.type != VAL_STRING) return qv_bool(false);
    size_t slen = strlen(str.data.string_val);
    size_t suflen = strlen(suffix.data.string_val);
    if (suflen > slen) return qv_bool(false);
    return qv_bool(strcmp(str.data.string_val + slen - suflen, suffix.data.string_val) == 0);
}

QValue q_replace(QValue str, QValue old, QValue new_str) {
    if (str.type != VAL_STRING || old.type != VAL_STRING || new_str.type != VAL_STRING)
        return qv_string("");
    const char* s = str.data.string_val;
    const char* o = old.data.string_val;
    const char* n = new_str.data.string_val;
    size_t olen = strlen(o);
    size_t nlen = strlen(n);
    if (olen == 0) return str;

    // Count occurrences
    int count = 0;
    const char* tmp = s;
    while ((tmp = strstr(tmp, o)) != NULL) { count++; tmp += olen; }

    // Allocate result
    size_t rlen = strlen(s) + count * (nlen - olen);
    char* result = malloc(rlen + 1);
    char* dest = result;

    while (*s) {
        if (strncmp(s, o, olen) == 0) {
            strcpy(dest, n);
            dest += nlen;
            s += olen;
        } else {
            *dest++ = *s++;
        }
    }
    *dest = '\0';

    QValue q = qv_string(result);
    free(result);
    return q;
}

QValue q_concat(QValue a, QValue b) {
    if (a.type != VAL_STRING || b.type != VAL_STRING) return qv_string("");
    size_t len = strlen(a.data.string_val) + strlen(b.data.string_val);
    char* result = malloc(len + 1);
    strcpy(result, a.data.string_val);
    strcat(result, b.data.string_val);
    QValue q = qv_string(result);
    free(result);
    return q;
}

// Forward declarations
`)

	// First pass: collect function declarations
	g.collectFunctions(node)

	// Emit forward declarations
	for _, fname := range g.functions {
		g.emitLine("QValue q_%s();", fname)
	}
	g.emit("\n")

	// Generate function definitions
	g.generateNode(node)

	// Generate main function
	g.emit("\nint main() {\n")
	g.indentLevel++

	// Generate top-level statements that aren't function/module definitions
	for _, child := range node.Children {
		if child.NodeType != ast.FunctionNode && child.NodeType != ast.ModuleNode && child.NodeType != ast.UseNode {
			g.emitLine("%s;", g.generateExpr(child))
		}
	}

	g.emitLine("return 0;")
	g.indentLevel--
	g.emit("}\n")

	return g.output.String()
}

func (g *Generator) collectFunctions(node *ast.TreeNode) {
	switch node.NodeType {
	case ast.FunctionNode:
		if len(node.Children) >= 1 {
			name := node.Children[0].TokenLiteral()
			// No module prefix - all functions are global in C
			// Modules are just a grouping mechanism in Quark
			g.functions = append(g.functions, name)
		}
	case ast.LambdaNode:
		// Assign a unique name to this lambda
		lambdaName := g.newLambda()
		g.lambdaNames[node] = lambdaName
		g.lambdas = append(g.lambdas, node)
		g.functions = append(g.functions, lambdaName)
	case ast.ModuleNode:
		if len(node.Children) >= 2 {
			bodyNode := node.Children[1]
			// Collect functions from module body (without prefix)
			g.collectFunctions(bodyNode)
		}
		return // Don't recurse further, we handled the module body
	}
	for _, child := range node.Children {
		g.collectFunctions(child)
	}
}

func (g *Generator) generateNode(node *ast.TreeNode) {
	switch node.NodeType {
	case ast.CompilationUnitNode:
		for _, child := range node.Children {
			g.generateNode(child)
		}
		// Generate all collected lambdas
		for _, lambda := range g.lambdas {
			g.generateLambdaFunc(lambda)
		}
	case ast.FunctionNode:
		g.generateFunction(node)
	case ast.ModuleNode:
		g.generateModule(node)
	}
}

func (g *Generator) generateFunction(node *ast.TreeNode) {
	if len(node.Children) < 3 {
		return
	}

	nameNode := node.Children[0]
	argsNode := node.Children[1]
	bodyNode := node.Children[2]

	funcName := nameNode.TokenLiteral()
	g.currentFunc = funcName
	g.inFunction = true

	// Build parameter list
	params := make([]string, 0)
	for _, param := range argsNode.Children {
		params = append(params, fmt.Sprintf("QValue %s", param.TokenLiteral()))
	}

	g.emit("QValue q_%s(%s) {\n", funcName, strings.Join(params, ", "))
	g.indentLevel++

	// Generate body
	result := g.generateBlock(bodyNode)
	g.emitLine("return %s;", result)

	g.indentLevel--
	g.emit("}\n\n")

	g.inFunction = false
}

func (g *Generator) generateModule(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}

	bodyNode := node.Children[1]

	// Generate module functions (all functions are global in the C output)
	for _, child := range bodyNode.Children {
		if child.NodeType == ast.FunctionNode {
			g.generateFunction(child)
		}
	}
}

func (g *Generator) generateBlock(node *ast.TreeNode) string {
	var lastExpr string = "qv_null()"
	for _, child := range node.Children {
		lastExpr = g.generateExpr(child)
		// Only emit as statement if it's not the last expression
		if child != node.Children[len(node.Children)-1] {
			g.emitLine("%s;", lastExpr)
		}
	}
	return lastExpr
}

func (g *Generator) generateExpr(node *ast.TreeNode) string {
	if node == nil {
		return "qv_null()"
	}

	switch node.NodeType {
	case ast.LiteralNode:
		return g.generateLiteral(node)
	case ast.IdentifierNode:
		return g.generateIdentifier(node)
	case ast.OperatorNode:
		return g.generateOperator(node)
	case ast.FunctionCallNode:
		return g.generateFunctionCall(node)
	case ast.PipeNode:
		return g.generatePipe(node)
	case ast.TernaryNode:
		return g.generateTernary(node)
	case ast.IfStatementNode:
		return g.generateIf(node)
	case ast.WhenStatementNode:
		return g.generateWhen(node)
	case ast.ForLoopNode:
		return g.generateFor(node)
	case ast.WhileLoopNode:
		return g.generateWhile(node)
	case ast.ListNode:
		return g.generateList(node)
	case ast.IndexNode:
		return g.generateIndex(node)
	case ast.LambdaNode:
		return g.generateLambdaExpr(node)
	case ast.BlockNode:
		return g.generateBlock(node)
	case ast.ModuleNode:
		// Module definitions are handled at top level
		return "qv_null()"
	case ast.UseNode:
		// Use statements are handled at compile time (imports are resolved by analyzer)
		return "qv_null()"
	default:
		return "qv_null()"
	}
}

func (g *Generator) generateLiteral(node *ast.TreeNode) string {
	if node.Token == nil {
		return "qv_null()"
	}

	switch node.Token.Type {
	case token.INT:
		return fmt.Sprintf("qv_int(%s)", node.Token.Literal)
	case token.FLOAT:
		return fmt.Sprintf("qv_float(%s)", node.Token.Literal)
	case token.STRING:
		// Escape the string properly
		escaped := strings.ReplaceAll(node.Token.Literal, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		return fmt.Sprintf("qv_string(\"%s\")", escaped)
	case token.TRUE:
		return "qv_bool(true)"
	case token.FALSE:
		return "qv_bool(false)"
	case token.NULL:
		return "qv_null()"
	default:
		return "qv_null()"
	}
}

func (g *Generator) generateIdentifier(node *ast.TreeNode) string {
	name := node.TokenLiteral()
	if name == "_" {
		return "qv_null()"
	}
	return name
}

func (g *Generator) generateOperator(node *ast.TreeNode) string {
	if node.Token == nil {
		return "qv_null()"
	}

	op := node.Token.Type

	// Unary operators
	if len(node.Children) == 1 {
		operand := g.generateExpr(node.Children[0])
		switch op {
		case token.MINUS:
			return fmt.Sprintf("q_neg(%s)", operand)
		case token.BANG, token.NOT:
			return fmt.Sprintf("q_not(%s)", operand)
		}
		return operand
	}

	// Binary operators
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	left := g.generateExpr(node.Children[0])
	right := g.generateExpr(node.Children[1])

	switch op {
	case token.PLUS:
		return fmt.Sprintf("q_add(%s, %s)", left, right)
	case token.MINUS:
		return fmt.Sprintf("q_sub(%s, %s)", left, right)
	case token.MULTIPLY:
		return fmt.Sprintf("q_mul(%s, %s)", left, right)
	case token.DIVIDE:
		return fmt.Sprintf("q_div(%s, %s)", left, right)
	case token.MODULO:
		return fmt.Sprintf("q_mod(%s, %s)", left, right)
	case token.DOUBLESTAR:
		return fmt.Sprintf("q_pow(%s, %s)", left, right)
	case token.LT:
		return fmt.Sprintf("q_lt(%s, %s)", left, right)
	case token.LTE:
		return fmt.Sprintf("q_lte(%s, %s)", left, right)
	case token.GT:
		return fmt.Sprintf("q_gt(%s, %s)", left, right)
	case token.GTE:
		return fmt.Sprintf("q_gte(%s, %s)", left, right)
	case token.DEQ:
		return fmt.Sprintf("q_eq(%s, %s)", left, right)
	case token.NE:
		return fmt.Sprintf("q_neq(%s, %s)", left, right)
	case token.AND:
		return fmt.Sprintf("q_and(%s, %s)", left, right)
	case token.OR:
		return fmt.Sprintf("q_or(%s, %s)", left, right)
	case token.EQUALS:
		// Assignment - emit as statement and return the value
		varName := node.Children[0].TokenLiteral()
		g.emitLine("QValue %s = %s;", varName, right)
		return varName
	case token.DOTDOT:
		// Range - used in for loops, not directly as a value
		return fmt.Sprintf("/* range %s..%s */", left, right)
	}

	return "qv_null()"
}

func (g *Generator) generateFunctionCall(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]

	funcName := funcNode.TokenLiteral()

	// Generate arguments
	args := make([]string, 0)
	for _, arg := range argsNode.Children {
		args = append(args, g.generateExpr(arg))
	}

	// Built-in functions
	switch funcName {
	case "print":
		if len(args) > 0 {
			return fmt.Sprintf("q_print(%s)", args[0])
		}
		return "q_print(qv_string(\"\"))"
	case "println":
		if len(args) > 0 {
			return fmt.Sprintf("q_println(%s)", args[0])
		}
		return "q_println(qv_string(\"\"))"
	case "len":
		if len(args) > 0 {
			return fmt.Sprintf("q_len(%s)", args[0])
		}
		return "qv_int(0)"
	case "input":
		return "q_input()"
	case "str":
		if len(args) > 0 {
			return fmt.Sprintf("q_str(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "int":
		if len(args) > 0 {
			return fmt.Sprintf("q_int(%s)", args[0])
		}
		return "qv_int(0)"
	case "float":
		if len(args) > 0 {
			return fmt.Sprintf("q_float(%s)", args[0])
		}
		return "qv_float(0.0)"
	case "bool":
		if len(args) > 0 {
			return fmt.Sprintf("q_bool(%s)", args[0])
		}
		return "qv_bool(false)"
	// Math module functions
	case "abs":
		if len(args) > 0 {
			return fmt.Sprintf("q_abs(%s)", args[0])
		}
		return "qv_int(0)"
	case "min":
		if len(args) >= 2 {
			return fmt.Sprintf("q_min(%s, %s)", args[0], args[1])
		}
		return "qv_int(0)"
	case "max":
		if len(args) >= 2 {
			return fmt.Sprintf("q_max(%s, %s)", args[0], args[1])
		}
		return "qv_int(0)"
	case "sqrt":
		if len(args) > 0 {
			return fmt.Sprintf("q_sqrt(%s)", args[0])
		}
		return "qv_float(0.0)"
	case "floor":
		if len(args) > 0 {
			return fmt.Sprintf("q_floor(%s)", args[0])
		}
		return "qv_int(0)"
	case "ceil":
		if len(args) > 0 {
			return fmt.Sprintf("q_ceil(%s)", args[0])
		}
		return "qv_int(0)"
	case "round":
		if len(args) > 0 {
			return fmt.Sprintf("q_round(%s)", args[0])
		}
		return "qv_int(0)"
	// String module functions
	case "upper":
		if len(args) > 0 {
			return fmt.Sprintf("q_upper(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "lower":
		if len(args) > 0 {
			return fmt.Sprintf("q_lower(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "trim":
		if len(args) > 0 {
			return fmt.Sprintf("q_trim(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "contains":
		if len(args) >= 2 {
			return fmt.Sprintf("q_contains(%s, %s)", args[0], args[1])
		}
		return "qv_bool(false)"
	case "startswith":
		if len(args) >= 2 {
			return fmt.Sprintf("q_startswith(%s, %s)", args[0], args[1])
		}
		return "qv_bool(false)"
	case "endswith":
		if len(args) >= 2 {
			return fmt.Sprintf("q_endswith(%s, %s)", args[0], args[1])
		}
		return "qv_bool(false)"
	case "replace":
		if len(args) >= 3 {
			return fmt.Sprintf("q_replace(%s, %s, %s)", args[0], args[1], args[2])
		}
		return "qv_string(\"\")"
	case "concat":
		if len(args) >= 2 {
			return fmt.Sprintf("q_concat(%s, %s)", args[0], args[1])
		}
		return "qv_string(\"\")"
	// List functions
	case "push":
		if len(args) >= 2 {
			return fmt.Sprintf("q_push(%s, %s)", args[0], args[1])
		}
		return "qv_null()"
	case "pop":
		if len(args) >= 1 {
			return fmt.Sprintf("q_pop(%s)", args[0])
		}
		return "qv_null()"
	case "get":
		if len(args) >= 2 {
			return fmt.Sprintf("q_get(%s, %s)", args[0], args[1])
		}
		return "qv_null()"
	case "set":
		if len(args) >= 3 {
			return fmt.Sprintf("q_set(%s, %s, %s)", args[0], args[1], args[2])
		}
		return "qv_null()"
	}

	// Check if this is a known user-defined function
	isKnownFunc := false
	for _, fname := range g.functions {
		if fname == funcName {
			isKnownFunc = true
			break
		}
	}

	if isKnownFunc {
		// User-defined function - call directly
		return fmt.Sprintf("q_%s(%s)", funcName, strings.Join(args, ", "))
	}

	// Otherwise, it might be a function value - use dynamic call
	funcExpr := g.generateExpr(funcNode)
	switch len(args) {
	case 0:
		return fmt.Sprintf("q_call0(%s)", funcExpr)
	case 1:
		return fmt.Sprintf("q_call1(%s, %s)", funcExpr, args[0])
	case 2:
		return fmt.Sprintf("q_call2(%s, %s, %s)", funcExpr, args[0], args[1])
	case 3:
		return fmt.Sprintf("q_call3(%s, %s, %s, %s)", funcExpr, args[0], args[1], args[2])
	case 4:
		return fmt.Sprintf("q_call4(%s, %s, %s, %s, %s)", funcExpr, args[0], args[1], args[2], args[3])
	default:
		// For more than 4 args, fall back to direct call (won't work for function values)
		return fmt.Sprintf("q_%s(%s)", funcName, strings.Join(args, ", "))
	}
}

func (g *Generator) generatePipe(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	// Left side is input value
	input := g.generateExpr(node.Children[0])

	// Right side is function or function call
	rightNode := node.Children[1]

	if rightNode.NodeType == ast.IdentifierNode {
		// Simple function name - call with input as argument
		funcName := rightNode.TokenLiteral()
		switch funcName {
		case "print":
			return fmt.Sprintf("q_print(%s)", input)
		case "println":
			return fmt.Sprintf("q_println(%s)", input)
		case "len":
			return fmt.Sprintf("q_len(%s)", input)
		case "str":
			return fmt.Sprintf("q_str(%s)", input)
		case "int":
			return fmt.Sprintf("q_int(%s)", input)
		case "float":
			return fmt.Sprintf("q_float(%s)", input)
		case "bool":
			return fmt.Sprintf("q_bool(%s)", input)
		// Math functions
		case "abs":
			return fmt.Sprintf("q_abs(%s)", input)
		case "sqrt":
			return fmt.Sprintf("q_sqrt(%s)", input)
		case "floor":
			return fmt.Sprintf("q_floor(%s)", input)
		case "ceil":
			return fmt.Sprintf("q_ceil(%s)", input)
		case "round":
			return fmt.Sprintf("q_round(%s)", input)
		// String functions
		case "upper":
			return fmt.Sprintf("q_upper(%s)", input)
		case "lower":
			return fmt.Sprintf("q_lower(%s)", input)
		case "trim":
			return fmt.Sprintf("q_trim(%s)", input)
		default:
			return fmt.Sprintf("q_%s(%s)", funcName, input)
		}
	} else if rightNode.NodeType == ast.FunctionCallNode {
		// Function call - prepend input to arguments
		if len(rightNode.Children) >= 2 {
			funcNode := rightNode.Children[0]
			argsNode := rightNode.Children[1]

			funcName := funcNode.TokenLiteral()
			args := []string{input}
			for _, arg := range argsNode.Children {
				args = append(args, g.generateExpr(arg))
			}

			switch funcName {
			case "print":
				return fmt.Sprintf("q_print(%s)", args[0])
			case "println":
				return fmt.Sprintf("q_println(%s)", args[0])
			case "len":
				return fmt.Sprintf("q_len(%s)", args[0])
			case "str":
				return fmt.Sprintf("q_str(%s)", args[0])
			case "int":
				return fmt.Sprintf("q_int(%s)", args[0])
			case "float":
				return fmt.Sprintf("q_float(%s)", args[0])
			case "bool":
				return fmt.Sprintf("q_bool(%s)", args[0])
			// Math functions
			case "abs":
				return fmt.Sprintf("q_abs(%s)", args[0])
			case "min":
				if len(args) >= 2 {
					return fmt.Sprintf("q_min(%s, %s)", args[0], args[1])
				}
				return "qv_int(0)"
			case "max":
				if len(args) >= 2 {
					return fmt.Sprintf("q_max(%s, %s)", args[0], args[1])
				}
				return "qv_int(0)"
			case "sqrt":
				return fmt.Sprintf("q_sqrt(%s)", args[0])
			case "floor":
				return fmt.Sprintf("q_floor(%s)", args[0])
			case "ceil":
				return fmt.Sprintf("q_ceil(%s)", args[0])
			case "round":
				return fmt.Sprintf("q_round(%s)", args[0])
			// String functions
			case "upper":
				return fmt.Sprintf("q_upper(%s)", args[0])
			case "lower":
				return fmt.Sprintf("q_lower(%s)", args[0])
			case "trim":
				return fmt.Sprintf("q_trim(%s)", args[0])
			case "contains":
				if len(args) >= 2 {
					return fmt.Sprintf("q_contains(%s, %s)", args[0], args[1])
				}
				return "qv_bool(false)"
			case "startswith":
				if len(args) >= 2 {
					return fmt.Sprintf("q_startswith(%s, %s)", args[0], args[1])
				}
				return "qv_bool(false)"
			case "endswith":
				if len(args) >= 2 {
					return fmt.Sprintf("q_endswith(%s, %s)", args[0], args[1])
				}
				return "qv_bool(false)"
			case "replace":
				if len(args) >= 3 {
					return fmt.Sprintf("q_replace(%s, %s, %s)", args[0], args[1], args[2])
				}
				return "qv_string(\"\")"
			case "concat":
				if len(args) >= 2 {
					return fmt.Sprintf("q_concat(%s, %s)", args[0], args[1])
				}
				return "qv_string(\"\")"
			default:
				return fmt.Sprintf("q_%s(%s)", funcName, strings.Join(args, ", "))
			}
		}
	}

	return g.generateExpr(rightNode)
}

func (g *Generator) generateTernary(node *ast.TreeNode) string {
	if len(node.Children) < 3 {
		return "qv_null()"
	}

	cond := g.generateExpr(node.Children[0])
	trueVal := g.generateExpr(node.Children[1])
	falseVal := g.generateExpr(node.Children[2])

	return fmt.Sprintf("(q_truthy(%s) ? %s : %s)", cond, trueVal, falseVal)
}

func (g *Generator) generateIf(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	temp := g.newTemp()
	g.emitLine("QValue %s;", temp)

	cond := g.generateExpr(node.Children[0])
	g.emitLine("if (q_truthy(%s)) {", cond)
	g.indentLevel++

	ifResult := g.generateExpr(node.Children[1])
	g.emitLine("%s = %s;", temp, ifResult)

	g.indentLevel--
	g.emit(g.indent() + "}")

	// Handle elseif/else
	for i := 2; i < len(node.Children); i++ {
		child := node.Children[i]
		if child.NodeType == ast.IfStatementNode && len(child.Children) >= 2 {
			// elseif
			g.emit(" else if (q_truthy(%s)) {\n", g.generateExpr(child.Children[0]))
			g.indentLevel++
			elseifResult := g.generateExpr(child.Children[1])
			g.emitLine("%s = %s;", temp, elseifResult)
			g.indentLevel--
			g.emit(g.indent() + "}")
		} else {
			// else
			g.emit(" else {\n")
			g.indentLevel++
			elseResult := g.generateExpr(child)
			g.emitLine("%s = %s;", temp, elseResult)
			g.indentLevel--
			g.emit(g.indent() + "}")
		}
	}
	g.emit("\n")

	return temp
}

func (g *Generator) generateWhen(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	temp := g.newTemp()
	matchExpr := g.generateExpr(node.Children[0])
	matchTemp := g.newTemp()

	g.emitLine("QValue %s;", temp)
	g.emitLine("QValue %s = %s;", matchTemp, matchExpr)

	first := true
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) < 2 {
			continue
		}

		// Last child is the result, others are patterns
		resultIdx := len(pattern.Children) - 1
		result := g.generateExpr(pattern.Children[resultIdx])

		// Build condition from patterns
		conditions := make([]string, 0)
		for j := 0; j < resultIdx; j++ {
			patternExpr := pattern.Children[j]
			if patternExpr.NodeType == ast.IdentifierNode && patternExpr.TokenLiteral() == "_" {
				// Wildcard matches everything
				conditions = append(conditions, "true")
			} else {
				patternVal := g.generateExpr(patternExpr)
				conditions = append(conditions, fmt.Sprintf("q_eq(%s, %s).data.bool_val", matchTemp, patternVal))
			}
		}

		condStr := strings.Join(conditions, " || ")
		if first {
			g.emitLine("if (%s) {", condStr)
			first = false
		} else {
			g.emit(g.indent() + "} else if (%s) {\n", condStr)
		}
		g.indentLevel++
		g.emitLine("%s = %s;", temp, result)
		g.indentLevel--
	}

	if !first {
		g.emitLine("}")
	}

	return temp
}

func (g *Generator) generateFor(node *ast.TreeNode) string {
	if len(node.Children) < 3 {
		return "qv_null()"
	}

	varNode := node.Children[0]
	rangeNode := node.Children[1]
	bodyNode := node.Children[2]

	varName := varNode.TokenLiteral()

	// Handle range expression
	if rangeNode.NodeType == ast.OperatorNode && rangeNode.Token != nil && rangeNode.Token.Type == token.DOTDOT {
		startExpr := g.generateExpr(rangeNode.Children[0])
		endExpr := g.generateExpr(rangeNode.Children[1])

		startTemp := g.newTemp()
		endTemp := g.newTemp()

		g.emitLine("long long %s = %s.data.int_val;", startTemp, startExpr)
		g.emitLine("long long %s = %s.data.int_val;", endTemp, endExpr)
		g.emitLine("for (long long _i = %s; _i < %s; _i++) {", startTemp, endTemp)
		g.indentLevel++
		g.emitLine("QValue %s = qv_int(_i);", varName)

		// Generate body - emit each statement
		if bodyNode.NodeType == ast.BlockNode {
			for _, stmt := range bodyNode.Children {
				expr := g.generateExpr(stmt)
				g.emitLine("%s;", expr)
			}
		} else {
			expr := g.generateExpr(bodyNode)
			g.emitLine("%s;", expr)
		}

		g.indentLevel--
		g.emitLine("}")
	}

	return "qv_null()"
}

func (g *Generator) generateWhile(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	condNode := node.Children[0]
	bodyNode := node.Children[1]

	g.emitLine("while (q_truthy(%s)) {", g.generateExpr(condNode))
	g.indentLevel++

	// Generate body
	if bodyNode.NodeType == ast.BlockNode {
		for _, stmt := range bodyNode.Children {
			expr := g.generateExpr(stmt)
			g.emitLine("%s;", expr)
		}
	} else {
		expr := g.generateExpr(bodyNode)
		g.emitLine("%s;", expr)
	}

	g.indentLevel--
	g.emitLine("}")

	return "qv_null()"
}

func (g *Generator) generateList(node *ast.TreeNode) string {
	if len(node.Children) == 0 {
		return "qv_list(8)"
	}

	// Generate list with initial elements
	temp := g.newTemp()
	g.emitLine("QValue %s = qv_list(%d);", temp, len(node.Children))

	for _, child := range node.Children {
		elem := g.generateExpr(child)
		g.emitLine("%s = q_push(%s, %s);", temp, temp, elem)
	}

	return temp
}

func (g *Generator) generateIndex(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	target := g.generateExpr(node.Children[0])
	index := g.generateExpr(node.Children[1])

	return fmt.Sprintf("q_get(%s, %s)", target, index)
}

func (g *Generator) generateLambdaExpr(node *ast.TreeNode) string {
	// Look up the lambda name we assigned during collection
	lambdaName, ok := g.lambdaNames[node]
	if !ok {
		// Lambda wasn't collected - this shouldn't happen but handle it
		lambdaName = g.newLambda()
		g.lambdaNames[node] = lambdaName
		g.lambdas = append(g.lambdas, node)
		g.functions = append(g.functions, lambdaName)
	}

	// Return a function value wrapping the lambda
	return fmt.Sprintf("qv_func((void*)q_%s)", lambdaName)
}

func (g *Generator) generateLambdaFunc(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}

	lambdaName := g.lambdaNames[node]
	argsNode := node.Children[0]
	bodyNode := node.Children[1]

	g.inFunction = true
	g.currentFunc = lambdaName

	// Build parameter list
	params := make([]string, 0)
	for _, param := range argsNode.Children {
		params = append(params, fmt.Sprintf("QValue %s", param.TokenLiteral()))
	}

	g.emit("QValue q_%s(%s) {\n", lambdaName, strings.Join(params, ", "))
	g.indentLevel++

	// Generate body - for lambdas, the body is a single expression
	result := g.generateExpr(bodyNode)
	g.emitLine("return %s;", result)

	g.indentLevel--
	g.emit("}\n\n")

	g.inFunction = false
}
