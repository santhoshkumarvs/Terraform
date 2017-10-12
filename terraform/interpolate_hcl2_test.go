package terraform

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	hcl2 "github.com/hashicorp/hcl2/hcl"
	hcl2syntax "github.com/hashicorp/hcl2/hcl/hclsyntax"
	hcl2dec "github.com/hashicorp/hcl2/hcldec"
	"github.com/hashicorp/terraform/config"
	"github.com/hashicorp/terraform/config/hcl2shim"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestInterpolatorValuesCty(t *testing.T) {
	module := testModule(t, "interpolate-hcl2")
	state := &State{
		Modules: []*ModuleState{
			{
				Path: []string{"root"},
				Locals: map[string]interface{}{
					"foo": "local foo",
				},
			},
		},
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	modulePath := module.Child([]string{"child"}).Config().Dir // gets copied into the data dir
	rootPath := filepath.Join(cwd, "test-fixtures", "interpolate-hcl2")

	tests := map[string]struct {
		SrcExpr   string
		Scope     *InterpolationScope
		Want      map[string]cty.Value
		DiagCount int
	}{
		"empty": {
			"true",
			&InterpolationScope{},
			map[string]cty.Value{},
			0,
		},

		// count.*
		"count.index inside resource": {
			"count.index",
			&InterpolationScope{
				Resource: &Resource{
					CountIndex: 2,
				},
			},
			map[string]cty.Value{
				"count": cty.ObjectVal(map[string]cty.Value{
					"index": cty.NumberIntVal(2),
				}),
			},
			0,
		},
		"count.index outside resource": {
			"count.index",
			&InterpolationScope{
				Resource: nil,
			},
			map[string]cty.Value{
				"count": cty.ObjectVal(map[string]cty.Value{
					"index": cty.UnknownVal(cty.Number),
				}),
			},
			1, // invalid use of "count"
		},

		// module.*
		// TODO: module.* is not yet implemented

		// path.*
		"path.cwd": {
			"path.cwd",
			&InterpolationScope{
				Path: []string{"root", "child"},
			},
			map[string]cty.Value{
				"path": cty.ObjectVal(map[string]cty.Value{
					"module": cty.StringVal(modulePath),
					"root":   cty.StringVal(rootPath),
					"cwd":    cty.StringVal(cwd),
				}),
			},
			0,
		},

		// data.*
		// TODO: data.* is not yet implemented

		// managed resources (unprefixed)
		// TODO: managed resources are not yet implemented

		// self.*
		// TODO: self.* is not yet implemented

		// terraform.*
		"terraform.workspace": {
			"terraform.workspace",
			&InterpolationScope{},
			map[string]cty.Value{
				"terraform": cty.ObjectVal(map[string]cty.Value{
					"workspace": cty.StringVal("foo-env"),
				}),
			},
			0,
		},

		// local.*
		"local value that exists": {
			"local.foo",
			&InterpolationScope{
				Path: []string{"root"},
			},
			map[string]cty.Value{
				"local": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("local foo"),
				}),
			},
			0,
		},
		"local value that isn't defined yet": {
			"local.undefined",
			&InterpolationScope{
				Path: []string{"root"},
			},
			map[string]cty.Value{
				"local": cty.ObjectVal(map[string]cty.Value{
					"undefined": cty.DynamicVal,
				}),
			},
			0,
		},
		"local value that doesn't exist": {
			"local.fou",
			&InterpolationScope{
				Path: []string{"root"},
			},
			map[string]cty.Value{
				"local": cty.ObjectVal(map[string]cty.Value{
					"fou": cty.DynamicVal,
				}),
			},
			1, // Reference to undefined local value
		},

		// var.*
		"var (set)": {
			"var.required",
			&InterpolationScope{},
			map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"required": cty.ObjectVal(map[string]cty.Value{
						"hello": cty.StringVal("world"),
					}),
				}),
			},
			0,
		},
		"var (defaulted)": {
			"var.optional",
			&InterpolationScope{},
			map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"optional": cty.TupleVal([]cty.Value{
						cty.StringVal("optional var default"),
					}),
				}),
			},
			0,
		},
		"var (required but not set)": {
			"var.not_set",
			&InterpolationScope{},
			map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"not_set": cty.DynamicVal,
				}),
			},
			0,
		},
		"var (misnamed)": {
			"var.requerd",
			&InterpolationScope{},
			map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"requerd": cty.DynamicVal,
				}),
			},
			1, // Reference to undefined variable
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			i := &Interpolater{
				Module:    module,
				State:     state,
				StateLock: new(sync.RWMutex),
				VariableValues: map[string]interface{}{
					"required": map[string]interface{}{"hello": "world"},
				},
				VariableValuesLock: new(sync.Mutex),
				Meta: &ContextMeta{
					Env: "foo-env",
				},
			}

			var diags tfdiags.Diagnostics
			expr, parseDiags := hcl2syntax.ParseExpression([]byte(test.SrcExpr), "", hcl2.Pos{Line: 1, Column: 1})
			diags = diags.Append(parseDiags)
			body := hcl2shim.SingleAttrBody{
				Name: "value",
				Expr: expr,
			}
			spec := &hcl2dec.AttrSpec{
				Name: "value",
				Type: cty.DynamicPseudoType,
			}

			vars, varDiags := config.DetectVariablesHCL2(body, spec)
			diags = diags.Append(varDiags)

			got, valuesDiags := i.ValuesCty(test.Scope, vars)
			diags = diags.Append(valuesDiags)

			if len(diags) != test.DiagCount {
				t.Errorf("wrong number of diagnostics %d; want %d", len(diags), test.DiagCount)
				for _, diag := range diags {
					desc := diag.Description()
					t.Logf("- %s: %s", desc.Summary, desc.Detail)
				}
			}

			if !reflect.DeepEqual(got, test.Want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}
