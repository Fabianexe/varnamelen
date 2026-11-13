package varnamelen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

type sharedTypeImporter struct{ pkg *types.Package }

func (i sharedTypeImporter) Import(string) (*types.Package, error) { return i.pkg, nil }

func typeNamePass(t *testing.T, source string, importer types.Importer) *analysis.Pass {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Instances: map[*ast.Ident]types.Instance{}}
	config := &types.Config{Importer: importer}
	pkg, err := config.Check("fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	return &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, ResultOf: map[*analysis.Analyzer]interface{}{inspect.Analyzer: inspector.New([]*ast.File{file})}, Report: func(analysis.Diagnostic) {}}
}

const aggregationSource = `package fixture
type Alias = int
type Holder int
type Box[T any] struct { Item T }
type IntBox = Box[int]
func (h Holder) method(a,b Alias) (r,s int) {
 x:=a
 x,y:=b,a
 var z,w Alias
 const c,d=1,2
 { x:=y;z=x }
 switch v:=any(x).(type) { case int:w=v;case string:w=len(v) }
 closure:=func(v int)(q int) { q=v;return }
 r=closure(x)+y+z+w+c+d+int(h)
 s=r
 return
}
func generic[T any](a T)(r T) {
 b:=a
 _=b
 _=b
 r=b
 return
}
func lines(a int) int {
//line fixture.go:100
 x:=a
//line fixture.go:500
 _=x
//line fixture.go:2
 _=x
 return x
}
func aliasInstance(value IntBox) int { return value.Item }
`

func allTypeRecords(result distancesResult) []*shortType {
	var records []*shortType
	for ident := range result.variableToDist {
		records = append(records, ident.typ)
	}
	for ident := range result.paramToDist {
		records = append(records, ident.typ)
	}
	for ident := range result.returnToDist {
		records = append(records, ident.typ)
	}
	for ident := range result.receiverToDist {
		records = append(records, ident.typ)
	}
	for ident := range result.typeParamToDist {
		records = append(records, ident.typ)
	}
	return records
}

func TestLazyTypeNamesPreserveDeclarationAggregation(t *testing.T) {
	t.Parallel()
	pass := typeNamePass(t, `package fixture
type Alias = int
func f(a,b Alias)(r,s Alias) {
 x:=a
 x,y:=b,a
 _=x
 _=y
 { x:=y;_=x }
 r=x
 s=r
 return
}`, nil)
	result := distances(pass, configuration{checkReturn: true})
	if len(result.variableToDist) != 3 || len(result.paramToDist) != 2 || len(result.returnToDist) != 2 {
		t.Fatal("declaration keys merged or split")
	}
	seen := map[string][]int{}
	for variable, distance := range result.variableToDist {
		seen[variable.name] = append(seen[variable.name], distance)
	}
	if len(seen["x"]) != 2 || len(seen["y"]) != 1 || seen["y"][0] != 3 {
		t.Fatalf("unexpected variable distances: %v", seen)
	}
	if seen["x"][0]+seen["x"][1] != 5 || seen["x"][0]*seen["x"][1] != 0 {
		t.Fatalf("unexpected shadowed x distances: %v", seen["x"])
	}
	for _, distance := range result.paramToDist {
		if distance != 2 {
			t.Fatalf("parameter distance=%d", distance)
		}
	}
	for _, distance := range result.returnToDist {
		if distance != 7 {
			t.Fatalf("return distance=%d", distance)
		}
	}
}

func TestLazyTypeNamesPreserveLineDirectives(t *testing.T) {
	t.Parallel()
	pass := typeNamePass(t, `package fixture
func f(a int) int {
//line fixture.go:100
 x:=a
//line fixture.go:500
 _=x
//line fixture.go:2
 return x
}`, nil)
	result := distances(pass, configuration{})
	if len(result.variableToDist) != 1 {
		t.Fatal("variable uses did not aggregate")
	}
	for _, distance := range result.variableToDist {
		if distance != -98 {
			t.Fatalf("line-directive distance=%d,want -98", distance)
		}
	}
}

func TestLazyTypeNamesPreserveTypeSwitchKey(t *testing.T) {
	t.Parallel()
	pass := typeNamePass(t, `package fixture
func f(value any) int {
 switch v:=value.(type) {
 case int: return v
 case string: return len(v)
 }
 return 0
}`, nil)
	result := distances(pass, configuration{})
	if len(result.variableToDist) != 1 {
		t.Fatal("type-switch case types split declaration key")
	}
	for variable, distance := range result.variableToDist {
		if variable.name != "v" || variable.typ.String() != "<type-switched>" || distance != 2 {
			t.Fatalf("unexpected type-switch key or distance: %v %d", variable, distance)
		}
	}
}

