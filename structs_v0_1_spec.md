# Quark Struct-Lite v0.1 Spec (Draft)

Status: Draft
Last Updated: 2026-04-02
Scope: Struct-lite only (data modeling). Schema/table integration is referenced but not specified here.

## 1. Purpose

Define a minimal struct system for Quark that fills the fixed-shape domain-model gap between dynamic `dict` values and columnar `table` workflows.

This spec prioritizes:

1. Small language surface area.
2. Strong compile-time guarantees for field shape and types.
3. DSL alignment with pipeline/data-contract goals.
4. Low implementation complexity for v0.1.

## 2. Design Constraints

### 2.1 Product constraints

Structs must:

1. Be useful for real pipeline-domain modeling (customer, order, job, config records).
2. Avoid turning Quark into class-oriented OOP.
3. Integrate cleanly with upcoming schemas.

### 2.2 Engineering constraints

1. Avoid parser-heavy features (no impl blocks, no inheritance, no generics in v0.1).
2. Reuse existing typed declaration and field-access model where possible.
3. Keep analyzer and runtime changes incremental and testable.

## 3. Goals and Non-Goals

### 3.1 Goals

1. Introduce a `struct` declaration form with typed fields.
2. Support optional default values for fields.
3. Support named field construction using `TypeName { ... }` literals.
4. Support deterministic update through reconstruction using the same literal form.
5. Enforce field shape and type checks at compile time whenever possible.

### 3.2 Non-goals (v0.1)

1. No methods on structs.
2. No impl blocks.
3. No inheritance/embedding/traits.
4. No generic structs.
5. No pattern destructuring syntax.
6. No mutable field-assignment syntax like `obj.field = ...` for structs.

## 4. Syntax

### 4.1 Declaration

```quark
struct Customer:
    id: int
    name: str
    age: int = 0
    city: str
```

Rules:

1. `struct` introduces a named nominal type.
2. Body contains only field declarations.
3. A field declaration is `name: Type` with optional `= default_expr`.
4. Field names must be unique within the struct.

### 4.2 Construction

```quark
c1 = Customer {
    id: 1
    name: 'Alice'
    city: 'NYC'
}
```

Rules:

1. Constructor literals are named-field only.
2. Missing required fields are compile-time errors.
3. Missing optional fields use defaults.
4. Unknown fields are compile-time errors.
5. Duplicate fields in one literal are compile-time errors.

### 4.3 Update by reconstruction (same syntax as construction)

```quark
c2 = Customer {
    id: c1.id
    name: c1.name
    age: c1.age + 1
    city: 'SF'
}
```

Notes:

1. v0.1 uses reconstruction as the canonical update pattern.
2. No new update keyword is introduced.
3. This keeps syntax minimal and parser changes small.

## 5. Grammar Additions (EBNF)

```ebnf
Statement        ::= ...
                 |   StructDef

StructDef        ::= "struct" ID ":" NEWLINE INDENT
                     FieldDecl { NEWLINE FieldDecl } NEWLINE?
                     DEDENT

FieldDecl        ::= ID ":" Type [ "=" DefaultExpr ]

DefaultExpr      ::= Expression

Expression       ::= ...
                 |   StructLiteral

StructLiteral    ::= ID "{" [ StructFieldInit { "," StructFieldInit } [ "," ] ] "}"
                 |   ID "{" NEWLINE INDENT StructFieldInitLine { NEWLINE StructFieldInitLine } NEWLINE? DEDENT "}"

StructFieldInit  ::= ID ":" Expression
StructFieldInitLine ::= ID ":" Expression
```

Implementation note:

1. `ID { ... }` is parsed as a struct literal only when `ID` resolves to a struct type symbol.
2. Otherwise existing expression rules apply and analyzer should emit a clear error.

## 6. Static Semantics

### 6.1 Type identity

1. Structs are nominal types.
2. Two structs with identical fields are still different types.

### 6.2 Field types

1. Field types may be builtins (`int`, `float`, `str`, `bool`, `list`, `dict`, `vector`, `result`, `any`) or user types.
2. Self-referential and mutually recursive structs are deferred from v0.1.

### 6.3 Default values

v0.1 default values allow constant expressions:

1. Allowed defaults include literals and constant expressions, for example `60 * 60`, `1 + 2`, `'a'.concat('b')` is not allowed in v0.1.
2. A constant expression default must be side-effect free and may not reference runtime symbols.
3. Function calls are not allowed in default expressions in v0.1.
4. Default must be type-compatible with field annotation.

### 6.4 Construction checks

At each struct literal site, analyzer must validate:

1. Target type exists and is a struct.
2. Every provided field exists in the struct.
3. No field appears more than once.
4. All required fields are provided or have defaults.
5. Each field expression type is compatible with annotated field type.

### 6.5 Field access checks

Given `x: Customer`, `x.name` is valid only if `name` is a declared field.

