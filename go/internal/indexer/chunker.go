package indexer

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"

	"github.com/netanelshoshan/goindexer/internal/chunk"
	"github.com/netanelshoshan/goindexer/internal/config"
	"github.com/netanelshoshan/goindexer/internal/storage"
	"github.com/netanelshoshan/goindexer/internal/tokenizer"
)

var extToLang = map[string]string{
	".py": "python", ".go": "go", ".js": "javascript", ".ts": "typescript",
	".jsx": "javascript", ".tsx": "typescript", ".rs": "rust",
	".c": "c", ".h": "c",
	".cpp": "cpp", ".cc": "cpp", ".cxx": "cpp", ".hpp": "cpp", ".hxx": "cpp",
	".java": "java",
	".cs":   "csharp",
}

var pythonNodeTypes = map[string]string{
	"function_definition":       "function",
	"async_function_definition": "function",
	"class_definition":          "class",
}

var jsNodeTypes = map[string]string{
	"function_declaration": "function",
	"class_declaration":    "class",
	"method_definition":    "method",
	"arrow_function":       "function",
}

var goNodeTypes = map[string]string{
	"function_declaration": "function",
	"method_declaration":   "method",
}

var rustNodeTypes = map[string]string{
	"function_item": "function",
	"impl_item":     "impl",
	"trait_item":    "trait",
	"struct_item":   "struct",
	"enum_item":     "enum",
}

var cNodeTypes = map[string]string{
	"function_definition": "function",
	"struct_specifier":    "struct",
	"enum_specifier":      "enum",
}

var cppNodeTypes = map[string]string{
	"function_definition": "function",
	"class_specifier":     "class",
	"struct_specifier":    "struct",
}

var javaNodeTypes = map[string]string{
	"class_declaration":     "class",
	"method_declaration":    "method",
	"interface_declaration": "interface",
}

var csharpNodeTypes = map[string]string{
	"class_declaration":     "class",
	"method_declaration":    "method",
	"struct_declaration":    "struct",
	"interface_declaration": "interface",
}

// Go: block children that can be emitted as sub-chunks
var goSplittableStmtTypes = map[string]bool{
	"for_statement":               true,
	"if_statement":                true,
	"expression_switch_statement": true,
	"type_switch_statement":       true,
	"select_statement":            true,
	"defer_statement":             true,
	"go_statement":                true,
	"block":                       true,
}

// Python: block children
var pythonSplittableStmtTypes = map[string]bool{
	"if_statement":              true,
	"for_statement":             true,
	"while_statement":           true,
	"match_statement":           true,
	"with_statement":            true,
	"try_statement":             true,
	"function_definition":       true,
	"async_function_definition": true,
	"class_definition":          true,
}

// JS/TS: statement_block children
var jsSplittableStmtTypes = map[string]bool{
	"if_statement":         true,
	"for_statement":        true,
	"for_in_statement":     true,
	"while_statement":      true,
	"switch_statement":     true,
	"try_statement":        true,
	"function_declaration": true,
	"expression_statement": true,
}

// Rust: block children
var rustSplittableStmtTypes = map[string]bool{
	"if_expression":    true,
	"match_expression": true,
	"loop_expression":  true,
	"for_expression":   true,
	"while_expression": true,
	"block":            true,
}

// C/C++: compound_statement children
var cSplittableStmtTypes = map[string]bool{
	"if_statement":       true,
	"for_statement":      true,
	"while_statement":    true,
	"switch_statement":   true,
	"compound_statement": true,
}

// Java/C#: block children
var javaSplittableStmtTypes = map[string]bool{
	"if_statement":           true,
	"for_statement":          true,
	"enhanced_for_statement": true,
	"while_statement":        true,
	"switch_expression":      true,
	"try_statement":          true,
	"block":                  true,
}

var csharpSplittableStmtTypes = map[string]bool{
	"if_statement":      true,
	"for_statement":     true,
	"foreach_statement": true,
	"while_statement":   true,
	"switch_expression": true,
	"try_statement":     true,
	"block":             true,
}

