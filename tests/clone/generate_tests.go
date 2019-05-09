package main

import (
	"errors"
	"fmt"
	"go/parser"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/loader"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "%s ./model/foo > tests/clone/generated/foo_test.go\n", os.Args[0])
		os.Exit(1)
	}
	infos := extractInfos(os.Args[1:])
	generateTests(infos)
}

// TODO we should test recursively. For example, apps.WebappManifest has
// Intents []Intent, and Intent has Types []string: we should check that
// modifying the types of the intents of a manifest has no effect on its clone.
func extractInfos(pkgs []string) []info {
	infos := make([]info, 0)

	for _, pkgPath := range pkgs {
		pkg, err := pkgInfoFromPath(pkgPath)
		if err != nil {
			panic(err)
		}
		in := info{
			PkgName: pkg.Name(),
			PkgPath: pkg.Path(),
			Structs: make(map[string][]mutableField),
		}
		scope := pkg.Scope()

		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			s, ok := obj.Type().Underlying().(*types.Struct)
			if !ok {
				continue
			}
			if !hasCloneMethod(obj) {
				continue
			}

			switch obj.Name() {
			// Ignore structs that have a Clone method that panics
			case "TreeFile", "DirOrFileDoc", "APICredentials", "FileDocWithRevisions", "APISharing":
				continue
			}

			fields := make([]mutableField, 0)
			for i := 0; i < s.NumFields(); i++ {
				field := s.Field(i)
				if !field.Exported() {
					fmt.Fprintf(os.Stderr, "Warning: cannot check unexported field: %s.%s -> %s\n",
						pkg.Name(), name, field)
					continue
				}
				switch t := field.Type().(type) {
				case (*types.Slice):
					fields = append(fields, &sliceField{
						Name:  field.Name(),
						Value: generatorForType(t.Elem()),
					})
				case (*types.Map):
					fields = append(fields, &mapField{
						Name:  field.Name(),
						Key:   generatorForType(t.Key()),
						Value: generatorForType(t.Elem()),
					})
				case (*types.Named):
					switch u := t.Underlying().(type) {
					case (*types.Basic):
						continue
					case (*types.Signature): // functions or methods
						continue
					case (*types.Slice):
						fields = append(fields, &sliceField{
							Name:  field.Name(),
							Value: generatorForType(u.Elem()),
						})
						continue
					case (*types.Map):
						fields = append(fields, &mapField{
							Name:  field.Name(),
							Key:   generatorForType(u.Key()),
							Value: generatorForType(u.Elem()),
						})
						continue
					case (*types.Struct):
						if allFieldsAreBasic(u) {
							continue
						}
					}
					var named string
					if p := t.Obj().Pkg(); p != nil {
						named = fmt.Sprintf("%s.%s", p.Name(), t.Obj().Name())
					}
					switch named {
					case "time.Time", "time.Duration":
						// These structs are known to be safe
					case "app.SubDomainer":
						// This struct is just an interface used for JSON-API links
					default:
						panic(fmt.Errorf("Unknown named type: %s", named))
					}
				case (*types.Interface):
					fmt.Fprintf(os.Stderr, "Warning: cannot check interfaces: %s.%s -> %s\n",
						pkg.Name(), name, field)
				case (*types.Pointer):
					if gen := generatorForType(t.Elem()); gen != nil {
						fields = append(fields, &ptrField{
							Name:  field.Name(),
							Value: gen,
						})
					}
				case (*types.Basic):
					// Basic types are immutables
				default:
					panic(fmt.Errorf("Unknown type: %#v", field.Type()))
				}
			}
			if len(fields) > 0 {
				in.Structs[name] = fields
			}
		}
		if len(in.Structs) > 0 {
			infos = append(infos, in)
		}
	}
	return infos
}

