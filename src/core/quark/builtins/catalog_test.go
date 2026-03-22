package builtins

import (
	"testing"
)

// --- Structural invariants ---

func TestCatalog_AllSpecsHaveNonEmptyName(t *testing.T) {
	for i, s := range catalog {
		if s.Name == "" {
			t.Errorf("catalog[%d]: Name is empty", i)
		}
	}
}

func TestCatalog_AllSpecsHaveNonEmptyRuntime(t *testing.T) {
	for i, s := range catalog {
		if s.Runtime == "" {
			t.Errorf("catalog[%d] (%s): Runtime symbol is empty", i, s.Name)
		}
	}
}

func TestCatalog_AllSpecsHaveRuntimePrefixQ(t *testing.T) {
	for i, s := range catalog {
		if len(s.Runtime) < 2 || s.Runtime[:2] != "q_" {
			t.Errorf("catalog[%d] (%s): Runtime symbol %q doesn't start with 'q_'", i, s.Name, s.Runtime)
		}
	}
}

func TestCatalog_MinArgsNotGreaterThanMaxArgs(t *testing.T) {
	for i, s := range catalog {
		if s.MinArgs > s.MaxArgs {
			t.Errorf("catalog[%d] (%s): MinArgs (%d) > MaxArgs (%d)", i, s.Name, s.MinArgs, s.MaxArgs)
		}
	}
}

func TestCatalog_AllSpecsHaveReturnType(t *testing.T) {
	for i, s := range catalog {
		if s.ReturnType == "" {
			t.Errorf("catalog[%d] (%s): ReturnType is empty", i, s.Name)
		}
	}
}

func TestCatalog_ParamTypesLengthMatchesMaxArgs(t *testing.T) {
	for i, s := range catalog {
		if len(s.ParamTypes) != s.MaxArgs {
			t.Errorf("catalog[%d] (%s): len(ParamTypes) = %d but MaxArgs = %d",
				i, s.Name, len(s.ParamTypes), s.MaxArgs)
		}
	}
}

func TestCatalog_MethodsHaveReceiverType(t *testing.T) {
	// Verify methods are correctly indexed by checking byMethod map
	for recvType, methods := range byMethod {
		if recvType == "" {
			t.Fatal("byMethod has entry with empty receiver type key")
		}
		for name, spec := range methods {
			if spec.ReceiverType == "" {
				t.Errorf("byMethod[%s][%s]: spec has empty ReceiverType", recvType, name)
			}
			if spec.ReceiverType != recvType {
				t.Errorf("byMethod[%s][%s]: spec.ReceiverType = %s, expected %s",
					recvType, name, spec.ReceiverType, recvType)
			}
		}
	}
}

func TestCatalog_FreeFunctionsHaveNoReceiverType(t *testing.T) {
	for name, spec := range byName {
		if spec.ReceiverType != "" {
			t.Errorf("byName[%s]: free function has ReceiverType = %s", name, spec.ReceiverType)
		}
	}
}

func TestCatalog_NoDuplicateFreeFunctions(t *testing.T) {
	seen := make(map[string]int)
	for i, s := range catalog {
		if s.ReceiverType != "" {
			continue // methods can share names across receiver types
		}
		if prev, ok := seen[s.Name]; ok {
			t.Errorf("catalog[%d] (%s): duplicate free function (first at index %d)", i, s.Name, prev)
		}
		seen[s.Name] = i
	}
}

func TestCatalog_NoDuplicateMethodsPerReceiver(t *testing.T) {
	type key struct {
		recv TypeKey
		name string
	}
	seen := make(map[key]int)
	for i, s := range catalog {
		if s.ReceiverType == "" {
			continue
		}
		k := key{s.ReceiverType, s.Name}
		if prev, ok := seen[k]; ok {
			t.Errorf("catalog[%d] (%s on %s): duplicate method (first at index %d)",
				i, s.Name, s.ReceiverType, prev)
		}
		seen[k] = i
	}
}

