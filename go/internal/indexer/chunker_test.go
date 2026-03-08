package indexer

import (
	"strings"
	"testing"

	"github.com/netanelshoshan/goindexer/internal/config"
)

func TestChunkFile_Python(t *testing.T) {
	code := `
def hello():
    print("world")

class Foo:
    def bar(self):
        pass
`
	cfg := &config.Config{MinChunkTokens: 1}
	chunks, _, err := ChunkFile("test.py", []byte(code), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks (function + class), got %d", len(chunks))
	}
	var foundFunc, foundClass bool
	for _, c := range chunks {
		if c.SymbolName == "hello" && c.SymbolType == "function" {
			foundFunc = true
		}
		if c.SymbolName == "Foo" && c.SymbolType == "class" {
			foundClass = true
		}
	}
	if !foundFunc {
		t.Error("expected to find function 'hello'")
	}
	if !foundClass {
		t.Error("expected to find class 'Foo'")
	}
}

func TestChunkFile_Go(t *testing.T) {
	code := `
package main

func main() {
	println("hi")
}

func helper() int {
	return 42
}
`
	cfg := &config.Config{MinChunkTokens: 1}
	chunks, _, err := ChunkFile("main.go", []byte(code), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestChunkFile_WholeFileFallback(t *testing.T) {
	code := "just some text\nno valid syntax"
	chunks, _, err := ChunkFile("unknown.xyz", []byte(code), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 whole-file chunk, got %d", len(chunks))
	}
	if chunks[0].SymbolType != "file" {
		t.Errorf("expected symbol_type=file, got %s", chunks[0].SymbolType)
	}
	if chunks[0].Language != "unknown" {
		t.Errorf("expected language=unknown, got %s", chunks[0].Language)
	}
}

func TestChunkFile_SwiftFallback(t *testing.T) {
	code := `import Swift
func foo() { }
`
	chunks, _, err := ChunkFile("App.swift", []byte(code), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Swift not in tree-sitter, falls back to whole-file
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk (whole-file fallback), got %d", len(chunks))
	}
}

func TestChunkFile_C(t *testing.T) {
	code := `
#include <stdio.h>

int main() {
    printf("hello");
    return 0;
}

void helper() {
    int x = 42;
}
`
	cfg := &config.Config{MinChunkTokens: 1}
	chunks, symbols, err := ChunkFile("main.c", []byte(code), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks (main + helper), got %d", len(chunks))
	}
	var hasMain, hasHelper bool
	for _, s := range symbols {
		if s.Type == "def" && s.SymbolName == "main" {
			hasMain = true
		}
		if s.Type == "def" && s.SymbolName == "helper" {
			hasHelper = true
		}
	}
	if !hasMain {
		t.Error("expected symbol def for 'main'")
	}
	if !hasHelper {
		t.Error("expected symbol def for 'helper'")
	}
}

func TestChunkFile_Java(t *testing.T) {
	code := `
public class Foo {
    public void bar() {
        System.out.println("hi");
    }
}
`
	cfg := &config.Config{MinChunkTokens: 1}
	chunks, symbols, err := ChunkFile("Foo.java", []byte(code), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 1 {
		t.Errorf("expected at least 1 chunk, got %d", len(chunks))
	}
	var hasClass bool
	for _, s := range symbols {
		if s.Type == "def" && (s.SymbolName == "Foo" || s.SymbolName == "bar") {
			hasClass = true
			break
		}
	}
	if !hasClass {
		t.Error("expected symbol def for class or method")
	}
}

func TestChunkFile_CSharp(t *testing.T) {
	code := `
public class Bar {
    public void DoSomething() {
        Console.WriteLine("hi");
    }
}
`
	cfg := &config.Config{MinChunkTokens: 1}
	chunks, symbols, err := ChunkFile("Bar.cs", []byte(code), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 1 {
		t.Errorf("expected at least 1 chunk, got %d", len(chunks))
	}
	var hasDef bool
	for _, s := range symbols {
		if s.Type == "def" {
			hasDef = true
			break
		}
	}
	if !hasDef {
		t.Error("expected at least one symbol def")
	}
}

func TestChunkID(t *testing.T) {
	cfg := &config.Config{MinChunkTokens: 1}
	chunks, symbols, _ := ChunkFile("a.go", []byte("package p\nfunc f(){}"), cfg)
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	// Symbol extraction: should have at least one def
	var hasDef bool
	for _, s := range symbols {
		if s.Type == "def" && s.SymbolName == "f" {
			hasDef = true
			break
		}
	}
	if !hasDef {
		t.Error("expected symbol def for 'f'")
	}
	id := chunks[0].ID()
	if id == "" {
		t.Error("chunk ID should not be empty")
	}
	// ID format: filepath:start_line:end_line
	if chunks[0].FilePath != "" && !strings.Contains(id, ":") {
		t.Errorf("chunk ID should contain colons: %s", id)
	}
}
