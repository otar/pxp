package plan

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

const MaxDocumentBytes = 16 << 20

// Read reads, decodes, and validates one PXP v1 document.
func Read(r io.Reader) (*Plan, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	return Decode(data)
}

// Decode decodes and validates one PXP v1 document without executing it.
func Decode(data []byte) (*Plan, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("plan is empty")
	}
	if len(data) > MaxDocumentBytes {
		return nil, fmt.Errorf("plan exceeds the %d-byte limit", MaxDocumentBytes)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("plan is not valid UTF-8")
	}
	parseData := data
	if bytes.HasPrefix(parseData, []byte{0xef, 0xbb, 0xbf}) {
		parseData = parseData[3:]
		if len(parseData) == 0 {
			return nil, fmt.Errorf("plan is empty")
		}
	}

	file, err := parser.ParseBytes(parseData, 0)
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	if len(file.Docs) != 1 {
		return nil, fmt.Errorf("plan must contain exactly one YAML document")
	}
	for _, typ := range []ast.NodeType{
		ast.AnchorType,
		ast.AliasType,
		ast.MergeKeyType,
		ast.TagType,
		ast.DirectiveType,
		ast.NullType,
		ast.BoolType,
		ast.IntegerType,
		ast.FloatType,
		ast.InfinityType,
		ast.NanType,
	} {
		if len(ast.FilterFile(typ, file)) != 0 {
			return nil, fmt.Errorf("YAML %s nodes are not allowed", typ.YAMLName())
		}
	}

	var decoded Plan
	if err := yaml.UnmarshalWithOptions(parseData, &decoded, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	if err := decoded.validate(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	decoded.SHA256 = fmt.Sprintf("%x", digest)
	return &decoded, nil
}
