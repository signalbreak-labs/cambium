// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/yangpattern"
)

// patternRuntime is rendered from the explicit upstream inventory, never a
// second implementation. Only package and import declarations are removed.
type patternRuntime struct {
	declarations string
	imports      []string
	names        []string
}

func readPatternRuntime(sources []yangpattern.SourceFile) (patternRuntime, error) {
	var runtime patternRuntime
	var declarations strings.Builder
	for _, source := range sources {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, source.Name, source.Data, parser.ParseComments)
		if err != nil {
			return patternRuntime{}, fmt.Errorf("pattern runtime %s: %w", source.Name, err)
		}
		fmt.Fprintf(&declarations, "// Shared yangpattern runtime: %s\n", source.Name)
		start := 0
		// Keep attribution and all declaration comments verbatim.
		declarations.Write(source.Data[:fset.Position(file.Package).Offset])
		start = fset.Position(file.Name.End()).Offset
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
				declarations.Write(source.Data[start:fset.Position(gen.Pos()).Offset])
				start = fset.Position(gen.End()).Offset
				for _, spec := range gen.Specs {
					imp := spec.(*ast.ImportSpec)
					text := imp.Path.Value
					if imp.Name != nil {
						text = imp.Name.Name + " " + text
					}
					runtime.imports = append(runtime.imports, text)
				}
				continue
			}
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					runtime.names = append(runtime.names, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						runtime.names = append(runtime.names, s.Name.Name)
					case *ast.ValueSpec:
						for _, name := range s.Names {
							runtime.names = append(runtime.names, name.Name)
						}
					}
				}
			}
		}
		declarations.Write(source.Data[start:])
		declarations.WriteString("\n")
	}
	runtime.declarations = declarations.String()
	return runtime, nil
}

func programDataLiteral(data yangpattern.ProgramData) string {
	var out strings.Builder
	fmt.Fprintf(&out, "ProgramData{Version:%d,Source:%s,Root:%d,Nodes:[]NodeData{", data.Version, strconv.Quote(data.Source), data.Root)
	for _, n := range data.Nodes {
		fmt.Fprintf(&out, "{Op:Op(%d),Min:%q,Max:%q,Unbounded:%t,Children:[]uint32{", n.Op, n.Min, n.Max, n.Unbounded)
		for _, child := range n.Children {
			fmt.Fprintf(&out, "%d,", child)
		}
		out.WriteString("},Ranges:[]RuneRange{")
		for _, r := range n.Ranges {
			fmt.Fprintf(&out, "{Lo:%d,Hi:%d},", r.Lo, r.Hi)
		}
		out.WriteString("}},")
	}
	out.WriteString("}}")
	return out.String()
}
