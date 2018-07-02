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
		fmt.Fprintf(os.Stderr, "%s ./pkg/foo > tests/clone/generated/foo_test.go\n", os.Args[0])
		os.Exit(1)
	}
	infos := extractInfos(os.Args[1:])
	generateTests(infos)
}

func extractInfos(pkgs []string) []info {
	infos := make([]info, 0)
	docIface := getDocIface()

	for _, pkgPath := range pkgs {
		pkg, err := pkgInfoFromPath(pkgPath)
		if err != nil {
			panic(err)
		}

		info := info{
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
			ptr := types.NewPointer(obj.Type())
			// FIXME implements returns false for intents.Intent but it should
			// return true, find why!
			// implements := types.Implements(ptr.Underlying(), docIface)
			f, g := types.MissingMethod(ptr.Underlying(), docIface, true)
			if f != nil && !g {
				continue
			}
			fields := make([]mutableField, 0)
			for i := 0; i < s.NumFields(); i++ {
				field := s.Field(i)
				// fmt.Printf(" - %d. %s - %s\n", i, field.Name(), field.Type())
				switch f := field.Type().(type) {
				case (*types.Slice):
					fields = append(fields, &sliceField{
						Name:  field.Name(),
						Value: generatorForType(f.Elem()),
					})
				case (*types.Map):
					fields = append(fields, &mapField{
						Name:  field.Name(),
						Key:   generatorForType(f.Key()),
						Value: generatorForType(f.Elem()),
					})
				case (*types.Named):
					named := fmt.Sprintf("%s.%s", f.Obj().Pkg().Name(), f.Obj().Name())
					switch named {
					case "time.Time", "time.Duration":
						// These structs are known to be safe
					default:
						panic(fmt.Errorf("Unknown named type: %s", named))
					}
				case (*types.Interface):
					fmt.Fprintf(os.Stderr, "Warning: cannot check interfaces: %s.%s -> %s\n",
						pkg.Name(), name, field)
				case (*types.Basic):
					// Basic types are immutables
				default:
					panic(fmt.Errorf("Unknown type: %#v", field.Type()))
				}
			}
			if len(fields) > 0 {
				info.Structs[name] = fields
			}
		}
		if len(info.Structs) > 0 {
			infos = append(infos, info)
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
			fmt.Printf("\t%sB := %sA.Clone().(*%s.%s)\n", v, v, info.PkgName, name)
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
	Value generator
}

func (f *sliceField) String() string { return f.Name }

func (f *sliceField) Initialize(v string) {
	if f.Value.Warning != "" {
		fmt.Printf("\t// Warning: %s", f.Value.Warning)
	}
	fmt.Printf("\t%sA.%s = []%s{%s}\n", v, f.Name, f.Value.Type, f.Value.Initial)
}
func (f *sliceField) Reassign(v string) {
	fmt.Printf("\t%sA.%s[0] = %s\n", v, f.Name, f.Value.Altered)
}
func (f *sliceField) Compare(v string) {
	fmt.Printf("\tif %sB.%s[0] != %s {\n", v, f.Name, f.Value.Initial)
}

type mapField struct {
	Name  string
	Key   generator
	Value generator
}

func (f *mapField) String() string { return f.Name }

func (f *mapField) Initialize(v string) {
	if f.Value.Warning != "" {
		fmt.Printf("\t// Warning: %s\n", f.Value.Warning)
	}
	fmt.Printf("\t%sA.%s = map[%s]%s{%s: %s}\n", v, f.Name, f.Key.Type, f.Value.Type, f.Key.Key, f.Value.Initial)
}
func (f *mapField) Reassign(v string) {
	fmt.Printf("\t%sA.%s[%s] = %s\n", v, f.Name, f.Key.Key, f.Value.Altered)
}
func (f *mapField) Compare(v string) {
	fmt.Printf("\tif %sB.%s[%s] != %s {\n", v, f.Name, f.Key.Key, f.Value.Initial)
}

func generatorForType(typ types.Type) generator {
	switch t := typ.(type) {
	case (*types.Basic):
		switch t.Name() {
		case "string":
			return stringGenerator
		default:
			panic(fmt.Errorf("Unknown basic type: %s", t.Name()))
		}
	case (*types.Interface):
		if t.Empty() {
			return emptyInterfaceGenerator
		}
	}
	//return stringGenerator
	panic(fmt.Errorf("Unknown generator type: %#v", typ))
}

type generator struct {
	Type    string
	Key     string
	Initial string
	Altered string
	Warning string
}

var stringGenerator = generator{
	Type:    "string",
	Key:     `"foo"`,
	Initial: `"bar"`,
	Altered: `"baz"`,
}

var emptyInterfaceGenerator = generator{
	Type:    "interface{}",
	Key:     "0",
	Initial: "1",
	Altered: "2",
	Warning: "interface{} can contain nested data!",
}

// getDocIface returns the couchdb.Doc interface
func getDocIface() *types.Interface {
	couchPkg := "github.com/cozy/cozy-stack/pkg/couchdb"
	conf := loader.Config{
		ParserMode: parser.SpuriousErrors,
	}
	conf.Import(couchPkg)
	lprog, err := conf.Load()
	if err != nil {
		panic(err)
	}
	scope := lprog.Package(couchPkg).Pkg.Scope()
	return scope.Lookup("Doc").Type().Underlying().(*types.Interface)
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
