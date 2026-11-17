// Copyright 2023-2026 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gnostic

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

	"connectrpc.com/protoc-gen-connect-openapi/internal/converter/options"
	"connectrpc.com/protoc-gen-connect-openapi/internal/converter/util"
	goa3 "github.com/google/gnostic/openapiv3"
	"github.com/pb33f/go-yaml"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/utils"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func SchemaWithSchemaAnnotations(opts options.Options, schema *base.Schema, desc protoreflect.MessageDescriptor) *base.Schema {
	if !proto.HasExtension(desc.Options(), goa3.E_Schema.TypeDescriptor().Type()) {
		return schema
	}

	ext := proto.GetExtension(desc.Options(), goa3.E_Schema.TypeDescriptor().Type())
	gnosticSchema, ok := ext.(*goa3.Schema)
	if !ok {
		return schema
	}
	return schemaWithAnnotations(opts, schema, gnosticSchema)
}

func SchemaWithPropertyAnnotations(opts options.Options, schema *base.Schema, desc protoreflect.FieldDescriptor) *base.Schema {
	if !proto.HasExtension(desc.Options(), goa3.E_Property.TypeDescriptor().Type()) {
		return schema
	}

	ext := proto.GetExtension(desc.Options(), goa3.E_Property.TypeDescriptor().Type())
	gnosticSchema, ok := ext.(*goa3.Schema)
	if !ok {
		return schema
	}
	// Preserve the numeric base for explicit overrides and constraints.
	if (desc.Kind() == protoreflect.FloatKind || desc.Kind() == protoreflect.DoubleKind) && len(schema.OneOf) > 0 {
		schema.Type = []string{"number"}
		schema.Format = desc.Kind().String()
		schema.OneOf = nil
	}
	return schemaWithAnnotations(opts, schema, gnosticSchema)
}

