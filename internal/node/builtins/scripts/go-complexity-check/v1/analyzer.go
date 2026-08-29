package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type output struct {
	APIVersion   string        `json:"apiVersion"`
	Functions    []function    `json:"functions"`
	SyntaxErrors []syntaxError `json:"syntaxErrors"`
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

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: analyzer workspace output")
		os.Exit(2)
	}
	workspace, destination := os.Args[1], os.Args[2]
	result := output{APIVersion: "goComplexityAnalyzer/v1", Functions: []function{}, SyntaxErrors: []syntaxError{}}
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		vendor := relative == "vendor" || strings.HasPrefix(filepath.ToSlash(relative), "vendor/")
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		generated := generatedFile(data)
		test := strings.HasSuffix(entry.Name(), "_test.go")
		set := token.NewFileSet()
		file, parseErr := parser.ParseFile(set, path, data, parser.AllErrors)
		if parseErr != nil {
			if list, ok := parseErr.(scanner.ErrorList); ok {
				for _, item := range list {
					result.SyntaxErrors = append(result.SyntaxErrors, syntaxError{File: filepath.ToSlash(relative), Line: item.Pos.Line, Message: item.Msg, Test: test, Generated: generated, Vendor: vendor})
				}
			} else {
				result.SyntaxErrors = append(result.SyntaxErrors, syntaxError{File: filepath.ToSlash(relative), Line: 1, Message: parseErr.Error(), Test: test, Generated: generated, Vendor: vendor})
			}
		}
		if file != nil {
			collectFunctions(&result, set, file, filepath.ToSlash(relative), test, generated, vendor)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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

func collectFunctions(result *output, set *token.FileSet, file *ast.File, relative string, test, generated, vendor bool) {
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			if typed.Body == nil {
				return false
			}
			position := set.Position(typed.Pos())
			result.Functions = append(result.Functions, function{File: relative, Line: position.Line, Name: typed.Name.Name, Complexity: complexity(typed.Body), Test: test, Generated: generated, Vendor: vendor})
			collectFuncLiterals(result, set, typed.Body, relative, test, generated, vendor)
			return false
		case *ast.FuncLit:
			position := set.Position(typed.Pos())
			result.Functions = append(result.Functions, function{File: relative, Line: position.Line, Name: fmt.Sprintf("func literal at line %d", position.Line), Complexity: complexity(typed.Body), Test: test, Generated: generated, Vendor: vendor})
			return false
		}
		return true
	})
}

func collectFuncLiterals(result *output, set *token.FileSet, body *ast.BlockStmt, relative string, test, generated, vendor bool) {
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		position := set.Position(literal.Pos())
		result.Functions = append(result.Functions, function{File: relative, Line: position.Line, Name: fmt.Sprintf("func literal at line %d", position.Line), Complexity: complexity(literal.Body), Test: test, Generated: generated, Vendor: vendor})
		collectFuncLiterals(result, set, literal.Body, relative, test, generated, vendor)
		return false
	})
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
