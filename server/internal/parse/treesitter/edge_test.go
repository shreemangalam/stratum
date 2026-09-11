package treesitter

import (
	"context"
	"testing"
)

func TestParseJavaScript_EmptyFile(t *testing.T) {
	p := NewGeneric("javascript", []string{".js"})
	tree, err := p.Parse(context.Background(), []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Root.Children) != 0 {
		t.Errorf("expected 0 children for empty file, got %d", len(tree.Root.Children))
	}
}

func TestParseJavaScript_OnlyComments(t *testing.T) {
	p := NewGeneric("javascript", []string{".js"})
	src := []byte("// just a comment\n/* block comment */\n")
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Root.Children) != 0 {
		t.Errorf("expected 0 children for comment-only file, got %d", len(tree.Root.Children))
	}
}

func TestParseJavaScript_ArrowFunctions(t *testing.T) {
	p := NewGeneric("javascript", []string{".js"})
	src := []byte(`const add = (a, b) => a + b;

const multiply = (a, b) => {
  return a * b;
};

let noop = () => {};
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	funcCount := 0
	for _, child := range tree.Root.Children {
		if child.Kind == "function_declaration" {
			funcCount++
		}
	}
	if funcCount < 2 {
		t.Errorf("expected at least 2 arrow functions detected, got %d", funcCount)
	}
}

func TestParseJavaScript_ExportDefault(t *testing.T) {
	p := NewGeneric("javascript", []string{".js"})
	src := []byte(`export default function main() {
  return 42;
}

export function helper() {}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Root.Children) < 2 {
		t.Errorf("expected at least 2 children, got %d", len(tree.Root.Children))
	}
}

