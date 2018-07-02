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

type mutableField struct {
	Name string
}

type info struct {
	PkgName string
	PkgPath string
	Structs map[string][]mutableField
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
				switch field.Type().(type) {
				case (*types.Slice):
					// fmt.Printf("\t\tSlice\n")
				// case (*types.Named):
				// 	fmt.Printf("\t\tNamed\n")
				default:
					continue
				}
				fields = append(fields, mutableField{
					Name: field.Name(),
				})
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
			fmt.Printf("\t%s := &%s.%s{}\n", v, info.PkgName, name)
			for _, field := range fields {
				fmt.Printf("\t%s.%s = []string{\"foo\"}\n", v, field.Name)
			}
			fmt.Printf("\t%sCloned := %s.Clone().(*%s.%s)\n", v, v, info.PkgName, name)
			for _, field := range fields {
				fmt.Printf("\t%s.%s[0] = \"bar\"\n", v, field.Name)
				fmt.Printf("\tif %sCloned.%s[0] != \"foo\" {\n", v, field.Name)
				fmt.Printf("\t\tt.Fatalf(\"Error for clone %s.%s.%s\")\n\t}\n", info.PkgName, name, field.Name)
			}
		}
		fmt.Printf("}\n\n")
	}
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