type chunkContext struct {
	filePath        string
	lang            string
	breadcrumbStack []string
	maxTokens       int
	minTokens       int
	symbols         *[]storage.SymbolEntry
	parentScope     string
}

func ChunkFile(filePath string, content []byte, cfg *config.Config) ([]*chunk.CodeChunk, []storage.SymbolEntry, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.MaxChunkTokens <= 0 {
		cfg.MaxChunkTokens = config.DefaultMaxChunkTokens
	}
	if cfg.MinChunkTokens < 0 {
		cfg.MinChunkTokens = config.DefaultMinChunkTokens
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	lang := extToLang[ext]
	if lang == "" {
		chunks := wholeFileChunk(filePath, content)
		return chunks, nil, nil
	}

	parser := sitter.NewParser()
	defer parser.Close()

	var langObj *sitter.Language
	switch lang {
	case "python":
		langObj = python.GetLanguage()
	case "go":
		langObj = golang.GetLanguage()
	case "javascript", "typescript":
		langObj = javascript.GetLanguage()
	case "rust":
		langObj = rust.GetLanguage()
	case "c":
		langObj = c.GetLanguage()
	case "cpp":
		langObj = cpp.GetLanguage()
	case "java":
		langObj = java.GetLanguage()
	case "csharp":
		langObj = csharp.GetLanguage()
	default:
		chunks := wholeFileChunk(filePath, content)
		return chunks, nil, nil
	}

	parser.SetLanguage(langObj)
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		chunks := wholeFileChunk(filePath, content)
		return chunks, nil, nil
	}
	defer tree.Close()

	var nodeTypes map[string]string
	switch lang {
	case "python":
		nodeTypes = pythonNodeTypes
	case "javascript", "typescript":
		nodeTypes = jsNodeTypes
	case "go":
		nodeTypes = goNodeTypes
	case "rust":
		nodeTypes = rustNodeTypes
	case "c":
		nodeTypes = cNodeTypes
	case "cpp":
		nodeTypes = cppNodeTypes
	case "java":
		nodeTypes = javaNodeTypes
	case "csharp":
		nodeTypes = csharpNodeTypes
	default:
		chunks := wholeFileChunk(filePath, content)
		return chunks, nil, nil
	}

	var chunks []*chunk.CodeChunk
	var symbols []storage.SymbolEntry
	ctx := &chunkContext{
		filePath:        filePath,
		lang:            lang,
		breadcrumbStack: []string{},
		maxTokens:       cfg.MaxChunkTokens,
		minTokens:       cfg.MinChunkTokens,
		symbols:         &symbols,
	}
	collectChunksRecursive(tree.RootNode(), content, nodeTypes, ctx, &chunks)
	if len(chunks) == 0 {
		chunks = wholeFileChunk(filePath, content)
		return chunks, symbols, nil
	}

	chunks = mergeSmallChunks(chunks, ctx.minTokens)
	return chunks, symbols, nil
}