Rules:

1. Unknown field access on known struct type is compile-time error.
2. Known field access returns the declared field type.
3. Dot access resolution priority should remain deterministic with existing dict/module/method rules.

### 6.6 Assignment and mutability

v0.1 struct values are immutable from user syntax.

1. `x.field = expr` where `x` is struct-typed is a compile-time error.
2. Update is expressed by constructing a new struct value.

## 7. Runtime Semantics

### 7.1 Representation

Proposed runtime representation:

1. Add a new runtime type tag for struct values.
2. A struct value stores:
   - type-id (or symbol id)
   - fixed field layout metadata reference
   - contiguous storage for field `QValue` slots

### 7.2 Copy and alias behavior

1. Struct values are treated as immutable from Quark code.
2. Assignment/call passing may share underlying storage internally as an optimization.
3. Because user-level mutation is disallowed, sharing cannot create visible aliasing bugs.

### 7.3 Equality

v0.1 behavior:

1. Whole-struct `==` and `!=` are not supported.
2. Comparing struct values with `==` or `!=` is a compile-time error.
3. Users must compare explicit fields (for example `a.id == b.id`).

### 7.4 Field order

1. Struct declaration field order is preserved in compiler metadata.
2. Runtime field lookup semantics are by field name, not by ordinal position.
3. Any tooling or serialization that renders struct fields should use declaration order for deterministic output.

## 8. Diagnostics

Struct-related failures should reuse stable frontend/runtime code families:

1. Parse shape errors: `QK-PARSE-001`.
2. Type/semantic errors: `QK-CHECK-001`.
3. Runtime struct misuse (if any dynamic-path checks remain): `QK-RUNTIME-001`.

Recommended error messages:

1. Unknown field: `struct Customer has no field 'foo'`.
2. Missing required field: `missing required field 'city' in Customer literal`.
3. Extra field: `unknown field 'zipcode' in Customer literal`.
4. Duplicate init: `duplicate field 'id' in Customer literal`.
5. Illegal assignment: `cannot assign to field 'age' of immutable struct Customer`.
6. Illegal struct equality: `operator '==' is not defined for struct values; compare fields explicitly`.

## 9. Interaction with Existing Language Features

### 9.1 Typed declarations

```quark
cust: Customer = Customer { id: 1, name: 'A', city: 'X' }
```

### 9.2 Function parameters and returns

```quark
fn is_adult(c: Customer) bool -> c.age >= 18
```

### 9.3 Lists/dicts/vectors containing structs

1. `list` can hold struct values (boxed as `QValue`).
2. `dict` values can be structs.
3. `vector` does not accept struct dtype in v0.1.

### 9.4 Schema integration (planned, not normative here)

1. `schema X for Customer` references this struct type as contract base.
2. Schema checks can target either `Customer` rows or `table Customer` values.

## 10. Compiler Work Plan (Implementation-Oriented)

1. Lexer: add `struct` keyword token.
2. AST: add nodes for struct definition and struct literal.
3. Parser: parse `struct` blocks and typed field initializers.
4. Analyzer:
   - register struct symbols
   - type-check field declarations/defaults
   - type-check struct literals and field access
   - reject struct field assignment targets
5. Codegen/runtime:
   - introduce struct runtime tag and storage layout
   - emit construction and field-read code paths
6. Tests:
   - parser tests
   - analyzer negative/positive tests
   - smoke tests covering defaults and reconstruction updates

## 11. Acceptance Criteria for v0.1

1. Struct declaration parses and type-checks.
2. Struct literal construction enforces field completeness/type correctness.
3. Defaults are applied deterministically.
4. Field access is statically checked when type info is available.
5. Struct field assignment is rejected.
6. Reconstruction update pattern works in smoke tests.
7. No regressions in existing dict dot access/module-qualified call behavior.
8. Whole-struct `==` / `!=` are rejected with a clear compile-time diagnostic.
9. Both inline and multiline struct literal forms parse and type-check.

## 12. Open Questions (Need Decision)

None for the v0.1 struct-lite baseline in this spec.

## 12.1 Locked Decisions

1. Defaults allow constant expressions in v0.1.
2. Recursive structs are deferred from v0.1.
3. No stdlib struct update helper is included in v0.1.
4. `any`-typed fields are allowed.
5. Field order is preserved for tooling/serialization determinism.
6. Whole-struct equality (`==`, `!=`) is rejected in v0.1.
7. Both inline and multiline struct literal forms are supported in v0.1.

## 13. Suggested Initial Smoke Cases

1. Basic declaration + construction + field access.
2. Missing required field fails compile.
3. Unknown field fails compile.
4. Default field populated when omitted.
5. Type mismatch in field init fails compile.
6. Reconstruction update pattern compiles and preserves original value.
7. Field assignment target fails compile.
8. Nested struct field access (if nested structs are enabled).
