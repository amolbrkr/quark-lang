package types

import (
	"strconv"
	"strings"

	"quark/ast"
	"quark/names"
	"quark/token"
)

// Tables (tables_v0_1_spec.md): expected types, the std/csv reads typed by
// them, and anonymous rows.

// TableRead is a call of csv::read_file or csv::parse_text. Lowering calls
// the native reader Native directly, with Row's definition when the
// expected type named the row (D18), so the file is read with its columns.
type TableRead struct {
	Native string
	Row    *StructType // nil: column types inferred from the file
}

// tableReaders are std/csv's reads by function name, with the native
// function each one calls.
var tableReaders = map[string]string{
	"read_file":  "q_csv_read_file",
	"parse_text": "q_csv_parse_text",
}

// expect records that node's value is stored where a value of type t is
// expected (an annotated variable, a parameter, a struct field, a return
// annotation), so a call that can make a value of that type takes the type
// from it (D18). Only calls consume it, and only csv::read_file and
// csv::parse_text do so far; unwrap passes it on to its argument.
func (a *Analyzer) expect(node *ast.TreeNode, t Type) {
	if node == nil || t == nil || isUnknownType(t) {
		return
	}
	a.expected[node] = t
}

// expectArgs passes a call's parameter types down to its arguments, before
// they are analyzed, when the callee's signature is known by name.
func (a *Analyzer) expectArgs(call, funcNode *ast.TreeNode, argNodes []*ast.TreeNode) {
	if funcNode == nil || funcNode.NodeType != ast.IdentifierNode {
		return
	}
	name := funcNode.TokenLiteral()
	if name == "unwrap" && a.refersToBuiltin(a.currentScope, name) && len(argNodes) == 1 {
		if want := a.expected[call]; want != nil {
			a.expect(argNodes[0], &ResultType{OkType: want, ErrType: TypeAny})
		}
		return
	}
	sym := a.currentScope.Lookup(name)
	if sym == nil {
		return
	}
	ft, ok := sym.Type.(*FunctionType)
	if !ok {
		return
	}
	for i, arg := range argNodes {
		if i < len(ft.ParamTypes) {
			a.expect(arg, ft.ParamTypes[i])
		}
	}
}

// lastValue is the node whose value a function body returns: the last
// statement of a block, or the body itself.
func lastValue(body *ast.TreeNode) *ast.TreeNode {
	if body != nil && body.NodeType == ast.BlockNode {
		if len(body.Children) == 0 {
			return nil
		}
		return body.Children[len(body.Children)-1]
	}
	return body
}

// tableReaderOf is the native reader of a call of csv::read_file or
// csv::parse_text from std/csv, or "".
func (a *Analyzer) tableReaderOf(funcNode *ast.TreeNode) string {
	if funcNode == nil || funcNode.NodeType != ast.IdentifierNode {
		return ""
	}
	o := a.names[funcNode.TokenLiteral()]
	if o == nil || o.File == nil || o.File.Display != "std/csv.qrk" || o.Module != "csv" || o.Kind != names.Func {
		return ""
	}
	return tableReaders[o.Name]
}

// tableRead types a csv::read_file or csv::parse_text call (D18): with an
// expected result[tbl[R], ...] the file is read with R's columns and the
// call is a result[tbl[R], str]; otherwise the column types come from the
// file and the table's row is unknown (tbl).
func (a *Analyzer) tableRead(node *ast.TreeNode, native string, argNodes []*ast.TreeNode) Type {
	var row *StructType
	if rt, ok := a.expected[node].(*ResultType); ok {
		if tt, ok := rt.OkType.(*TableType); ok {
			row = tt.Row
		}
	}
	if len(argNodes) > 1 {
		a.checkColumnMapping(argNodes[1], row)
	}
	a.tableReads[node] = &TableRead{Native: native, Row: row}
	return &ResultType{OkType: &TableType{Row: row}, ErrType: TypeString}
}

// checkColumnMapping checks the D17 mapping of a read: a dict literal from
// field names to header names (string literals). With a row struct, each
// key must be one of its fields.
func (a *Analyzer) checkColumnMapping(arg *ast.TreeNode, row *StructType) {
	if arg.NodeType != ast.DictNode {
		a.errorAt(arg, "the column mapping must be a dict literal, dict { field: 'Header' }, so its fields can be checked against the row struct")
		return
	}
	readBy := map[string]string{}
	for _, pair := range arg.Children {
		if pair == nil || len(pair.Children) < 2 {
			continue
		}
		field := pair.Children[0].TokenLiteral()
		val := pair.Children[1]
		if val.NodeType != ast.LiteralNode || val.Token == nil || val.Token.Type != token.STRING {
			a.errorAt(val, "the column mapping maps field '%s' to a header name, which must be a string literal", field)
			continue
		}
		if row != nil {
			if f, _ := row.FieldByName(field); f == nil {
				a.errorAt(pair.Children[0], "the column mapping names '%s', but %s has no field '%s' (fields: %s)", field, row.String(), field, columnList(row))
				continue
			}
		}
		header := val.Token.Literal
		if other, dup := readBy[header]; dup {
			a.errorAt(val, "fields '%s' and '%s' both read the header column '%s'", other, field, header)
			continue
		}
		readBy[header] = field
	}
}

// anonRow is the anonymous row type with these fields, in this order: one
// StructType per shape, registered with the struct types so lowering emits
// its definition (tables_v0_1_spec.md §3.2).
func (a *Analyzer) anonRow(fields []StructField) *StructType {
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = f.Name + ":" + typeKey(f.Type)
	}
	key := strings.Join(parts, ",")
	if st, ok := a.anonRows[key]; ok {
		return st
	}
	st := &StructType{Name: "row__" + strconv.Itoa(len(a.anonRows)+1), Fields: fields, Anonymous: true}
	a.anonRows[key] = st
	a.structTypes[st.Name] = st
	return st
}