func collectChunksRecursive(node *sitter.Node, content []byte, nodeTypes map[string]string, ctx *chunkContext, out *[]*chunk.CodeChunk) {
	if node == nil || out == nil {
		return
	}
	nodeType := node.Type()
	symType, isTopLevel := nodeTypes[nodeType]

	if isTopLevel {
		symbolName := getSymbolName(node, content)
		text := string(content[node.StartByte():node.EndByte()])
		if strings.TrimSpace(text) == "" {
			goto recurse
		}
		tokCount, err := tokenizer.CountTokens(text)
		if err != nil {
			tokCount = len(text) / 4
		}
		startPoint := node.StartPoint()
		endPoint := node.EndPoint()
		line := int(startPoint.Row) + 1

		breadcrumb := strings.Join(ctx.breadcrumbStack, " | ")
		if breadcrumb != "" {
			breadcrumb = "// " + breadcrumb + "\n"
		}
		fullText := breadcrumb + text

		if tokCount <= ctx.maxTokens {
			c := &chunk.CodeChunk{
				Text:       fullText,
				FilePath:   ctx.filePath,
				SymbolName: symbolName,
				SymbolType: symType,
				StartLine:  int(startPoint.Row) + 1,
				EndLine:    int(endPoint.Row) + 1,
				Language:   ctx.lang,
				Breadcrumb: strings.Join(ctx.breadcrumbStack, " | "),
			}
			*out = append(*out, c)
			ctx.addDef(symbolName, line, ctx.parentScope, c)
			extractCallRefs(node, content, ctx)
			return
		}

		block := findBlockChild(node, content)
		if block != nil {
			scopePart := symbolName
			if ctx.parentScope != "" {
				scopePart = ctx.parentScope + "." + symbolName
			}
			prevScope := ctx.parentScope
			ctx.breadcrumbStack = append(ctx.breadcrumbStack, symType+": "+symbolName)
			ctx.parentScope = scopePart
			collectBlockChunks(block, content, nodeTypes, ctx, out)
			ctx.breadcrumbStack = ctx.breadcrumbStack[:len(ctx.breadcrumbStack)-1]
			ctx.parentScope = prevScope
			ctx.addDef(symbolName, line, ctx.parentScope, nil)
			extractCallRefs(node, content, ctx)
			return
		}
		ctx.addDef(symbolName, line, ctx.parentScope, nil)
		extractCallRefs(node, content, ctx)
	}

recurse:
	for i := 0; i < int(node.ChildCount()); i++ {
		collectChunksRecursive(node.Child(i), content, nodeTypes, ctx, out)
	}
}

func (ctx *chunkContext) addDef(symbolName string, line int, parentScope string, c *chunk.CodeChunk) {
	chunkID := ""
	if c != nil {
		chunkID = c.ID()
	}
	*ctx.symbols = append(*ctx.symbols, storage.SymbolEntry{
		SymbolName:  symbolName,
		FilePath:    ctx.filePath,
		LineNumber:  line,
		Type:        "def",
		ParentScope: parentScope,
		ChunkID:     chunkID,
	})
}

func (ctx *chunkContext) addRef(symbolName string, line int, chunkID string) {
	*ctx.symbols = append(*ctx.symbols, storage.SymbolEntry{
		SymbolName:  symbolName,
		FilePath:    ctx.filePath,
		LineNumber:  line,
		Type:        "ref",
		ParentScope: ctx.parentScope,
		ChunkID:     chunkID,
	})
}

func findBlockChild(node *sitter.Node, content []byte) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		ch := node.Child(i)
		typ := ch.Type()
		if typ == "block" || typ == "block_node" || typ == "suite" || typ == "compound_statement" {
			return ch
		}
	}
	return nil
}