func TestParseJavaScript_NestedClasses(t *testing.T) {
	p := NewGeneric("javascript", []string{".js"})
	src := []byte(`class Outer {
  method() {
    class Inner {
      innerMethod() {}
    }
    return new Inner();
  }
}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	foundOuter := false
	for _, child := range tree.Root.Children {
		if child.Kind == "class_declaration" && child.Label == "Outer" {
			foundOuter = true
		}
	}
	if !foundOuter {
		t.Error("expected to find class 'Outer'")
	}
}

func TestParseJavaScript_ImportVariants(t *testing.T) {
	p := NewGeneric("javascript", []string{".js"})
	src := []byte(`import { foo, bar } from 'module';
import * as baz from 'other';
import React from 'react';

function use() {}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	importCount := 0
	funcCount := 0
	for _, child := range tree.Root.Children {
		if child.Kind == "import_declaration" {
			importCount++
		}
		if child.Kind == "function_declaration" {
			funcCount++
		}
	}
	if importCount != 3 {
		t.Errorf("expected 3 imports, got %d", importCount)
	}
	if funcCount != 1 {
		t.Errorf("expected 1 function, got %d", funcCount)
	}
}

func TestParseTypeScript_InterfaceAndType(t *testing.T) {
	p := NewGeneric("typescript", []string{".ts"})
	src := []byte(`interface User {
  name: string;
  age: number;
}

type ID = string | number;

function getUser(id: ID): User {
  return { name: "test", age: 0 };
}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	foundInterface := false
	foundType := false
	foundFunc := false
	for _, child := range tree.Root.Children {
		switch child.Kind {
		case "interface_declaration":
			foundInterface = true
		case "type_alias":
			foundType = true
		case "function_declaration":
			foundFunc = true
		}
	}
	if !foundInterface {
		t.Error("expected interface_declaration")
	}
	if !foundType {
		t.Error("expected type_alias")
	}
	if !foundFunc {
		t.Error("expected function_declaration")
	}
}

func TestParsePython_EmptyFile(t *testing.T) {
	p := NewGeneric("python", []string{".py"})
	tree, err := p.Parse(context.Background(), []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Root.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(tree.Root.Children))
	}
}

func TestParsePython_ClassWithDecorators(t *testing.T) {
	p := NewGeneric("python", []string{".py"})
	src := []byte(`import os

class Config:
    def __init__(self):
        self.debug = False

    def enable_debug(self):
        self.debug = True

def standalone():
    pass
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	importCount := 0
	classCount := 0
	funcCount := 0
	for _, child := range tree.Root.Children {
		switch child.Kind {
		case "import_statement":
			importCount++
		case "class_definition":
			classCount++
			methodCount := 0
			for _, m := range child.Children {
				if m.Kind == "method_definition" {
					methodCount++
				}
			}
			if methodCount != 2 {
				t.Errorf("expected 2 methods in Config, got %d", methodCount)
			}
		case "function_definition":
			funcCount++
		}
	}
	if importCount != 1 {
		t.Errorf("expected 1 import, got %d", importCount)
	}
	if classCount != 1 {
		t.Errorf("expected 1 class, got %d", classCount)
	}
	if funcCount != 1 {
		t.Errorf("expected 1 standalone function, got %d", funcCount)
	}
}

func TestParsePython_NestedFunctions(t *testing.T) {
	p := NewGeneric("python", []string{".py"})
	src := []byte(`def outer():
    def inner():
        return 42
    return inner()
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if len(tree.Root.Children) != 1 {
		t.Errorf("expected 1 top-level function, got %d", len(tree.Root.Children))
	}
	if tree.Root.Children[0].Label != "outer" {
		t.Errorf("expected 'outer', got %q", tree.Root.Children[0].Label)
	}
}

func TestParsePython_MultilineString(t *testing.T) {
	p := NewGeneric("python", []string{".py"})
	src := []byte(`def documented():
    """
    This is a docstring.
    It spans multiple lines.
    """
    return True

def other():
    pass
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if len(tree.Root.Children) != 2 {
		t.Errorf("expected 2 functions, got %d", len(tree.Root.Children))
	}
}

func TestParseJava_EmptyFile(t *testing.T) {
	p := NewGeneric("java", []string{".java"})
	tree, err := p.Parse(context.Background(), []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Root.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(tree.Root.Children))
	}
}

func TestParseJava_InterfaceWithDefaults(t *testing.T) {
	p := NewGeneric("java", []string{".java"})
	src := []byte(`package com.example;

public interface Sortable {
    int compareTo(Object other);

    default boolean isLessThan(Object other) {
        return compareTo(other) < 0;
    }
}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	foundInterface := false
	for _, child := range tree.Root.Children {
		if child.Kind == "interface_declaration" && child.Label == "Sortable" {
			foundInterface = true
			if len(child.Children) < 2 {
				t.Errorf("expected at least 2 members, got %d", len(child.Children))
			}
		}
	}
	if !foundInterface {
		t.Error("expected interface 'Sortable'")
	}
}

func TestParseJava_EnumDeclaration(t *testing.T) {
	p := NewGeneric("java", []string{".java"})
	src := []byte(`package com.example;

public enum Color {
    RED,
    GREEN,
    BLUE;

    public String display() {
        return name().toLowerCase();
    }
}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	foundEnum := false
	for _, child := range tree.Root.Children {
		if child.Kind == "enum_declaration" && child.Label == "Color" {
			foundEnum = true
		}
	}
	if !foundEnum {
		t.Error("expected enum 'Color'")
	}
}

func TestParseJava_MultipleImports(t *testing.T) {
	p := NewGeneric("java", []string{".java"})
	src := []byte(`package com.test;

import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

public class Main {
    public static void main(String[] args) {}
}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	importCount := 0
	for _, child := range tree.Root.Children {
		if child.Kind == "import_declaration" {
			importCount++
		}
	}
	if importCount != 3 {
		t.Errorf("expected 3 imports, got %d", importCount)
	}
}

func TestParseJava_Annotations(t *testing.T) {
	p := NewGeneric("java", []string{".java"})
	src := []byte(`package com.test;

public class Service {
    @Override
    public String toString() {
        return "Service";
    }

    @Deprecated
    public void oldMethod() {}
}
`)
	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	foundClass := false
	for _, child := range tree.Root.Children {
		if child.Kind == "class_declaration" && child.Label == "Service" {
			foundClass = true
			methodCount := 0
			for _, m := range child.Children {
				if m.Kind == "method_declaration" {
					methodCount++
				}
			}
			if methodCount < 2 {
				t.Errorf("expected at least 2 methods, got %d", methodCount)
			}
		}
	}
	if !foundClass {
		t.Error("expected class 'Service'")
	}
}

func TestParseGeneric_WhitespaceOnly(t *testing.T) {
	p := NewGeneric("c", []string{".c"})
	tree, err := p.Parse(context.Background(), []byte("   \n  \n\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range tree.Root.Children {
		if child.Kind != "line" {
			t.Errorf("expected line nodes, got %q", child.Kind)
		}
	}
}

func TestParseGeneric_EmptyFile(t *testing.T) {
	p := NewGeneric("c", []string{".c"})
	tree, err := p.Parse(context.Background(), []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Root.Children) != 0 {
		t.Errorf("expected 0 children for empty file, got %d", len(tree.Root.Children))
	}
}
