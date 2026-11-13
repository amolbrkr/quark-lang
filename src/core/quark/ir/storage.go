package ir

import "fmt"

// Storage is how a variable (or parameter, loop variable, binding) is held in
// generated C++. The analyzer decides it once per symbol (types.Symbol); lowering
// only reads it.
type Storage int

const (
	StorageValue Storage = iota // QValue (tagged union)
	StorageInt                  // long long: the value is always an int
	StorageFloat                // double: always a float (an int is widened)
	StorageBool                 // bool
	StorageCell                 // QCell*: captured by a closure, so shared by reference
)

// CType is the C++ type of a variable with this storage.
func (s Storage) CType() string {
	switch s {
	case StorageInt:
		return "long long"
	case StorageFloat:
		return "double"
	case StorageBool:
		return "bool"
	case StorageCell:
		return "QCell*"
	}
	return "QValue"
}

// IsScalar reports whether the storage is a raw C++ scalar.
func (s Storage) IsScalar() bool {
	return s == StorageInt || s == StorageFloat || s == StorageBool
}

// Generated C++ names. A user name is emitted as quark_<name> (with the loader's
// internal name). User names never contain "__" and no internal name starts
// with "__", so every name below, which starts with quark___, can't be written
// by a user or produced by the loader (rule 6 of the robustness bar).

// VarCName names the C++ variable of symbol id when the plain quark_<name> is
// taken by another symbol of the same name.
func VarCName(id int, name string) string { return fmt.Sprintf("quark___v%d_%s", id, name) }

// LambdaFnName is the generated function name (without the quark_ prefix) of
// the n-th anonymous lambda.
func LambdaFnName(n int) string { return fmt.Sprintf("__lambda%d", n) }

// NestedFnName is the generated function name (without the quark_ prefix) of
// a `fn` declaration that isn't a plain top-level function.
func NestedFnName(n int, name string) string { return fmt.Sprintf("__fn%d_%s", n, name) }