func collectBlockChunks(block *sitter.Node, content []byte, nodeTypes map[string]string, ctx *chunkContext, out *[]*chunk.CodeChunk) {
	var splittable map[string]bool
	switch ctx.lang {
	case "go":
		splittable = goSplittableStmtTypes
	case "python":
		splittable = pythonSplittableStmtTypes
	case "javascript", "typescript":
		splittable = jsSplittableStmtTypes
	case "rust":
		splittable = rustSplittableStmtTypes
	case "c", "cpp":
		splittable = cSplittableStmtTypes
	case "java":
		splittable = javaSplittableStmtTypes
	case "csharp":
		splittable = csharpSplittableStmtTypes
	default:
		return
	}

	var acc []*sitter.Node
	flushAcc := func() {
		if len(acc) == 0 {
			return
		}
		start := acc[0].StartByte()
		end := acc[len(acc)-1].EndByte()
		text := string(content[start:end])
		tokCount, _ := tokenizer.CountTokens(text)
		if tokCount < 1 {
			acc = nil
			return
		}
		breadcrumb := strings.Join(ctx.breadcrumbStack, " | ")
		if breadcrumb != "" {
			breadcrumb = "// " + breadcrumb + "\n"
		}
		fullText := breadcrumb + text
		c := &chunk.CodeChunk{
			Text:       fullText,
			FilePath:   ctx.filePath,
			SymbolName: "",
			SymbolType: "block",
			StartLine:  int(acc[0].StartPoint().Row) + 1,
			EndLine:    int(acc[len(acc)-1].EndPoint().Row) + 1,
			Language:   ctx.lang,
			Breadcrumb: strings.Join(ctx.breadcrumbStack, " | "),
		}
		if out != nil {
			*out = append(*out, c)
		}
		extractCallRefsFromNodes(acc, content, ctx, c.ID())
		acc = nil
	}

	for i := 0; i < int(block.ChildCount()); i++ {
		ch := block.Child(i)
		typ := ch.Type()
		if splittable[typ] {
			flushAcc()
			text := string(content[ch.StartByte():ch.EndByte()])
			tokCount, _ := tokenizer.CountTokens(text)
			if tokCount > ctx.maxTokens && (typ == "block" || typ == "compound_statement") {
				collectBlockChunks(ch, content, nodeTypes, ctx, out)
			} else {
				breadcrumb := strings.Join(ctx.breadcrumbStack, " | ")
				if breadcrumb != "" {
					breadcrumb = "// " + breadcrumb + "\n"
				}
				fullText := breadcrumb + text
				c := &chunk.CodeChunk{
					Text:       fullText,
					FilePath:   ctx.filePath,
					SymbolName: "",
					SymbolType: typ,
					StartLine:  int(ch.StartPoint().Row) + 1,
					EndLine:    int(ch.EndPoint().Row) + 1,
					Language:   ctx.lang,
					Breadcrumb: strings.Join(ctx.breadcrumbStack, " | "),
				}
				if out != nil {
					*out = append(*out, c)
				}
				extractCallRefs(ch, content, ctx)
			}
		} else {
			acc = append(acc, ch)
		}
	}
	flushAcc()
}

func extractCallRefs(node *sitter.Node, content []byte, ctx *chunkContext) {
	extractCallRefsFromNodes([]*sitter.Node{node}, content, ctx, "")
}

func extractCallRefsFromNodes(nodes []*sitter.Node, content []byte, ctx *chunkContext, chunkID string) {
	callTypes := map[string]bool{
		"call_expression":       true,
		"call":                  true,
		"method_invocation":     true, // Java
		"invocation_expression": true, // C#
	}
	for _, n := range nodes {
		walkNodes(n, func(nd *sitter.Node) bool {
			if !callTypes[nd.Type()] {
				return true
			}
			name := getCalleeName(nd, content, ctx.lang)
			if name != "" {
				line := int(nd.StartPoint().Row) + 1
				ctx.addRef(name, line, chunkID)
			}
			return true
		})
	}
}

func walkNodes(node *sitter.Node, f func(*sitter.Node) bool) {
	if node == nil {
		return
	}
	if !f(node) {
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkNodes(node.Child(i), f)
	}
}

func getCalleeName(node *sitter.Node, content []byte, lang string) string {
	nodeType := node.Type()
	for i := 0; i < int(node.ChildCount()); i++ {
		ch := node.Child(i)
		typ := ch.Type()
		switch typ {
		case "function", "callee":
			return getCalleeNameFromNode(ch, content)
		case "identifier", "name":
			return strings.TrimSpace(string(content[ch.StartByte():ch.EndByte()]))
		case "selector_expression", "attribute":
			return getSelectorCallee(ch, content)
		case "member_access_expression", "field_access":
			return getSelectorCallee(ch, content)
		case "scoped_identifier":
			return getScopedIdentifierCallee(ch, content)
		}
	}
	if nodeType == "method_invocation" {
		return getMethodInvocationCallee(node, content)
	}
	return ""
}

