package varnamelen

import "go/types"

// Shared pointers keep distance-map keys stable when the cached name is resolved.
type shortType struct {
	typ      types.Type
	imports  []importDeclaration
	name     string
	resolved bool
}

func (typ *shortType) String() string {
	if !typ.resolved {
		typ.name = shortTypeName(typ.typ, typ.imports)
		typ.resolved = true
	}
	return typ.name
}
