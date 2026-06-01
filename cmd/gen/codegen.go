package main

import (
	"fmt"
	"go/format"
	"sort"
	"strings"
	"unicode"
)

var goInitialisms = map[string]string{
	"ID":   "ID",
	"URL":  "URL",
	"URI":  "URI",
	"API":  "API",
	"HTML": "HTML",
	"JSON": "JSON",
	"XML":  "XML",
	"HTTP": "HTTP",
}

// goName converts a JSON property name to an exported Go identifier.
// Examples: "id" → "ID", "wordCount" → "WordCount", "basic-science" → "BasicScience"
func goName(name string) string {
	parts := splitIdent(name)
	var b strings.Builder
	for _, p := range parts {
		up := strings.ToUpper(p)
		if initialism, ok := goInitialisms[up]; ok {
			b.WriteString(initialism)
		} else if len(p) > 0 {
			b.WriteString(strings.ToUpper(p[:1]))
			b.WriteString(p[1:])
		}
	}
	return b.String()
}

// splitIdent splits on hyphens, underscores, and camelCase boundaries.
func splitIdent(s string) []string {
	var parts []string
	var cur strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if r == '_' || r == '-' {
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		} else if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			parts = append(parts, cur.String())
			cur.Reset()
			cur.WriteRune(r)
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func isNullable(s *Schema) bool {
	for _, t := range s.Type.Values {
		if t == "null" {
			return true
		}
	}
	return false
}

func primaryType(s *Schema) string {
	for _, t := range s.Type.Values {
		if t != "null" {
			return t
		}
	}
	return ""
}

func goScalarType(jsonType string) string {
	switch jsonType {
	case "string":
		return "string"
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	default:
		return "any"
	}
}

// refTypeName extracts a Go type name from a $ref string.
// "#/$defs/RichTextNode" → "RichTextNode", "Category.json" → "Category"
func refTypeName(ref string) string {
	if strings.HasPrefix(ref, "#/$defs/") {
		return strings.TrimPrefix(ref, "#/$defs/")
	}
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	return strings.TrimSuffix(ref, ".json")
}

// findConst finds the first property with a const value and returns (jsonFieldName, constValue).
// Used to locate the discriminator field in oneOf variants.
func findConst(s *Schema) (field, val string) {
	for name, prop := range s.Properties {
		if prop.Const != nil {
			if v, ok := prop.Const.(string); ok {
				return name, v
			}
		}
	}
	return "", ""
}

// goFieldType returns the Go type string for a property.
// parentTypeName is used to build derived names (e.g. "Article" + "Content" → "ArticleContent").
func goFieldType(fieldName string, s *Schema, parentTypeName string) string {
	// $ref → pointer to the referenced type
	if s.Ref != "" {
		return "*" + refTypeName(s.Ref)
	}

	// oneOf → pointer to the generated union wrapper
	if len(s.OneOf) > 0 {
		return "*" + parentTypeName + goName(fieldName)
	}

	nullable := isNullable(s)
	prim := primaryType(s)

	// Enum → named type
	if len(s.Enum) > 0 && prim == "string" {
		t := parentTypeName + goName(fieldName)
		if nullable {
			return "*" + t
		}
		return t
	}

	// Array — items may be a $ref or a primitive
	if prim == "array" {
		if s.Items != nil {
			if s.Items.Ref != "" {
				return "[]" + refTypeName(s.Items.Ref)
			}
			return "[]" + goScalarType(primaryType(s.Items))
		}
		return "[]any"
	}

	// Nested object → pointer to named sub-struct
	if prim == "object" && len(s.Properties) > 0 {
		return "*" + parentTypeName + goName(fieldName)
	}

	// Scalars
	switch prim {
	case "string", "integer", "number", "boolean":
		t := goScalarType(prim)
		if nullable {
			return "*" + t
		}
		return t
	}

	return "any"
}

func jsonTag(propName string, required bool) string {
	if required {
		return "`json:\"" + propName + "\"`"
	}
	return "`json:\"" + propName + ",omitempty\"`"
}

func sortedKeys(m map[string]*Schema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// generateOneOf generates a union struct and its UnmarshalJSON method.
// It peeks at the discriminator field (the property with a const value) to decide
// which concrete type to unmarshal into.
func generateOneOf(typeName string, oneOf []*Schema, defs map[string]*Schema) string {
	type variant struct {
		fieldName string
		typeName  string
		constVal  string
	}

	var variants []variant
	discField := ""

	for _, entry := range oneOf {
		if entry.Ref == "" {
			continue
		}
		name := refTypeName(entry.Ref)
		v := variant{fieldName: name, typeName: name}
		if def, ok := defs[name]; ok {
			f, val := findConst(def)
			if discField == "" && f != "" {
				discField = f
			}
			v.constVal = val
		}
		variants = append(variants, v)
	}

	if discField == "" {
		discField = "type"
	}
	discGoName := goName(discField)

	var b strings.Builder

	fmt.Fprintf(&b, "type %s struct {\n", typeName)
	for _, v := range variants {
		// json:"-" because marshal/unmarshal are handled entirely by the custom methods below.
		fmt.Fprintf(&b, "\t%s *%s `json:\"-\"`\n", v.fieldName, v.typeName)
	}
	b.WriteString("}\n\n")

	// MarshalJSON delegates to whichever variant is set.
	fmt.Fprintf(&b, "func (u *%s) MarshalJSON() ([]byte, error) {\n", typeName)
	for _, v := range variants {
		fmt.Fprintf(&b, "\tif u.%s != nil {\n", v.fieldName)
		fmt.Fprintf(&b, "\t\treturn json.Marshal(u.%s)\n", v.fieldName)
		b.WriteString("\t}\n")
	}
	b.WriteString("\treturn []byte(\"null\"), nil\n")
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "func (u *%s) UnmarshalJSON(data []byte) error {\n", typeName)
	b.WriteString("\tvar probe struct {\n")
	fmt.Fprintf(&b, "\t\t%s string `json:%q`\n", discGoName, discField)
	b.WriteString("\t}\n")
	b.WriteString("\tif err := json.Unmarshal(data, &probe); err != nil {\n")
	b.WriteString("\t\treturn err\n")
	b.WriteString("\t}\n")
	fmt.Fprintf(&b, "\tswitch probe.%s {\n", discGoName)
	for _, v := range variants {
		fmt.Fprintf(&b, "\tcase %q:\n", v.constVal)
		fmt.Fprintf(&b, "\t\tu.%s = new(%s)\n", v.fieldName, v.typeName)
		fmt.Fprintf(&b, "\t\treturn json.Unmarshal(data, u.%s)\n", v.fieldName)
	}
	b.WriteString("\tdefault:\n")
	fmt.Fprintf(&b, "\t\treturn fmt.Errorf(\"unknown %s: %%q\", probe.%s)\n", discField, discGoName)
	b.WriteString("\t}\n")
	b.WriteString("}\n\n")

	return b.String()
}

// generateObject generates all Go source for an object schema and any nested types it uses.
// defs is the $defs map from the root schema, needed to resolve oneOf variants.
// Sub-types (enums, nested structs, union wrappers) are emitted before the struct itself.
func generateObject(typeName string, s *Schema, defs map[string]*Schema) string {
	var b strings.Builder
	required := toSet(s.Required)

	// First pass: emit supporting types so they are declared before the struct that uses them.
	for _, propName := range sortedKeys(s.Properties) {
		prop := s.Properties[propName]
		prim := primaryType(prop)

		if len(prop.Enum) > 0 && prim == "string" {
			enumTypeName := typeName + goName(propName)
			fmt.Fprintf(&b, "type %s string\n\n", enumTypeName)
			b.WriteString("const (\n")
			for _, v := range prop.Enum {
				val := fmt.Sprintf("%v", v)
				fmt.Fprintf(&b, "\t%s%s %s = %q\n", enumTypeName, goName(val), enumTypeName, val)
			}
			b.WriteString(")\n\n")
		}

		if prim == "object" && len(prop.Properties) > 0 {
			b.WriteString(generateObject(typeName+goName(propName), prop, defs))
		}

		if len(prop.OneOf) > 0 && defs != nil {
			b.WriteString(generateOneOf(typeName+goName(propName), prop.OneOf, defs))
		}
	}

	// Second pass: the struct itself.
	fmt.Fprintf(&b, "type %s struct {\n", typeName)
	for _, propName := range sortedKeys(s.Properties) {
		prop := s.Properties[propName]
		// Treat const fields as required: the value is always fixed, never absent.
		isRequired := required[propName] || prop.Const != nil
		fmt.Fprintf(&b, "\t%s %s %s\n",
			goName(propName),
			goFieldType(propName, prop, typeName),
			jsonTag(propName, isRequired),
		)
	}
	b.WriteString("}\n\n")

	return b.String()
}

// hasOneOf reports whether any property in s uses oneOf.
func hasOneOf(s *Schema) bool {
	for _, prop := range s.Properties {
		if len(prop.OneOf) > 0 {
			return true
		}
	}
	return false
}

// buildModel assembles the full model/model.go source and formats it with gofmt.
func buildModel(schemas map[string]*Schema) ([]byte, error) {
	var types strings.Builder
	needsJSON := false

	// Process every non-Article schema sorted by title so output is stable
	// and new schemas are picked up automatically.
	var names []string
	for name := range schemas {
		if name != "Article" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		types.WriteString(generateObject(name, schemas[name], nil))
	}

	if art, ok := schemas["Article"]; ok {
		// Check properties and all defs for oneOf — either location requires the json import.
		needsJSON = hasOneOf(art)
		if !needsJSON {
			for _, def := range art.Defs {
				if hasOneOf(def) {
					needsJSON = true
					break
				}
			}
		}
		// $defs types must appear before Article references them.
		for _, defName := range sortedKeys(art.Defs) {
			types.WriteString(generateObject(defName, art.Defs[defName], art.Defs))
		}
		types.WriteString(generateObject("Article", art, art.Defs))
	}

	var b strings.Builder
	b.WriteString("// Code generated by cmd/gen. DO NOT EDIT.\n\n")
	b.WriteString("package model\n\n")
	if needsJSON {
		b.WriteString("import (\n\t\"encoding/json\"\n\t\"fmt\"\n)\n\n")
	}
	b.WriteString(types.String())

	return format.Source([]byte(b.String()))
}