func getMethodInvocationCallee(node *sitter.Node, content []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		ch := node.Child(i)
		if ch.Type() == "identifier" {
			return strings.TrimSpace(string(content[ch.StartByte():ch.EndByte()]))
		}
		if ch.Type() == "field_expression" {
			return getSelectorCallee(ch, content)
		}
	}
	return ""
}

func getScopedIdentifierCallee(node *sitter.Node, content []byte) string {
	nc := int(node.ChildCount())
	if nc == 0 {
		return ""
	}
	last := node.Child(nc - 1)
	if last.Type() == "identifier" || last.Type() == "template_function" {
		return strings.TrimSpace(string(content[last.StartByte():last.EndByte()]))
	}
	return ""
}

func getCalleeNameFromNode(node *sitter.Node, content []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		ch := node.Child(i)
		typ := ch.Type()
		if typ == "identifier" || typ == "name" || typ == "type_identifier" {
			return strings.TrimSpace(string(content[ch.StartByte():ch.EndByte()]))
		}
		if typ == "selector_expression" || typ == "attribute" {
			return getSelectorCallee(ch, content)
		}
	}
	return ""
}

func getSelectorCallee(node *sitter.Node, content []byte) string {
	nc := int(node.ChildCount())
	if nc < 2 {
		return ""
	}
	last := node.Child(nc - 1)
	typ := last.Type()
	if typ == "identifier" || typ == "property_identifier" || typ == "name" {
		return strings.TrimSpace(string(content[last.StartByte():last.EndByte()]))
	}
	return ""
}

func mergeSmallChunks(chunks []*chunk.CodeChunk, minTokens int) []*chunk.CodeChunk {
	if minTokens <= 0 || len(chunks) <= 1 {
		return chunks
	}
	var out []*chunk.CodeChunk
	i := 0
	for i < len(chunks) {
		c := chunks[i]
		text := c.Text
		tokCount, _ := tokenizer.CountTokens(text)
		j := i + 1
		for j < len(chunks) && tokCount < minTokens {
			next := chunks[j]
			nextTok, _ := tokenizer.CountTokens(next.Text)
			text = text + "\n\n" + next.Text
			tokCount += nextTok
			j++
		}
		if j > i+1 {
			c = &chunk.CodeChunk{
				Text:       text,
				FilePath:   c.FilePath,
				SymbolName: c.SymbolName,
				SymbolType: c.SymbolType,
				StartLine:  c.StartLine,
				EndLine:    chunks[j-1].EndLine,
				Language:   c.Language,
				Breadcrumb: c.Breadcrumb,
			}
		}
		out = append(out, c)
		i = j
	}
	return out
}

func getSymbolName(node *sitter.Node, content []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		typ := child.Type()
		if typ == "identifier" || typ == "name" || typ == "type_identifier" {
			return strings.TrimSpace(string(content[child.StartByte():child.EndByte()]))
		}
		if typ == "property_identifier" {
			return strings.TrimSpace(string(content[child.StartByte():child.EndByte()]))
		}
	}
	return getSymbolNameRecurse(node, content)
}

func getSymbolNameRecurse(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		typ := child.Type()
		if typ == "identifier" || typ == "name" || typ == "type_identifier" || typ == "property_identifier" {
			return strings.TrimSpace(string(content[child.StartByte():child.EndByte()]))
		}
		if s := getSymbolNameRecurse(child, content); s != "" {
			return s
		}
	}
	return ""
}

func wholeFileChunk(filePath string, content []byte) []*chunk.CodeChunk {
	text := string(content)
	lines := strings.Count(text, "\n") + 1
	if lines < 1 {
		lines = 1
	}
	return []*chunk.CodeChunk{{
		Text:       text,
		FilePath:   filePath,
		SymbolName: "",
		SymbolType: "file",
		StartLine:  1,
		EndLine:    lines,
		Language:   "unknown",
	}}
}