// --- Lookup functions ---

func TestLookup_FindsFreeFunctions(t *testing.T) {
	freeFuncs := []string{"print", "println", "input", "len", "to_str", "to_int",
		"to_float", "to_bool", "type", "range", "abs", "sqrt", "unwrap"}
	for _, name := range freeFuncs {
		spec, ok := Lookup(name)
		if !ok {
			t.Errorf("Lookup(%q): not found", name)
			continue
		}
		if spec.Name != name {
			t.Errorf("Lookup(%q): spec.Name = %q", name, spec.Name)
		}
	}
}

func TestLookup_DoesNotFindMethods(t *testing.T) {
	// "upper" is a method on str, should not be found as free function
	_, ok := Lookup("upper")
	if ok {
		t.Error("Lookup('upper') found a method as free function")
	}
}

func TestLookup_UnknownReturnsNotFound(t *testing.T) {
	_, ok := Lookup("nonexistent_function")
	if ok {
		t.Error("Lookup('nonexistent_function') should return false")
	}
}

func TestLookupMethod_FindsStringMethods(t *testing.T) {
	methods := []string{"upper", "lower", "trim", "contains", "startswith",
		"endswith", "replace", "concat", "split", "slice"}
	for _, name := range methods {
		spec, ok := LookupMethod(TypeString, name)
		if !ok {
			t.Errorf("LookupMethod(TypeString, %q): not found", name)
			continue
		}
		if spec.ReceiverType != TypeString {
			t.Errorf("LookupMethod(TypeString, %q): ReceiverType = %s", name, spec.ReceiverType)
		}
	}
}

func TestLookupMethod_FindsListMethods(t *testing.T) {
	methods := []string{"push", "pop", "get", "set", "insert", "remove",
		"slice", "reverse", "enumerate", "join", "to_vector", "concat"}
	for _, name := range methods {
		spec, ok := LookupMethod(TypeListAny, name)
		if !ok {
			t.Errorf("LookupMethod(TypeListAny, %q): not found", name)
			continue
		}
		if spec.ReceiverType != TypeListAny {
			t.Errorf("LookupMethod(TypeListAny, %q): ReceiverType = %s", name, spec.ReceiverType)
		}
	}
}

func TestLookupMethod_FindsDictMethods(t *testing.T) {
	methods := []string{"get", "set", "keys", "values", "items"}
	for _, name := range methods {
		spec, ok := LookupMethod(TypeDictAny, name)
		if !ok {
			t.Errorf("LookupMethod(TypeDictAny, %q): not found", name)
			continue
		}
		if spec.ReceiverType != TypeDictAny {
			t.Errorf("LookupMethod(TypeDictAny, %q): ReceiverType = %s", name, spec.ReceiverType)
		}
	}
}

func TestLookupMethod_FindsVectorMethods(t *testing.T) {
	methods := []string{"get", "fillna", "astype", "to_list"}
	for _, name := range methods {
		spec, ok := LookupMethod(TypeVectorAny, name)
		if !ok {
			t.Errorf("LookupMethod(TypeVectorAny, %q): not found", name)
			continue
		}
		if spec.ReceiverType != TypeVectorAny {
			t.Errorf("LookupMethod(TypeVectorAny, %q): ReceiverType = %s", name, spec.ReceiverType)
		}
	}
}

func TestLookupMethod_SameNameDifferentReceiver(t *testing.T) {
	// "get" exists on list, dict, and vector with different runtime symbols
	listGet, ok1 := LookupMethod(TypeListAny, "get")
	dictGet, ok2 := LookupMethod(TypeDictAny, "get")
	vecGet, ok3 := LookupMethod(TypeVectorAny, "get")
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("'get' should exist on list, dict, and vector")
	}
	if listGet.Runtime == dictGet.Runtime {
		t.Errorf("list.get and dict.get should have different runtime symbols: both = %s", listGet.Runtime)
	}
	if listGet.ReceiverType == dictGet.ReceiverType {
		t.Error("list.get and dict.get should have different receiver types")
	}
	// list.get and vector.get happen to share q_get — that's fine, just verify they're both found
	_ = vecGet
}