func TestAcceptedNamesAvoidTypeFormatting(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{"length", "distance", "ignored"} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			pass := typeNamePass(t, aggregationSource, nil)
			cfg := configuration{checkReceiver: true, checkReturn: true, checkTypeParam: true, minNameLength: 100, maxDistance: -10000}
			result := distances(pass, cfg)
			switch reason {
			case "length":
				cfg.minNameLength = 0
			case "distance":
				cfg.maxDistance = 10000
			case "ignored":
				for ident := range result.variableToDist {
					cfg.ignoreNames.Values = append(cfg.ignoreNames.Values, ident.name)
				}
				for ident := range result.constantToDist {
					cfg.ignoreNames.Values = append(cfg.ignoreNames.Values, ident.name)
				}
				for ident := range result.paramToDist {
					cfg.ignoreNames.Values = append(cfg.ignoreNames.Values, ident.name)
				}
				for ident := range result.returnToDist {
					cfg.ignoreNames.Values = append(cfg.ignoreNames.Values, ident.name)
				}
				for ident := range result.receiverToDist {
					cfg.ignoreNames.Values = append(cfg.ignoreNames.Values, ident.name)
				}
				for ident := range result.typeParamToDist {
					cfg.ignoreNames.Values = append(cfg.ignoreNames.Values, ident.name)
				}
			}
			cfg.ignoreDecls.Values = []identDeclaration{{name: "a", typ: "Alias"}, {name: "r", typ: "int"}, {name: "h", typ: "Holder"}, {name: "T", typ: "any"}}
			checkVariables(pass, result.variableToDist, cfg)
			checkConstants(pass, result.constantToDist, cfg)
			checkParams(pass, result.paramToDist, cfg)
			checkReturns(pass, result.returnToDist, cfg)
			checkReceivers(pass, result.receiverToDist, cfg)
			checkTypeParams(pass, result.typeParamToDist, cfg)
			for _, record := range allTypeRecords(result) {
				if record.name != "<type-switched>" && record.resolved {
					t.Fatalf("accepted name formatted type %q", record.name)
				}
			}
		})
	}
}

func TestTypeNameCacheIsPassLocal(t *testing.T) {
	t.Parallel()
	shared := types.NewPackage("example.org/shared", "shared")
	object := types.NewTypeName(token.NoPos, shared, "Value", nil)
	types.NewNamed(object, types.NewStruct(nil, nil), nil)
	shared.Scope().Insert(object)
	shared.MarkComplete()
	var previous *shortType
	for _, alias := range []string{"first", "second"} {
		pass := typeNamePass(t, "package fixture\nimport "+alias+" \"example.org/shared\"\nfunc f(a "+alias+".Value)(r "+alias+".Value) { b:=a;r=b;return }", sharedTypeImporter{shared})
		result := distances(pass, configuration{checkReturn: true})
		var sharedRecord *shortType
		for _, record := range allTypeRecords(result) {
			if record.resolved {
				t.Fatal("name was not deferred")
			}
			if sharedRecord == nil {
				sharedRecord = record
			} else if sharedRecord != record {
				t.Fatal("same type was not interned")
			}
		}
		if sharedRecord == nil {
			t.Fatal("fixture produced no records")
		}
		if previous == sharedRecord {
			t.Fatal("cache leaked between analysis passes")
		}
		if got := sharedRecord.String(); got != alias+".Value" {
			t.Fatalf("name=%q", got)
		}
		previous = sharedRecord
	}
}

type countedType struct{ calls int }

func (typ *countedType) String() string     { typ.calls++; return "example.org/shared.Value" }
func (*countedType) Underlying() types.Type { return types.Typ[types.Int] }

func TestTypeNamesCacheEmptyAndFormattedValues(t *testing.T) {
	t.Parallel()
	typ := &countedType{}
	record := &shortType{typ: typ, imports: []importDeclaration{{name: "alias", path: "example.org/shared"}}}
	for i := 0; i < 2; i++ {
		if got := record.String(); got != "alias.Value" {
			t.Fatalf("name=%q", got)
		}
	}
	if typ.calls != 1 {
		t.Fatalf("format calls=%d,want 1", typ.calls)
	}
	empty := &shortType{}
	if empty.String() != "" || !empty.resolved || empty.String() != "" {
		t.Fatal("nil type's empty name was not cached")
	}
}
