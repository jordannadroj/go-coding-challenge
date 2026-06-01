package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Schema represents a JSON Schema node. Most fields can appear at any level.
type Schema struct {
	Title      string             `json:"title"`
	Type       SchemaType         `json:"type"`
	Required   []string           `json:"required"`
	Properties map[string]*Schema `json:"properties"`
	Items      *Schema            `json:"items"`
	Enum       []any              `json:"enum"`
	Const      any                `json:"const"`
	Ref        string             `json:"$ref"`
	OneOf      []*Schema          `json:"oneOf"`
	Defs       map[string]*Schema `json:"$defs"`
}

// SchemaType handles both "string" and ["string","null"] forms of the type field.
type SchemaType struct {
	Values []string
}

func (t *SchemaType) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		t.Values = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(data, &multi); err != nil {
		return err
	}
	t.Values = multi
	return nil
}

func loadSchema(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &s, nil
}

func main() {
	schemaDir := "schemas"
	files, _ := filepath.Glob(filepath.Join(schemaDir, "*.json"))
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no schema files found in", schemaDir)
		os.Exit(1)
	}

	schemas := make(map[string]*Schema, len(files))
	for _, f := range files {
		s, err := loadSchema(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		schemas[s.Title] = s
	}

	src, err := buildModel(schemas)
	if err != nil {
		fmt.Fprintln(os.Stderr, "codegen error:", err)
		os.Exit(1)
	}

	if err := os.WriteFile("model/model.go", src, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote model/model.go")
}
