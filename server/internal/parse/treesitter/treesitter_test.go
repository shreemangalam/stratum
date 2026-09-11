package treesitter

import (
	"context"
	"testing"
)

func TestParseGo_BasicFunction(t *testing.T) {
	p := NewGo()

	src := []byte(`package main

func hello() {
	fmt.Println("hello")
}

func world() {
	fmt.Println("world")
}
`)

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if tree.Root == nil {
		t.Fatal("expected non-nil root")
	}
	if tree.Language != "go" {
		t.Errorf("expected language 'go', got %q", tree.Language)
	}
	if tree.Size() < 5 {
		t.Errorf("expected at least 5 nodes, got %d", tree.Size())
	}
}

func TestParseGo_ExtractsLabels(t *testing.T) {
	p := NewGo()

	src := []byte(`package main

func myFunction(x int, y string) int {
	return 0
}
`)

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	foundFunc := false
	for _, n := range tree.NodeMap {
		if n.Kind == "function_declaration" && n.Label == "myFunction" {
			foundFunc = true
			break
		}
	}
	if !foundFunc {
		t.Error("expected to find function_declaration with label 'myFunction'")
	}
}

func TestParseGo_StructAndInterface(t *testing.T) {
	p := NewGo()

	src := []byte(`package main

type MyStruct struct {
	Name string
	Age  int
}

type MyInterface interface {
	DoThing() error
}
`)

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	foundStruct := false
	foundInterface := false
	for _, n := range tree.NodeMap {
		if n.Kind == "type_spec" && n.Label == "MyStruct" {
			foundStruct = true
		}
		if n.Kind == "type_spec" && n.Label == "MyInterface" {
			foundInterface = true
		}
	}
	if !foundStruct {
		t.Error("expected to find type_spec 'MyStruct'")
	}
	if !foundInterface {
		t.Error("expected to find type_spec 'MyInterface'")
	}
}

func TestParseGo_IfStatement(t *testing.T) {
	p := NewGo()

	src := []byte(`package main

func check(x int) {
	if x > 0 {
		fmt.Println("positive")
	} else {
		fmt.Println("non-positive")
	}
}
`)

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	foundIf := false
	for _, n := range tree.NodeMap {
		if n.Kind == "if_statement" {
			foundIf = true
			break
		}
	}
	if !foundIf {
		t.Error("expected to find if_statement")
	}
}

func TestParsePython_StructuralParser(t *testing.T) {
	p := NewGeneric("python", []string{".py"})

	src := []byte("def hello():\n    print('hello')\n\ndef world():\n    print('world')\n")

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if tree.Language != "python" {
		t.Errorf("expected language 'python', got %q", tree.Language)
	}
	if len(tree.Root.Children) != 2 {
		t.Errorf("expected 2 function nodes, got %d", len(tree.Root.Children))
	}
	if tree.Root.Children[0].Kind != "function_definition" {
		t.Errorf("expected function_definition, got %q", tree.Root.Children[0].Kind)
	}
	if tree.Root.Children[0].Label != "hello" {
		t.Errorf("expected label 'hello', got %q", tree.Root.Children[0].Label)
	}
	if tree.Root.Children[1].Label != "world" {
		t.Errorf("expected label 'world', got %q", tree.Root.Children[1].Label)
	}
}

func TestParseJavaScript_FunctionsAndClasses(t *testing.T) {
	p := NewGeneric("javascript", []string{".js"})

	src := []byte(`function greet(name) {
  return "Hello, " + name;
}

class Greeter {
  constructor(name) {
    this.name = name;
  }

  sayHello() {
    return greet(this.name);
  }
}

const farewell = (name) => "Goodbye, " + name;
`)

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if tree.Language != "javascript" {
		t.Errorf("expected language 'javascript', got %q", tree.Language)
	}

	var funcNames []string
	for _, child := range tree.Root.Children {
		if child.Kind == "function_declaration" {
			funcNames = append(funcNames, child.Label)
		}
		if child.Kind == "class_declaration" {
			if child.Label != "Greeter" {
				t.Errorf("expected class 'Greeter', got %q", child.Label)
			}
			if len(child.Children) < 2 {
				t.Errorf("expected at least 2 methods in Greeter, got %d", len(child.Children))
			}
		}
	}
	if len(funcNames) < 1 || funcNames[0] != "greet" {
		t.Errorf("expected function 'greet', got %v", funcNames)
	}
}

