package treesitter

import (
	"context"
	"testing"
)

func fuzzParser(f *testing.F, lang string, seeds []string) {
	f.Helper()
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		p := &Parser{lang: lang}
		tree, err := p.Parse(context.Background(), src)
		if err != nil {
			return
		}
		if tree == nil {
			t.Fatal("Parse returned nil tree without error")
		}
		if tree.Root == nil {
			t.Fatal("Parse returned tree with nil root")
		}
	})
}

func FuzzParseGo(f *testing.F) {
	fuzzParser(f, "go", []string{
		"package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
		"package p\n\nimport \"fmt\"\n\ntype S struct {\n\tX int\n}\n\nfunc (s S) M() { fmt.Println(s.X) }\n",
		"package p\n\nvar x = 1\nconst y = 2\n",
		"",
		"not valid go at all {{{",
		"package p\n\nfunc f() {\n\tif true {\n\t\tfor i := range 10 {\n\t\t\t_ = i\n\t\t}\n\t}\n}\n",
	})
}

func FuzzParseJavaScript(f *testing.F) {
	fuzzParser(f, "javascript", []string{
		"function hello() {\n  console.log('hi');\n}\n",
		"class Foo extends Bar {\n  constructor() {\n    super();\n  }\n  method() {}\n}\n",
		"const x = () => { return 1; };\nexport default x;\n",
		"",
		"{{{{",
		"import { a } from 'b';\nexport function c() {}\n",
	})
}

func FuzzParseTypeScript(f *testing.F) {
	fuzzParser(f, "typescript", []string{
		"interface Foo {\n  bar: string;\n  baz(x: number): void;\n}\n",
		"function greet(name: string): string {\n  return `Hello ${name}`;\n}\n",
		"export class Service {\n  private val: number;\n  constructor(v: number) { this.val = v; }\n}\n",
		"",
		"type X = { a: 1 } & { b: 2 };",
	})
}

func FuzzParsePython(f *testing.F) {
	fuzzParser(f, "python", []string{
		"def hello():\n    print('hi')\n\nclass Foo:\n    def method(self):\n        pass\n",
		"import os\nfrom sys import argv\n\ndef main():\n    pass\n",
		"",
		"not valid python ::::",
		"class A:\n    class B:\n        def f(self):\n            return 1\n",
	})
}

func FuzzParseJava(f *testing.F) {
	fuzzParser(f, "java", []string{
		"public class Foo {\n    public void bar() {\n        System.out.println(\"hi\");\n    }\n}\n",
		"import java.util.List;\n\npublic class Main {\n    public static void main(String[] args) {}\n}\n",
		"",
		"{{{{",
		"interface Baz {\n    void run();\n}\n",
	})
}

func FuzzParseC(f *testing.F) {
	fuzzParser(f, "c", []string{
		"#include <stdio.h>\n\nint main() {\n    printf(\"hello\\n\");\n    return 0;\n}\n",
		"typedef struct {\n    int x;\n    int y;\n} Point;\n\nvoid print_point(Point p) {}\n",
		"",
		"{{{{",
		"#define MAX 100\n\nstatic int arr[MAX];\n",
	})
}

func FuzzParseCpp(f *testing.F) {
	fuzzParser(f, "cpp", []string{
		"#include <iostream>\n\nclass Foo {\npublic:\n    void bar() {}\n};\n\nint main() { Foo f; f.bar(); }\n",
		"namespace ns {\n    template<typename T>\n    T add(T a, T b) { return a + b; }\n}\n",
		"",
		"{{{{",
	})
}

func FuzzParseGeneric(f *testing.F) {
	fuzzParser(f, "unknown", []string{
		"line one\nline two\nline three\n",
		"",
		"single line no newline",
		"\n\n\n",
		"{\n}\n(\n)\n",
		"a very long line " + string(make([]byte, 10000)),
	})
}
