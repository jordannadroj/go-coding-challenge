package main

import (
	"strings"
	"testing"
)

func TestGoName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"id", "ID"},
		{"url", "URL"},
		{"name", "Name"},
		{"wordCount", "WordCount"},
		{"readingTime", "ReadingTime"},
		{"basic-science", "BasicScience"},
		{"word_count", "WordCount"},
		{"htmlContent", "HTMLContent"},
		{"apiKey", "APIKey"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := goName(tt.input)
			if got != tt.want {
				t.Errorf("goName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRefTypeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"#/$defs/RichTextNode", "RichTextNode"},
		{"#/$defs/PlainText", "PlainText"},
		{"Category.json", "Category"},
		{"schemas/Category.json", "Category"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := refTypeName(tt.input)
			if got != tt.want {
				t.Errorf("refTypeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsNullable(t *testing.T) {
	tests := []struct {
		values []string
		want   bool
	}{
		{[]string{"string"}, false},
		{[]string{"string", "null"}, true},
		{[]string{"null"}, true},
		{[]string{}, false},
	}
	for _, tt := range tests {
		s := &Schema{Type: SchemaType{Values: tt.values}}
		got := isNullable(s)
		if got != tt.want {
			t.Errorf("isNullable(%v) = %v, want %v", tt.values, got, tt.want)
		}
	}
}

func TestPrimaryType(t *testing.T) {
	tests := []struct {
		values []string
		want   string
	}{
		{[]string{"string"}, "string"},
		{[]string{"string", "null"}, "string"},
		{[]string{"null", "object"}, "object"},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		s := &Schema{Type: SchemaType{Values: tt.values}}
		got := primaryType(s)
		if got != tt.want {
			t.Errorf("primaryType(%v) = %q, want %q", tt.values, got, tt.want)
		}
	}
}

func TestGoFieldType(t *testing.T) {
	tests := []struct {
		name       string
		fieldName  string
		schema     *Schema
		parentType string
		want       string
	}{
		{
			name:       "$ref cross-file",
			fieldName:  "category",
			schema:     &Schema{Ref: "Category.json"},
			parentType: "Article",
			want:       "*Category",
		},
		{
			name:      "oneOf becomes union wrapper",
			fieldName: "content",
			schema: &Schema{OneOf: []*Schema{
				{Ref: "#/$defs/RichText"},
				{Ref: "#/$defs/PlainText"},
			}},
			parentType: "Article",
			want:       "*ArticleContent",
		},
		{
			name:      "enum non-nullable",
			fieldName: "classification",
			schema: &Schema{
				Type: SchemaType{Values: []string{"string"}},
				Enum: []any{"basic_science", "applied"},
			},
			parentType: "Product",
			want:       "ProductClassification",
		},
		{
			name:      "enum nullable",
			fieldName: "kind",
			schema: &Schema{
				Type: SchemaType{Values: []string{"string", "null"}},
				Enum: []any{"basic-science", "clinical"},
			},
			parentType: "Category",
			want:       "*CategoryKind",
		},
		{
			name:      "array with $ref items",
			fieldName: "nodes",
			schema: &Schema{
				Type:  SchemaType{Values: []string{"array"}},
				Items: &Schema{Ref: "#/$defs/RichTextNode"},
			},
			parentType: "RichText",
			want:       "[]RichTextNode",
		},
		{
			name:      "array with primitive items",
			fieldName: "authors",
			schema: &Schema{
				Type:  SchemaType{Values: []string{"array"}},
				Items: &Schema{Type: SchemaType{Values: []string{"string"}}},
			},
			parentType: "Article",
			want:       "[]string",
		},
		{
			name:       "array without items",
			fieldName:  "tags",
			schema:     &Schema{Type: SchemaType{Values: []string{"array"}}},
			parentType: "Product",
			want:       "[]any",
		},
		{
			name:      "nested object",
			fieldName: "dimensions",
			schema: &Schema{
				Type: SchemaType{Values: []string{"object"}},
				Properties: map[string]*Schema{
					"width": {Type: SchemaType{Values: []string{"integer"}}},
				},
			},
			parentType: "Product",
			want:       "*ProductDimensions",
		},
		{
			name:       "string scalar",
			fieldName:  "name",
			schema:     &Schema{Type: SchemaType{Values: []string{"string"}}},
			parentType: "Product",
			want:       "string",
		},
		{
			name:       "nullable string",
			fieldName:  "description",
			schema:     &Schema{Type: SchemaType{Values: []string{"string", "null"}}},
			parentType: "Product",
			want:       "*string",
		},
		{
			name:       "integer",
			fieldName:  "quantity",
			schema:     &Schema{Type: SchemaType{Values: []string{"integer"}}},
			parentType: "Product",
			want:       "int",
		},
		{
			name:       "boolean",
			fieldName:  "active",
			schema:     &Schema{Type: SchemaType{Values: []string{"boolean"}}},
			parentType: "Product",
			want:       "bool",
		},
		{
			name:       "number",
			fieldName:  "price",
			schema:     &Schema{Type: SchemaType{Values: []string{"number"}}},
			parentType: "Product",
			want:       "float64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goFieldType(tt.fieldName, tt.schema, tt.parentType)
			if got != tt.want {
				t.Errorf("goFieldType(%q, ..., %q) = %q, want %q", tt.fieldName, tt.parentType, got, tt.want)
			}
		})
	}
}

func TestJsonTag(t *testing.T) {
	tests := []struct {
		propName string
		required bool
		want     string
	}{
		{"name", true, "`json:\"name\"`"},
		{"name", false, "`json:\"name,omitempty\"`"},
		{"word-count", true, "`json:\"word-count\"`"},
		{"word-count", false, "`json:\"word-count,omitempty\"`"},
	}
	for _, tt := range tests {
		got := jsonTag(tt.propName, tt.required)
		if got != tt.want {
			t.Errorf("jsonTag(%q, %v) = %q, want %q", tt.propName, tt.required, got, tt.want)
		}
	}
}

func TestFindConst(t *testing.T) {
	t.Run("has const", func(t *testing.T) {
		s := &Schema{
			Properties: map[string]*Schema{
				"format": {Const: "richtext"},
			},
		}
		field, val := findConst(s)
		if field != "format" || val != "richtext" {
			t.Errorf("findConst() = (%q, %q), want (\"format\", \"richtext\")", field, val)
		}
	})

	t.Run("no const", func(t *testing.T) {
		s := &Schema{
			Properties: map[string]*Schema{
				"text": {Type: SchemaType{Values: []string{"string"}}},
			},
		}
		field, val := findConst(s)
		if field != "" || val != "" {
			t.Errorf("findConst() = (%q, %q), want (\"\", \"\")", field, val)
		}
	})
}

// buildModel helpers — construct schemas in-memory to verify generator output.

func TestBuildModel_SimpleStruct(t *testing.T) {
	schemas := map[string]*Schema{
		"Category": {
			Title:    "Category",
			Type:     SchemaType{Values: []string{"object"}},
			Required: []string{"id"},
			Properties: map[string]*Schema{
				"id":   {Type: SchemaType{Values: []string{"string"}}},
				"name": {Type: SchemaType{Values: []string{"string"}}},
				"kind": {
					Type: SchemaType{Values: []string{"string"}},
					Enum: []any{"basic-science", "clinical", "other"},
				},
				"parent": {Type: SchemaType{Values: []string{"string", "null"}}},
			},
		},
	}

	src, err := buildModel(schemas)
	if err != nil {
		t.Fatalf("buildModel returned error: %v", err)
	}
	out := string(src)

	for _, want := range []string{
		"type Category struct",
		"type CategoryKind string",
		"CategoryKindBasicScience",
		`json:"id"`,         // required — no omitempty
		`json:"kind,omitempty"`, // optional — has omitempty
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestBuildModel_OneOf(t *testing.T) {
	schemas := map[string]*Schema{
		"Article": {
			Title:    "Article",
			Type:     SchemaType{Values: []string{"object"}},
			Required: []string{"id"},
			Properties: map[string]*Schema{
				"id": {Type: SchemaType{Values: []string{"string"}}},
				"content": {
					OneOf: []*Schema{
						{Ref: "#/$defs/RichText"},
						{Ref: "#/$defs/PlainText"},
					},
				},
			},
			Defs: map[string]*Schema{
				"RichText": {
					Type: SchemaType{Values: []string{"object"}},
					Properties: map[string]*Schema{
						"format": {Type: SchemaType{Values: []string{"string"}}, Const: "richtext"},
						"html":   {Type: SchemaType{Values: []string{"string"}}},
					},
				},
				"PlainText": {
					Type: SchemaType{Values: []string{"object"}},
					Properties: map[string]*Schema{
						"format": {Type: SchemaType{Values: []string{"string"}}, Const: "plaintext"},
						"text":   {Type: SchemaType{Values: []string{"string"}}},
					},
				},
			},
		},
	}

	src, err := buildModel(schemas)
	if err != nil {
		t.Fatalf("buildModel returned error: %v", err)
	}
	out := string(src)

	for _, want := range []string{
		`"encoding/json"`,
		"type ArticleContent struct",
		"func (u *ArticleContent) MarshalJSON",
		"func (u *ArticleContent) UnmarshalJSON",
		`case "richtext"`,
		`case "plaintext"`,
		"type Article struct",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestBuildModel_RequiredVsOptional(t *testing.T) {
	schemas := map[string]*Schema{
		"Product": {
			Title:    "Product",
			Type:     SchemaType{Values: []string{"object"}},
			Required: []string{"id", "name"},
			Properties: map[string]*Schema{
				"id":          {Type: SchemaType{Values: []string{"string"}}},
				"name":        {Type: SchemaType{Values: []string{"string"}}},
				"description": {Type: SchemaType{Values: []string{"string", "null"}}},
			},
		},
	}

	src, err := buildModel(schemas)
	if err != nil {
		t.Fatalf("buildModel returned error: %v", err)
	}
	out := string(src)

	if strings.Contains(out, `"id,omitempty"`) {
		t.Error("required field 'id' must not have omitempty")
	}
	if strings.Contains(out, `"name,omitempty"`) {
		t.Error("required field 'name' must not have omitempty")
	}
	if !strings.Contains(out, `"description,omitempty"`) {
		t.Error("optional field 'description' must have omitempty")
	}
}

func TestBuildModel_ConstFieldNoOmitempty(t *testing.T) {
	// const fields are discriminators — they must always be present in JSON output.
	schemas := map[string]*Schema{
		"Article": {
			Title:    "Article",
			Type:     SchemaType{Values: []string{"object"}},
			Required: []string{"id"},
			Properties: map[string]*Schema{
				"id": {Type: SchemaType{Values: []string{"string"}}},
				"content": {
					OneOf: []*Schema{{Ref: "#/$defs/PlainText"}},
				},
			},
			Defs: map[string]*Schema{
				"PlainText": {
					Type: SchemaType{Values: []string{"object"}},
					Properties: map[string]*Schema{
						"format": {Type: SchemaType{Values: []string{"string"}}, Const: "plaintext"},
						"text":   {Type: SchemaType{Values: []string{"string"}}},
					},
				},
			},
		},
	}

	src, err := buildModel(schemas)
	if err != nil {
		t.Fatalf("buildModel returned error: %v", err)
	}
	if strings.Contains(string(src), `"format,omitempty"`) {
		t.Error("const field 'format' must not have omitempty")
	}
}

func TestBuildModel_OutputIsValidGo(t *testing.T) {
	// format.Source (called inside buildModel) errors on invalid Go syntax,
	// so a nil error is proof the output is syntactically valid.
	schemas := map[string]*Schema{
		"Category": {
			Title: "Category",
			Type:  SchemaType{Values: []string{"object"}},
			Properties: map[string]*Schema{
				"id":   {Type: SchemaType{Values: []string{"string"}}},
				"name": {Type: SchemaType{Values: []string{"string"}}},
			},
		},
	}
	_, err := buildModel(schemas)
	if err != nil {
		t.Fatalf("buildModel produced invalid Go: %v", err)
	}
}
