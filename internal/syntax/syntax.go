// Package syntax parses source code with the official tree-sitter C binding (D-40).
// It is the only package that knows about the engine. Nothing draws highlights yet:
// for now it proves the C build works on all three systems (M0-03).
package syntax

import (
	"fmt"

	ts "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Check parses src and reports whether the parser found syntax errors.
func Check(language string, src []byte) (hasErrors bool, err error) {
	var grammar *ts.Language
	switch language {
	case "go":
		grammar = ts.NewLanguage(golang.Language())
	case "javascript":
		grammar = ts.NewLanguage(javascript.Language())
	case "python":
		grammar = ts.NewLanguage(python.Language())
	case "typescript":
		grammar = ts.NewLanguage(typescript.LanguageTypescript())
	default:
		return false, fmt.Errorf("no grammar for %q", language)
	}
	parser := ts.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(grammar); err != nil {
		return false, err
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return false, fmt.Errorf("parsing %s returned no tree", language)
	}
	defer tree.Close()
	return tree.RootNode().HasError(), nil
}
