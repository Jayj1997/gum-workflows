package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const analyzerAPIVersion = "goComplexityAnalyzer/v1"

type output struct {
	APIVersion   string        `json:"apiVersion"`
	Functions    []function    `json:"functions"`
	SyntaxErrors []syntaxError `json:"syntaxErrors"`
}

type sourceMetadata struct {
	File      string
	Test      bool
	Generated bool
	Vendor    bool
}

type function struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Name       string `json:"name"`
	Complexity int    `json:"complexity"`
	Test       bool   `json:"test"`
	Generated  bool   `json:"generated"`
	Vendor     bool   `json:"vendor"`
}

type syntaxError struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Message   string `json:"message"`
	Test      bool   `json:"test"`
	Generated bool   `json:"generated"`
	Vendor    bool   `json:"vendor"`
}

type listedPackage struct {
	Dir            string
	GoFiles        []string
	CgoFiles       []string
	TestGoFiles    []string
	XTestGoFiles   []string
	InvalidGoFiles []string
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: analyzer workspace output")
		os.Exit(2)
	}
	workspace, destination := os.Args[1], os.Args[2]
	result := output{APIVersion: analyzerAPIVersion, Functions: []function{}, SyntaxErrors: []syntaxError{}}
	sources, err := listSources(workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, source := range sources {
		if err := analyzeSource(&result, workspace, source); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	sort.Slice(result.Functions, func(i, j int) bool {
		if result.Functions[i].File == result.Functions[j].File {
			return result.Functions[i].Line < result.Functions[j].Line
		}
		return result.Functions[i].File < result.Functions[j].File
	})
	data, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(destination, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func listSources(workspace string) ([]sourceMetadata, error) {
	command := exec.Command("go", "list", "-e", "-json", "./...")
	command.Dir = workspace
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("go list ./...: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	decoder := json.NewDecoder(&stdout)
	sources := map[string]sourceMetadata{}
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go list ./... output: %w", err)
		}
		for _, group := range []struct {
			files []string
			test  bool
		}{
			{files: pkg.GoFiles}, {files: pkg.CgoFiles},
			{files: pkg.TestGoFiles, test: true}, {files: pkg.XTestGoFiles, test: true},
			{files: pkg.InvalidGoFiles},
		} {
			for _, name := range group.files {
				path := filepath.Join(pkg.Dir, name)
				relative, err := filepath.Rel(workspace, path)
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					return nil, fmt.Errorf("go list returned source outside workspace: %s", path)
				}
				sources[path] = sourceMetadata{File: filepath.ToSlash(relative), Test: group.test || strings.HasSuffix(name, "_test.go"), Vendor: hasPathComponent(relative, "vendor")}
			}
		}
	}
	ordered := make([]sourceMetadata, 0, len(sources))
	for path, source := range sources {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", source.File, err)
		}
		source.Generated = generatedFile(data)
		ordered = append(ordered, source)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].File < ordered[j].File })
	return ordered, nil
}

func analyzeSource(result *output, workspace string, source sourceMetadata) error {
	path := filepath.Join(workspace, filepath.FromSlash(source.File))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", source.File, err)
	}
	set := token.NewFileSet()
	file, parseErr := parser.ParseFile(set, path, data, parser.AllErrors)
	if parseErr != nil {
		if list, ok := parseErr.(scanner.ErrorList); ok {
			for _, item := range list {
				result.SyntaxErrors = append(result.SyntaxErrors, syntaxError{File: source.File, Line: item.Pos.Line, Message: item.Msg, Test: source.Test, Generated: source.Generated, Vendor: source.Vendor})
			}
		} else {
			result.SyntaxErrors = append(result.SyntaxErrors, syntaxError{File: source.File, Line: 1, Message: parseErr.Error(), Test: source.Test, Generated: source.Generated, Vendor: source.Vendor})
		}
	}
	if file != nil {
		collectFunctions(result, set, file, source)
	}
	return nil
}

func collectFunctions(result *output, set *token.FileSet, file *ast.File, source sourceMetadata) {
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			if typed.Body == nil {
				return false
			}
			position := set.Position(typed.Pos())
			result.Functions = append(result.Functions, newFunction(source, position.Line, typed.Name.Name, complexity(typed.Body)))
			collectFuncLiterals(result, set, typed.Body, source)
			return false
		case *ast.FuncLit:
			position := set.Position(typed.Pos())
			result.Functions = append(result.Functions, newFunction(source, position.Line, fmt.Sprintf("func literal at line %d", position.Line), complexity(typed.Body)))
			collectFuncLiterals(result, set, typed.Body, source)
			return false
		}
		return true
	})
}

func collectFuncLiterals(result *output, set *token.FileSet, body *ast.BlockStmt, source sourceMetadata) {
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		position := set.Position(literal.Pos())
		result.Functions = append(result.Functions, newFunction(source, position.Line, fmt.Sprintf("func literal at line %d", position.Line), complexity(literal.Body)))
		collectFuncLiterals(result, set, literal.Body, source)
		return false
	})
}

func newFunction(source sourceMetadata, line int, name string, value int) function {
	return function{File: source.File, Line: line, Name: name, Complexity: value, Test: source.Test, Generated: source.Generated, Vendor: source.Vendor}
}

func complexity(body *ast.BlockStmt) int {
	value := 1
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && literal.Body != body {
			return false
		}
		switch typed := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			value++
		case *ast.CaseClause:
			if len(typed.List) > 0 {
				value++
			}
		case *ast.CommClause:
			if typed.Comm != nil {
				value++
			}
		case *ast.BinaryExpr:
			if typed.Op == token.LAND || typed.Op == token.LOR {
				value++
			}
		}
		return true
	})
	return value
}

func generatedFile(data []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(trimmed, "package ") {
			return false
		}
		if strings.HasPrefix(trimmed, "// Code generated ") && strings.HasSuffix(trimmed, " DO NOT EDIT.") {
			return true
		}
	}
	return false
}

func hasPathComponent(path, component string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == component {
			return true
		}
	}
	return false
}