func generateTests(infos []info) {
	fmt.Printf(`// Generated tests for Clone(). Do not manually edit!
package clone

import (
	"testing"
`)
	for _, info := range infos {
		if info.PkgPath == "github.com/cozy/cozy-stack/model/vfs" {
			fmt.Printf("\t\"github.com/cozy/cozy-stack/pkg/couchdb\"\n")
		}
		if info.PkgPath == "github.com/cozy/cozy-stack/pkg/config/dynamic" {
			fmt.Printf("\t\"github.com/cozy/cozy-stack/pkg/statik/fs\"\n")
		}
		fmt.Printf("\t\"%s\"\n", info.PkgPath)
	}
	fmt.Printf(")\n\n")

	for _, info := range infos {
		fmt.Printf("func Test%s(t *testing.T) {\n", strings.Title(info.PkgName))
		for name, fields := range info.Structs {
			v := strings.ToLower(name)
			fmt.Printf("\t%sA := &%s.%s{}\n", v, info.PkgName, name)
			for _, field := range fields {
				field.Initialize(v)
			}
			ptr := "*"
			if name == "JSONDoc" {
				ptr = ""
			}
			fmt.Printf("\t%sB := %sA.Clone().(%s%s.%s)\n", v, v, ptr, info.PkgName, name)
			for _, field := range fields {
				field.Reassign(v)
				field.Compare(v)
				fmt.Printf("\t\tt.Fatalf(\"Error for clone %s.%s -> %s\")\n\t}\n", info.PkgName, name, field)
			}
		}
		fmt.Printf("}\n\n")
	}
}

type info struct {
	PkgName string
	PkgPath string
	Structs map[string][]mutableField
}

type mutableField interface {
	String() string
	Initialize(v string)
	Reassign(v string)
	Compare(v string)
}

type sliceField struct {
	Name  string
	Value *generator
}

func (f *sliceField) String() string { return f.Name }

func (f *sliceField) Initialize(v string) {
	if f.Value.Warning != "" {
		fmt.Printf("\t// Warning: %s", f.Value.Warning)
	}
	fmt.Printf("\t%sA.%s = []%s{%s}\n", v, f.Name, f.Value.Type, f.Value.Initial)
}
func (f *sliceField) Reassign(v string) {
	fmt.Printf("\t%sA.%s[0]%s = %s\n", v, f.Name, f.Value.SubKey, f.Value.Altered)
}
func (f *sliceField) Compare(v string) {
	fmt.Printf("\tif %sB.%s[0]%s != %s {\n", v, f.Name, f.Value.SubKey, f.Value.SubValue)
}

type mapField struct {
	Name  string
	Key   *generator
	Value *generator
}

func (f *mapField) String() string { return f.Name }

func (f *mapField) Initialize(v string) {
	if f.Value.Warning != "" {
		fmt.Printf("\t// Warning: %s\n", f.Value.Warning)
	}
	fmt.Printf("\t%sA.%s = map[%s]%s{%s: %s}\n", v, f.Name, f.Key.Type, f.Value.Type, f.Key.Key, f.Value.Initial)
}
func (f *mapField) Reassign(v string) {
	if f.Value.SubKey == "" {
		fmt.Printf("\t%sA.%s[%s] = %s\n", v, f.Name, f.Key.Key, f.Value.Altered)
	} else {
		fmt.Printf(`	{
		tmp := %sA.%s[%s]
		tmp%s = %s
		%sA.%s[%s] = tmp
	}
`, v, f.Name, f.Key.Key, f.Value.SubKey, f.Value.Altered, v, f.Name, f.Key.Key)
	}
}
func (f *mapField) Compare(v string) {
	fmt.Printf("\tif %sB.%s[%s]%s != %s {\n", v, f.Name, f.Key.Key, f.Value.SubKey, f.Value.SubValue)
}

type ptrField struct {
	Name  string
	Value *generator
}

func (f *ptrField) String() string { return f.Name }

func (f *ptrField) Initialize(v string) {
	if f.Value.Warning != "" {
		fmt.Printf("\t// Warning: %s", f.Value.Warning)
	}
	fmt.Printf("\t%sA.%s = func() *%s { tmp := %s; return &tmp }()\n", v, f.Name, f.Value.Type, f.Value.Initial)
}
func (f *ptrField) Reassign(v string) {
	fmt.Printf("\t%sA.%s%s = %s\n", v, f.Name, f.Value.SubKey, f.Value.Altered)
}
func (f *ptrField) Compare(v string) {
	fmt.Printf("\tif %sB.%s%s != %s {\n", v, f.Name, f.Value.SubKey, f.Value.SubValue)
}

func generatorForType(typ types.Type) *generator {
	switch t := typ.(type) {
	case (*types.Basic):
		switch t.Name() {
		case "string":
			return &stringGenerator
		case "byte":
			return &byteGenerator
		case "int":
			return &intGenerator
		case "uint64":
			return &uint64Generator
		default:
			panic(fmt.Errorf("Unknown basic type: %s", t.Name()))
		}
	case (*types.Interface):
		if t.Empty() {
			return &emptyInterfaceGenerator
		}
	case (*types.Named):
		named := fmt.Sprintf("%s.%s", t.Obj().Pkg().Name(), t.Obj().Name())
		if named == "json.RawMessage" || named == "time.Time" {
			// We consider that json.RawMessage and time.Time are not modifiable in our code
			return nil
		}
		if s, ok := t.Obj().Type().Underlying().(*types.Struct); ok {
			return structGenerator(named, s)
		}
	case (*types.Pointer):
		return ptrGenerator(t)
	}
	panic(fmt.Errorf("Unknown generator type: %#v", typ))
}