func TestLookupMethod_UnknownMethodOnKnownType(t *testing.T) {
	_, ok := LookupMethod(TypeString, "nonexistent")
	if ok {
		t.Error("LookupMethod(TypeString, 'nonexistent') should return false")
	}
}

func TestLookupMethod_UnknownReceiverType(t *testing.T) {
	_, ok := LookupMethod(TypeKey("unknown_type"), "get")
	if ok {
		t.Error("LookupMethod with unknown type should return false")
	}
}

// --- Specific arity checks ---

func TestCatalog_PrintArity(t *testing.T) {
	s, ok := Lookup("print")
	if !ok {
		t.Fatal("print not found")
	}
	if s.MinArgs != 1 || s.MaxArgs != 5 {
		t.Errorf("print: expected arity 1..5, got %d..%d", s.MinArgs, s.MaxArgs)
	}
}

func TestCatalog_RangeArity(t *testing.T) {
	s, ok := Lookup("range")
	if !ok {
		t.Fatal("range not found")
	}
	if s.MinArgs != 1 || s.MaxArgs != 3 {
		t.Errorf("range: expected arity 1..3, got %d..%d", s.MinArgs, s.MaxArgs)
	}
}

func TestCatalog_ZeroArgMethods(t *testing.T) {
	zeroArgMethods := []struct {
		recv TypeKey
		name string
	}{
		{TypeString, "upper"},
		{TypeString, "lower"},
		{TypeString, "trim"},
		{TypeListAny, "pop"},
		{TypeListAny, "reverse"},
		{TypeListAny, "enumerate"},
		{TypeDictAny, "keys"},
		{TypeDictAny, "values"},
		{TypeDictAny, "items"},
		{TypeVectorAny, "to_list"},
	}
	for _, tt := range zeroArgMethods {
		spec, ok := LookupMethod(tt.recv, tt.name)
		if !ok {
			t.Errorf("LookupMethod(%s, %s): not found", tt.recv, tt.name)
			continue
		}
		if spec.MinArgs != 0 || spec.MaxArgs != 0 {
			t.Errorf("%s.%s: expected 0 args, got %d..%d", tt.recv, tt.name, spec.MinArgs, spec.MaxArgs)
		}
	}
}

// --- Catalog() returns a copy ---

func TestCatalog_ReturnsCopy(t *testing.T) {
	c1 := Catalog()
	c2 := Catalog()
	if len(c1) != len(c2) {
		t.Fatalf("Catalog() returned different lengths: %d vs %d", len(c1), len(c2))
	}
	// Modifying the copy shouldn't affect the original
	c1[0].Name = "MODIFIED"
	c3 := Catalog()
	if c3[0].Name == "MODIFIED" {
		t.Error("Catalog() returned a reference to internal data instead of a copy")
	}
}

// --- Cross-check: concat is both a str method and a list method ---

func TestCatalog_ConcatExistsOnBothStrAndList(t *testing.T) {
	strConcat, ok1 := LookupMethod(TypeString, "concat")
	listConcat, ok2 := LookupMethod(TypeListAny, "concat")
	if !ok1 {
		t.Error("str.concat not found")
	}
	if !ok2 {
		t.Error("list.concat not found")
	}
	if ok1 && ok2 && strConcat.Runtime == listConcat.Runtime {
		t.Error("str.concat and list.concat should have different runtime symbols")
	}
}

// --- Backward compatibility aliases ---

func TestCatalog_BackwardCompatAliasesExist(t *testing.T) {
	aliases := []string{"enumerate", "vfrom_list"}
	for _, name := range aliases {
		_, ok := Lookup(name)
		if !ok {
			t.Errorf("backward-compat alias %q not found as free function", name)
		}
	}
}
