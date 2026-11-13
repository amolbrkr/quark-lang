# Quark Structs v0.1 Spec

Status: Implemented. Structs landed on 2026-04-02; the modules v0.2 rules for structs (qualified names, structs in module bodies, identity by definition), int→float widening of fields and checks on field values of unknown type landed on 2026-11-12; the `null` rules for field types on 2026-11-13. Not implemented: structs defined inside a function body (§9). Tables typed by a struct (`tbl[Employee]`) are specified, not implemented, in `tables_v0_1_spec.md`. There is no separate schema type: it was folded into `struct` (decided 2026-11-12).
Last Updated: 2026-11-13
Scope: struct declarations, literals, field access, and how structs interact with modules, annotations, containers and tables.

## Contents

- [1. Purpose](#1-purpose)
- [2. Design Rules](#2-design-rules)
- [3. Syntax](#3-syntax)
  - [3.1 Declaration](#31-declaration)
  - [3.2 Construction](#32-construction)
  - [3.3 Update by reconstruction](#33-update-by-reconstruction)
  - [3.4 Grammar](#34-grammar)
- [4. Where Structs Live and How They Are Named](#4-where-structs-live-and-how-they-are-named)
- [5. Static Semantics](#5-static-semantics)
  - [5.1 Type identity](#51-type-identity)
  - [5.2 Field types](#52-field-types)
  - [5.3 Default values](#53-default-values)
  - [5.4 Construction checks](#54-construction-checks)
  - [5.5 Field access](#55-field-access)
  - [5.6 Immutability](#56-immutability)
  - [5.7 Equality](#57-equality)
- [6. Runtime Semantics](#6-runtime-semantics)
- [7. Diagnostics](#7-diagnostics)
- [8. Interaction with Other Features](#8-interaction-with-other-features)
  - [8.1 Annotations, parameters and returns](#81-annotations-parameters-and-returns)
  - [8.2 Containers](#82-containers)
  - [8.3 Tables](#83-tables)
- [9. Limitations](#9-limitations)
- [10. Tests](#10-tests)

## 1. Purpose

A struct is a fixed-shape, typed record: the domain model between dynamic `dict` values and columnar tables (customer, order, job and config records, and the row type of a table). The design keeps the language surface small and checks field shape and types at compile time wherever it can.

## 2. Design Rules

- Structs are nominal, immutable values with named, typed fields.
- No methods, `impl` blocks, inheritance, embedding, traits or generic structs.
- No destructuring patterns and no field assignment (`obj.field = ...`); a changed value is a new literal (§3.3).
- `struct` and `dict` stay separate types (decided 2026-11-12): a dict is a dynamic map, a struct a checked record.

## 3. Syntax

### 3.1 Declaration

```quark
struct Customer:
    id: int
    name: str
    age: int = 0
    city: str
```

- `struct` introduces a named type. The body contains only field declarations.
- A field is `name: Type` with an optional `= default` (§5.3).
- Field names are unique within the struct. Like every user name, they can't contain `__` (§9).

### 3.2 Construction

```quark
c1 = Customer {
    id: 1
    name: 'Alice'
    city: 'NYC'
}
c2 = Customer { id: 2, name: 'Bob', city: 'SF' }
```

- Literals are named-field only, inline (comma-separated) or one field per line.
- Missing required fields, unknown fields and duplicate fields are compile-time errors. Omitted fields with a default get it.

### 3.3 Update by reconstruction

```quark
c3 = Customer {
    id: c1.id
    name: c1.name
    age: c1.age + 1
    city: 'SF'
}
```

A changed value is built with the same literal form; there is no update keyword or helper.

### 3.4 Grammar

```ebnf
StructDef        ::= "struct" ID ":" NEWLINE INDENT
                     FieldDecl { NEWLINE FieldDecl } NEWLINE?
                     DEDENT
FieldDecl        ::= ID ":" Type [ "=" ConstDefault ]

StructLiteral    ::= StructName "{" [ FieldInit { "," FieldInit } [ "," ] ] "}"
                 |   StructName "{" NEWLINE INDENT FieldInit { NEWLINE FieldInit } NEWLINE? DEDENT "}"
StructName       ::= ID { "::" ID }
FieldInit        ::= ID ":" Expression
```

`Name {` and `a::b::Name {` always parse as a struct literal: `{` after a name has no other meaning, so the parser doesn't need to know which structs exist. The analyzer resolves the name; a name that isn't a struct is a check error (`unknown struct type 'Pnt'`).

## 4. Where Structs Live and How They Are Named

The naming rules are those of `modules_v0_2_spec.md` §3.7:

- A struct can be defined at the top level of a file or inside a `module` block.
- Inside its file it is written bare (`Point`), or `module::Point` from outside its module.
- From an importing file it is `file::Point` or `file::module::Point`, in literals, annotations and return types alike:

  ```quark
  incl './lib/geo'
  p = geo::Point { x: 1.0, y: 2.0 }
  fn shift(p: geo::Point) geo::Point -> geo::Point { x: p.x + 1, y: p.y }
  s = geo::shapes::Sq { side: 2 }
  ```

- `geo.Point { ... }` is a parse error that points to `::`: `a struct from a module or imported file is written geo::Point { ... }, with '::'`.
- `print`, `type()` and diagnostics show an imported struct qualified by its file's stem (`geo::Point`, also when the file was imported `as g`), a struct of a main-script module as `m::S`, and a main-script top-level struct bare (`Point`).

## 5. Static Semantics

### 5.1 Type identity

- Structs are nominal: two structs with identical fields are different types.
- Identity is the definition, not the name. `a::Point` and `b::Point` are different types at compile time and at runtime (§6): `argument 1 of 'f' expects g1::Point, got g2::Point`. A file imported along several paths is loaded once, so its structs are one type.

### 5.2 Field types

- Any annotation type: `int`, `float`, `str`, `bool`, `any`, `list`, `dict`, `vec`, `result`, `resource`, `fn`, or a struct name (bare or qualified).
- A field type can name a struct declared later, and structs can refer to each other and to themselves (`struct Node:` / `next: Node`), since a struct field accepts `null`.
- `fn` fields hold any function value. The value is read with `.` and then called: `f = h.run`, `f(2)`. Calling it directly (`h.run(2)`) is a compile error, `dot-call syntax is not supported on values of type 'H'`.

### 5.3 Default values

A default is a constant expression, checked by the parser (`struct field default values must be constant expressions`):

- a literal: `int`, `float`, `str`, `bool` or `null`;
- unary minus on a constant (`-1`);
- `+ - * / %` over constants (`60 * 60`, `'a' + 'b'`);
- an empty `list []` or `dict {}`. Each struct value gets a fresh one, so two values never share a default container.

Names, calls, `**` and `!` are not allowed. The default must match the field type (`default value type 'float' is not compatible with field type 'int'`); an int default for a `float` field is widened (`weight: float = 2` stores `2.0`). A `null` default is accepted only where the field type accepts `null` (§8.1).

### 5.4 Construction checks

At each literal the analyzer checks that:

1. the name resolves to a struct;
2. every given field exists and appears once;
3. every field without a default is given;
4. each value's type fits the field's type. A known wrong type is a compile error (`cannot assign value of type 'str' to field 'x' of type 'int'`). An int for a `float` field is widened.

A value whose type the compiler doesn't know (from `any`, an unannotated parameter, `unwrap`, native code) is checked when the literal is built: `fn mk(a) -> P { x: a }`, then `mk('hello')` stops with `field 'x' of P expects int, got str`. A field therefore always holds a value of its declared type, and field reads need no further check.

### 5.5 Field access

- On a value whose struct type is known, `x.name` must be a declared field (`struct P has no field 'y'`) and has the field's declared type. The read is by position, resolved at compile time.
- Reading a field of a `null` struct value is a runtime error: `cannot read a field of null (expected a struct value)`.
- Field access needs the struct type to be known at compile time (an annotation, a literal, a typed list element, a function's return type). On a value of unknown type (`fn get(o) -> o.x`), `.` only reads dict keys, so a struct is a runtime error (`dot access is only supported on dict; got type 'struct'`).

### 5.6 Immutability

`x.field = expr` on a struct-typed value is a compile error (`cannot assign to field 'x' of immutable struct P`). On a value of unknown type it is a runtime error (`cannot set member 'x' on non-dict type`). Since no code can change a field, values can share storage freely.

A container in a field is still mutable through its own methods (`p.xs.push(1)` on a `list` field), following the container's element-type rules.

### 5.7 Equality

`==` and `!=` on struct values are compile errors: `operator '==' is not defined for struct values; compare fields explicitly`. Compare fields (`a.id == b.id`). See §9 for comparisons the compiler can't see.

## 6. Runtime Semantics

- A struct value is a `QStruct` (`runtime/include/quark/types/struct.hpp`): a pointer to its `QStructDef` plus one `QValue` slot per field, in declaration order. Values are GC-allocated and boxed with the `VAL_STRUCT` tag.
- Codegen emits one `static const QStructDef _sdef_<name>` per definition (`{display name, field names, field count}`). Pointer identity of the def is type identity: the runtime checks `q_param_struct` (parameters, `builtins/params.hpp`) and `q_field_struct` (struct-typed fields) compare `QStructDef*`, never names. `QStructDef.name` holds the display name (`geo::Point`) used by `print`, `type()` and messages.
- Field values of unproven type are wrapped in `q_field_int`/`_float`/`_bool`/`_check`/`_struct` when the literal is built; proven values are stored unchanged.
- Printing uses declaration order: `Point {x: 1.0, y: 2.0}`. A struct is always truthy.

## 7. Diagnostics

Struct errors use `QK-PARSE-001` (shape, including a non-constant default), `QK-CHECK-001` (types and fields) and `QK-RUNTIME-001` (checks on values of unknown type).

| Error | Message |
|---|---|
| Unknown struct | `unknown struct type 'Pnt'` (in an annotation: `unknown type 'Pnt'`) |
| Duplicate field in a declaration | `duplicate field 'x' in struct 'P'` |
| Non-constant default | `struct field default values must be constant expressions` |
| Wrong default type | `default value type 'float' is not compatible with field type 'int'` |
| Unknown field in a literal | `unknown field 'zipcode' in Customer literal` |
| Missing field | `missing required field 'city' in Customer literal` |
| Duplicate field in a literal | `duplicate field 'id' in Customer literal` |
| Wrong field value type | `cannot assign value of type 'str' to field 'x' of type 'int'` |
| Field value of unknown type (runtime) | `field 'x' of P expects int, got str` |
| Unknown field read | `struct Customer has no field 'foo'` |
| Field assignment | `cannot assign to field 'age' of immutable struct Customer` |
| Struct equality | `operator '==' is not defined for struct values; compare fields explicitly` |
| Field of `null` (runtime) | `cannot read a field of null (expected a struct value)` |
| Wrong struct (compile / runtime) | `argument 1 of 'f' expects g1::Point, got g2::Point` / `argument 'p' of 'f' expects g1::Point, got g2::Point` |
| `.` in a qualified literal | `a struct from a module or imported file is written geo::Point { ... }, with '::'` |

## 8. Interaction with Other Features

### 8.1 Annotations, parameters and returns

```quark
cust: Customer = Customer { id: 1, name: 'A', city: 'X' }
fn is_adult(c: Customer) bool -> c.age >= 18
```

- A struct annotation (parameter, typed declaration, field, return) is checked like any annotation: at compile time when the value's type is known, otherwise on entry (`q_param_struct`, `q_var_struct`, `q_ret_struct`).
- Struct and `resource` annotations accept `null`; `list`, `dict`, `vec`, `fn` and `result` annotations don't, in fields as everywhere else (`cannot assign value of type 'null' to field 'v' of type 'vec[any]'`).

### 8.2 Containers

- A `list` can hold struct values, and a `dict` can map keys to them; the element type includes the struct type, so `for c in customers: c.name` is checked.
- A `vec` can't hold structs (`vec elements must be homogeneous int, float, bool, or str`).

### 8.3 Tables

A table's row type is a struct (`tables_v0_1_spec.md` D2 and D10; planned):

```quark
struct Employee:
    id: int
    name: str
    salary: float

staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))
```

- The struct gives the column names and types, and the compiler checks every column reference in a query against it. A bare struct annotation (`t: Employee`) always means one struct value, never a table.
- CSV columns are matched to fields by name, with an optional literal `dict` mapping fields to header text (D17).
- There is no `schema` declaration: the struct is the schema.

## 9. Limitations

- A `struct` inside a function body is not supported and fails with `internal: struct 'S' not pre-declared` (changelog, Known issues). Define it at the top level or in a module.
- A field named with `__` (`a__b: int`) is accepted in the declaration and rejected only where a literal uses it (changelog, Known issues).
- A parse error in a field default is reported one line below the field.
- **Known bug (found 2026-11-13):** struct equality is rejected only where the compiler sees that both operands are structs. Through a value of unknown type, or inside lists, `==` returns `false` even for the same value: `fn id(v) -> v`, `a = P { x: 1 }`, then `id(a) == id(a)` and `list [a] == list [a]` print `false`. It should be a runtime error.
- Not planned for v0.1: methods, recursive defaults, a struct update helper, `vec` of structs.

## 10. Tests

- Smoke programs: `smoke_structs.qrk` (declaration, inline and multiline literals, defaults, reconstruction), `smoke_struct_fields.qrk` (widening and runtime field checks), and the rejected programs `smoke_structs_err_{assign,equality,missing_field,type_mismatch,unknown_field}.qrk`.
- `TestStructsAcrossFiles` (imported structs, identity across files, qualified literals) and the struct cases of `TestSemanticsSafety_Rejected` (field values from `any`, `null` per field type).
