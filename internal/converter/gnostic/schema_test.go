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
	"testing"

	"connectrpc.com/protoc-gen-connect-openapi/internal/converter/options"
	goa3 "github.com/google/gnostic/openapiv3"
	"github.com/pb33f/go-yaml"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandExampleEntry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "object", value: "{id: 1, label: first}", want: "id: 1\nlabel: first\n"},
		{name: "nested object in sequence", value: "[{id: 1}, {id: 2}]", want: "- id: 1\n- id: 2\n"},
		{name: "non-string key", value: "{1: one}", want: "1: one\n"},
		{name: "multiple non-string keys", value: "{2: two, 1: one}", want: "1: one\n2: two\n"},
		{name: "boolean", value: "true", want: "true\n"},
		{name: "integer", value: "42", want: "42\n"},
		{name: "float", value: "1.5", want: "1.5\n"},
		{name: "string", value: "Example value", want: "Example value\n"},
		{name: "quoted numeric string", value: "'123'", want: "\"123\"\n"},
		{name: "null", value: "null", want: "null\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var node any
			require.NoError(t, yaml.Unmarshal([]byte(tc.value), &node))
			out, err := yaml.Marshal(expandExampleEntry(node))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(out))
		})
	}
}

func TestSchemaWithAnnotationsExampleTypes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "object", value: "{id: 1, label: first}", want: "id: 1\nlabel: first\n"},
		{name: "boolean", value: "true", want: "true\n"},
		{name: "integer", value: "42", want: "42\n"},
		{name: "sequence", value: "[20, 40, 60]", want: "- 20\n- 40\n- 60\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			schema := schemaWithAnnotations(options.Options{}, &base.Schema{}, &goa3.Schema{
				Example: &goa3.Any{Yaml: tc.value},
			})
			require.Len(t, schema.Examples, 1)
			out, err := yaml.Marshal(schema.Examples[0])
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(out))
		})
	}
}