func TestParseJava_ClassWithMethods(t *testing.T) {
	p := NewGeneric("java", []string{".java"})

	src := []byte(`package com.example;

import java.util.List;

public class Calculator {
    private int value;

    public Calculator(int initial) {
        this.value = initial;
    }

    public int add(int x) {
        value += x;
        return value;
    }

    public int getValue() {
        return value;
    }
}
`)

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if tree.Language != "java" {
		t.Errorf("expected language 'java', got %q", tree.Language)
	}

	foundPackage := false
	foundImport := false
	foundClass := false
	for _, child := range tree.Root.Children {
		if child.Kind == "package_declaration" {
			foundPackage = true
		}
		if child.Kind == "import_declaration" {
			foundImport = true
		}
		if child.Kind == "class_declaration" && child.Label == "Calculator" {
			foundClass = true
			methodCount := 0
			for _, m := range child.Children {
				if m.Kind == "method_declaration" {
					methodCount++
				}
			}
			if methodCount < 3 {
				t.Errorf("expected at least 3 methods (constructor+add+getValue), got %d", methodCount)
			}
		}
	}
	if !foundPackage {
		t.Error("expected package_declaration")
	}
	if !foundImport {
		t.Error("expected import_declaration")
	}
	if !foundClass {
		t.Error("expected class_declaration 'Calculator'")
	}
}

func TestParseGeneric_LineBasedFallback(t *testing.T) {
	p := NewGeneric("lua", []string{".lua"})

	src := []byte("function main()\n    return 0\nend\n\nfunction helper()\n    print(\"hi\")\nend\n")

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if tree.Language != "lua" {
		t.Errorf("expected language 'lua', got %q", tree.Language)
	}
	if len(tree.Root.Children) < 4 {
		t.Errorf("expected at least 4 line nodes, got %d", len(tree.Root.Children))
	}
}

func TestParseC_StructuralParser(t *testing.T) {
	p := NewGeneric("c", []string{".c", ".h"})

	src := []byte("#include <stdio.h>\n#define MAX 100\n\nstruct Point {\n    int x;\n    int y;\n};\n\nenum Color { RED, GREEN, BLUE };\n\nint add(int a, int b) {\n    return a + b;\n}\n\nvoid greet(const char *name) {\n    printf(\"Hello, %s\\n\", name);\n}\n\nint main() {\n    struct Point p = {1, 2};\n    greet(\"World\");\n    return 0;\n}\n")

	tree, err := p.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	if tree.Language != "c" {
		t.Errorf("expected language 'c', got %q", tree.Language)
	}

	kinds := map[string]int{}
	for _, child := range tree.Root.Children {
		kinds[child.Kind]++
	}

	if kinds["include_directive"] < 1 {
		t.Error("expected at least 1 include_directive")
	}
	if kinds["macro_definition"] < 1 {
		t.Error("expected at least 1 macro_definition")
	}
	if kinds["struct_declaration"] < 1 {
		t.Error("expected at least 1 struct_declaration")
	}
	if kinds["enum_declaration"] < 1 {
		t.Error("expected at least 1 enum_declaration")
	}
	if kinds["function_definition"] < 3 {
		t.Errorf("expected at least 3 function_definitions (add, greet, main), got %d", kinds["function_definition"])
	}

	funcNames := map[string]bool{}
	for _, child := range tree.Root.Children {
		if child.Kind == "function_definition" {
			funcNames[child.Label] = true
		}
	}
	for _, name := range []string{"add", "greet", "main"} {
		if !funcNames[name] {
			t.Errorf("expected function_definition with label %q", name)
		}
	}
}

func TestSupportedLanguages(t *testing.T) {
	langs := SupportedLanguages()
	if len(langs) < 7 {
		t.Errorf("expected at least 7 languages, got %d", len(langs))
	}
}