type generator struct {
	Type     string
	Key      string
	Initial  string
	Altered  string
	Warning  string
	SubKey   string
	SubValue string
}

var byteGenerator = generator{
	Type:     "byte",
	Key:      `'a'`,
	Initial:  `'b'`,
	Altered:  `'c'`,
	SubValue: `'b'`,
}

var stringGenerator = generator{
	Type:     "string",
	Key:      `"foo"`,
	Initial:  `"bar"`,
	Altered:  `"baz"`,
	SubValue: `"bar"`,
}

var intGenerator = generator{
	Type:     "int",
	Key:      "0",
	Initial:  "1",
	Altered:  "2",
	SubValue: "1",
}

var uint64Generator = generator{
	Type:     "uint64",
	Key:      "0",
	Initial:  "1",
	Altered:  "2",
	SubValue: "1",
}

var emptyInterfaceGenerator = generator{
	Type:     "interface{}",
	Key:      "0",
	Initial:  "1",
	Altered:  "2",
	SubValue: "1",
	Warning:  "interface{} can contain nested data!",
}

func structGenerator(name string, s *types.Struct) *generator {
	var f *types.Var
	for i := 0; i < s.NumFields(); i++ {
		ff := s.Field(i)
		if ff.Exported() {
			f = ff
			break
		}
	}
	if f == nil {
		panic(fmt.Errorf("No generator for a struct with no exported fields: %s", s))
	}
	g := generatorForType(f.Type())
	return &generator{
		Type:     name,
		Initial:  fmt.Sprintf("%s{%s: %s}", name, f.Name(), g.Initial),
		Altered:  g.Altered,
		SubKey:   "." + f.Name(),
		SubValue: g.SubValue,
	}
}

func ptrGenerator(t *types.Pointer) *generator {
	g := generatorForType(t.Elem())
	if g == nil {
		panic(fmt.Errorf("No generator for pointers to %s", t.Elem()))
	}
	return &generator{
		Type:     "*" + g.Type,
		Initial:  "&" + g.Initial,
		Altered:  g.Altered,
		SubKey:   g.SubKey,
		SubValue: g.SubValue,
	}
}

func hasCloneMethod(obj types.Object) bool {
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return false
	}
	for i := 0; i < named.NumMethods(); i++ {
		m := named.Method(i)
		if m.Name() != "Clone" {
			continue
		}
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			continue
		}
		if sig.Params().Len() != 0 {
			return false
		}
		if sig.Results().Len() != 1 {
			return false
		}
		r, ok := sig.Results().At(0).Type().(*types.Named)
		if !ok {
			return false
		}
		o := r.Obj()
		return o.Name() == "Doc" && o.Pkg().Name() == "couchdb"
	}
	return false
}

func allFieldsAreBasic(s *types.Struct) bool {
	for i := 0; i < s.NumFields(); i++ {
		if _, ok := s.Field(i).Type().(*types.Basic); !ok {
			return false
		}
	}
	return true
}

// pkgInfoFromPath returns information about the package
// Taken from https://github.com/matryer/moq
func pkgInfoFromPath(src string) (*types.Package, error) {
	abs, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}
	pkgFull := stripGopath(abs)

	conf := loader.Config{
		ParserMode: parser.SpuriousErrors,
	}
	conf.Import(pkgFull)
	lprog, err := conf.Load()
	if err != nil {
		return nil, err
	}

	pkgInfo := lprog.Package(pkgFull)
	if pkgInfo == nil {
		return nil, errors.New("package was nil")
	}

	return pkgInfo.Pkg, nil
}

// stripGopath takes the directory to a package and remove the gopath to get the
// canonical package name.
// Taken from https://github.com/ernesto-jimenez/gogen
func stripGopath(p string) string {
	for _, gopath := range gopaths() {
		p = strings.TrimPrefix(p, path.Join(gopath, "src")+"/")
	}
	return p
}

func gopaths() []string {
	return strings.Split(os.Getenv("GOPATH"), string(filepath.ListSeparator))
}
