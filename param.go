package varnamelen

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// parameter represents a declared function or method parameter.
type parameter struct {
	// name is the name of the parameter.
	name string

	// typ is the type of the parameter.
	typ *shortType

	// field is the declaration of the parameter.
	field *ast.Field
}

// checkParams applies the analysis to parameters in paramToDist, according to cfg.
func checkParams(pass *analysis.Pass, paramToDist map[parameter]int, cfg configuration) {
	for param, dist := range paramToDist {
		if cfg.ignoreNames.contains(param.name) {
			continue
		}

		if checkNameAndDistance(param.name, dist, cfg) {
			continue
		}

		if param.isConventional() {
			continue
		}

		if cfg.ignoreDecls.matchParameter(param) {
			continue
		}

		pass.Reportf(param.field.Pos(), "parameter name '%s' is too short for the scope of its usage", param.name)
	}
}

// match returns whether p matches decl.
func (p parameter) match(decl identDeclaration) bool {
	if p.name != decl.name {
		return false
	}

	return decl.matchType(p.typ.String())
}

// isConventional returns true if p matches a conventional Go parameter name and type,
// such as "ctx context.Context" or "t *testing.T".
func (p parameter) isConventional() bool {
	for _, decl := range conventionalDecls {
		if p.match(decl) {
			return true
		}
	}

	return false
}