//gocyclo:ignore
func schemaWithAnnotations(opts options.Options, schema *base.Schema, gnosticSchema *goa3.Schema) *base.Schema {
	if gnosticSchema.Description != "" {
		schema.Description = gnosticSchema.Description
	}
	if gnosticSchema.Title != "" {
		schema.Title = gnosticSchema.Title
	}
	if gnosticSchema.Format != "" {
		schema.Format = gnosticSchema.Format
	}
	if gnosticSchema.Nullable {
		if !slices.Contains(schema.Type, "null") {
			schema.Type = append(schema.Type, "null")
		}
	}
	if gnosticSchema.ReadOnly {
		schema.ReadOnly = &gnosticSchema.ReadOnly
	}
	if gnosticSchema.WriteOnly {
		schema.WriteOnly = &gnosticSchema.WriteOnly
	}
	if gnosticSchema.Example != nil {
		// Decode as-is so scalars keep their YAML type.
		if yamlText := gnosticSchema.Example.GetYaml(); yamlText != "" {
			var node any
			if err := yaml.Unmarshal([]byte(yamlText), &node); err != nil {
				opts.Logger.Warn("unable to unmarshal example", slog.Any("error", err))
			} else {
				schema.Examples = append(schema.Examples, expandExampleEntry(node))
			}
		}
		// If the example is defined with google.protobuf.Any
		if gnosticSchema.Example.Value != nil {
			opts.Logger.Warn("unable to unmarshal pb.any example")
		}
	}
	if gnosticSchema.ExternalDocs != nil {
		schema.ExternalDocs = toExternalDocs(gnosticSchema.ExternalDocs)
	}
	if gnosticSchema.Deprecated {
		schema.Deprecated = &gnosticSchema.Deprecated
	}
	if gnosticSchema.MultipleOf != 0 {
		schema.MultipleOf = &gnosticSchema.MultipleOf
	}
	if gnosticSchema.Maximum != 0 {
		if gnosticSchema.ExclusiveMaximum {
			schema.ExclusiveMaximum = &base.DynamicValue[bool, float64]{N: 1, B: gnosticSchema.Maximum}
		} else {
			schema.Maximum = &gnosticSchema.Maximum
		}
	}
	if gnosticSchema.Minimum != 0 {
		if gnosticSchema.ExclusiveMinimum {
			schema.ExclusiveMinimum = &base.DynamicValue[bool, float64]{N: 1, B: gnosticSchema.Minimum}
		} else {
			schema.Minimum = &gnosticSchema.Minimum
		}
	}
	if gnosticSchema.MaxLength > 0 {
		schema.MaxLength = &gnosticSchema.MaxLength
	}
	if gnosticSchema.MinLength > 0 {
		v := gnosticSchema.MinLength
		schema.MinLength = &v
	}
	if gnosticSchema.Pattern != "" {
		schema.Pattern = gnosticSchema.Pattern
	}
	if gnosticSchema.MaxItems > 0 {
		if slices.Contains(schema.Type, "array") {
			schema.MaxItems = &gnosticSchema.MaxItems
		}
	}
	if gnosticSchema.MinItems > 0 {
		if slices.Contains(schema.Type, "array") {
			schema.MinItems = &gnosticSchema.MinItems
		}
	}
	if gnosticSchema.UniqueItems {
		if slices.Contains(schema.Type, "array") {
			schema.UniqueItems = &gnosticSchema.UniqueItems
		}
	}
	if gnosticSchema.MaxProperties > 0 {
		if slices.Contains(schema.Type, "object") {
			schema.MaxProperties = &gnosticSchema.MaxProperties
		}
	}
	if gnosticSchema.MinProperties > 0 {
		if slices.Contains(schema.Type, "object") {
			schema.MinProperties = &gnosticSchema.MinProperties
		}
	}
	if len(gnosticSchema.Required) > 0 {
		schema.Required = gnosticSchema.Required
	}
	if len(gnosticSchema.Enum) > 0 {
		enums := make([]*yaml.Node, len(gnosticSchema.Enum))
		for i, enum := range gnosticSchema.Enum {
			enums[i] = util.ConvertNodeV3toV4(enum.ToRawInfo())
		}
		schema.Enum = enums
	}
	if gnosticSchema.Type != "" {
		// Overriding the type keeps nullability, whether it came from the
		// field (proto3 optional) or from nullable above.
		nullable := slices.Contains(schema.Type, "null")
		schema.Type = []string{gnosticSchema.Type}
		if nullable {
			schema.Type = append(schema.Type, "null")
		}
	}

	if gnosticSchema.AdditionalProperties != nil {
		switch v := gnosticSchema.AdditionalProperties.GetOneof().(type) {
		case *goa3.AdditionalPropertiesItem_SchemaOrReference:
			if vv := toSchemaOrReference(opts, v.SchemaOrReference); vv != nil {
				schema.AdditionalProperties = &base.DynamicValue[*base.SchemaProxy, bool]{A: vv}
			}
		case *goa3.AdditionalPropertiesItem_Boolean:
			schema.AdditionalProperties = &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: v.Boolean}
		}
	}

	if len(gnosticSchema.AllOf) > 0 {
		schema.AllOf = toSchemaOrReferences(opts, gnosticSchema.AllOf)
	}
	if len(gnosticSchema.OneOf) > 0 {
		schema.OneOf = toSchemaOrReferences(opts, gnosticSchema.OneOf)
	}
	if len(gnosticSchema.AnyOf) > 0 {
		schema.AnyOf = toSchemaOrReferences(opts, gnosticSchema.AnyOf)
	}
	if gnosticSchema.Not != nil {
		// The schema may already forbid shapes (oneof rules), so add to that
		// rather than replace it.
		util.AddViolations(schema, base.CreateSchemaProxy(toSchema(opts, gnosticSchema.Not)))
	}
	if gnosticSchema.Items != nil {
		items := toSchemaOrReferences(opts, gnosticSchema.Items.SchemaOrReference)
		var itemsSchema *base.SchemaProxy
		if len(items) == 1 {
			itemsSchema = items[0]
		} else {
			itemsSchema = base.CreateSchemaProxy(&base.Schema{OneOf: items})
		}
		schema.Items = &base.DynamicValue[*base.SchemaProxy, bool]{A: itemsSchema}
	}
	if gnosticSchema.Properties != nil {
		schema.Properties = toSchemaOrReferenceMap(opts, gnosticSchema.Properties.GetAdditionalProperties())
	}
	if gnosticSchema.Default != nil {
		schema.Default = toDefault(gnosticSchema.Default)
	}
	if gnosticSchema.AdditionalProperties != nil {
		schema.AdditionalProperties = toAdditionalPropertiesItem(opts, gnosticSchema.AdditionalProperties)
	}
	if gnosticSchema.Xml != nil {
		extensions := *orderedmap.New[string, *yaml.Node]()
		for _, namedAny := range gnosticSchema.Xml.GetSpecificationExtension() {
			extensions.Set(namedAny.Name, util.ConvertNodeV3toV4(namedAny.ToRawInfo()))
		}
		schema.XML = &base.XML{
			Name:       gnosticSchema.Xml.Name,
			Namespace:  gnosticSchema.Xml.Namespace,
			Prefix:     gnosticSchema.Xml.Prefix,
			Attribute:  gnosticSchema.Xml.Attribute,
			Wrapped:    gnosticSchema.Xml.Wrapped,
			Extensions: &extensions,
		}
	}
	if gnosticSchema.Discriminator != nil {
		mapping := orderedmap.New[string, string]()
		for _, prop := range gnosticSchema.Discriminator.GetMapping().GetAdditionalProperties() {
			mapping.Set(prop.Name, prop.Value)
		}
		schema.Discriminator = &base.Discriminator{
			PropertyName: gnosticSchema.Discriminator.GetPropertyName(),
			Mapping:      mapping,
		}
	}
	if gnosticSchema.SpecificationExtension != nil {
		schema.Extensions = toExtensions(gnosticSchema.SpecificationExtension)
	}

	return schema
}

func expandExampleEntry(entry any) *yaml.Node {
	switch vv := entry.(type) {
	// The decoder uses map[string]any for string keys and map[any]any otherwise.
	case map[string]any:
		node := utils.CreateEmptyMapNode()
		keys := make([]string, 0, len(vv))
		for k := range vv {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			node.Content = append(node.Content, utils.CreateStringNode(k), expandExampleEntry(vv[k]))
		}
		return node
	case map[any]any:
		node := utils.CreateEmptyMapNode()
		keys := make([]any, 0, len(vv))
		for k := range vv {
			keys = append(keys, k)
		}
		// Sort by type and value so non-string keys render deterministically.
		slices.SortFunc(keys, func(a, b any) int {
			return cmp.Compare(fmt.Sprintf("%T:%v", a, a), fmt.Sprintf("%T:%v", b, b))
		})
		for _, k := range keys {
			node.Content = append(node.Content, expandExampleEntry(k), expandExampleEntry(vv[k]))
		}
		return node
	// Handle sequence nodes
	case []any:
		node := utils.CreateEmptySequenceNode()
		for _, v := range vv {
			node.Content = append(node.Content, expandExampleEntry(v))
		}
		return node
	case bool:
		return utils.CreateBoolNode(strconv.FormatBool(vv))
	case float64:
		return utils.CreateFloatNode(strconv.FormatFloat(vv, 'g', -1, 64))
	case int:
		return utils.CreateIntNode(strconv.Itoa(vv))
	case int64:
		return utils.CreateIntNode(strconv.FormatInt(vv, 10))
	case uint64:
		return utils.CreateIntNode(strconv.FormatUint(vv, 10))
	case string:
		return utils.CreateStringNode(vv)
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	default:
		return utils.CreateStringNode(fmt.Sprintf("%v", vv))
	}
}
