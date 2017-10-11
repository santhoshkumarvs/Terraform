package config

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/hashicorp/terraform/tfdiags"

	hcl2 "github.com/hashicorp/hcl2/hcl"
	hcl2syntax "github.com/hashicorp/hcl2/hcl/hclsyntax"
	hcl2dec "github.com/hashicorp/hcl2/hcldec"
	"github.com/zclconf/go-cty/cty"
)

func TestDetectVariablesHCL2(t *testing.T) {
	tests := []struct {
		Expr      string
		Want      []InterpolatedVariable
		DiagCount int
	}{
		{
			`true`,
			nil,
			0,
		},

		// count.*
		{
			`count.index`,
			[]InterpolatedVariable{
				&CountVariable{
					Type: CountValueIndex,
					key:  "count.index",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 12, Byte: 11},
					}),
				},
			},
			0,
		},
		{
			`count.index.blah`,
			[]InterpolatedVariable{
				&CountVariable{
					Type: CountValueIndex,
					key:  "count.index",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 17, Byte: 16},
					}),
				},
			},
			0,
		},
		{
			`count.baz`,
			[]InterpolatedVariable{
				&CountVariable{
					Type: CountValueInvalid,
					key:  "count.baz",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 10, Byte: 9},
					}),
				},
			},
			1, // invalid "count" attribute
		},
		{
			`count`,
			nil,
			1, // missing "count" attribute
		},

		// path.*
		{
			`path.module`,
			[]InterpolatedVariable{
				&PathVariable{
					Type: PathValueModule,
					key:  "path.module",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 12, Byte: 11},
					}),
				},
			},
			0,
		},
		{
			`path.root`,
			[]InterpolatedVariable{
				&PathVariable{
					Type: PathValueRoot,
					key:  "path.root",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 10, Byte: 9},
					}),
				},
			},
			0,
		},
		{
			`path.cwd`,
			[]InterpolatedVariable{
				&PathVariable{
					Type: PathValueCwd,
					key:  "path.cwd",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 9, Byte: 8},
					}),
				},
			},
			0,
		},
		{
			`path.module.blah`,
			[]InterpolatedVariable{
				&PathVariable{
					Type: PathValueModule,
					key:  "path.module",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 17, Byte: 16},
					}),
				},
			},
			0,
		},
		{
			`path.mollycoddle`,
			[]InterpolatedVariable{
				&PathVariable{
					Type: PathValueInvalid,
					key:  "path.mollycoddle",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 17, Byte: 16},
					}),
				},
			},
			1, // Invalid "path" attribute
		},
		{
			`path`,
			nil,
			1, // Missing "path" attribute
		},

		// self.*
		{
			`self`,
			[]InterpolatedVariable{
				&SelfVariable{
					key: "self",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 5, Byte: 4},
					}),
				},
			},
			0,
		},
		{
			`self.blah`,
			[]InterpolatedVariable{
				&SelfVariable{
					key: "self",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 10, Byte: 9},
					}),
				},
			},
			0,
		},

		// terraform.*
		{
			`terraform.workspace`,
			[]InterpolatedVariable{
				&TerraformVariable{
					Field: "workspace",
					key:   "terraform.workspace",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 20, Byte: 19},
					}),
				},
			},
			0,
		},
		{
			`terraform.env`, // was deprecated prior to HCL2, and not supported at all in HCL2
			[]InterpolatedVariable{
				&TerraformVariable{
					Field: "env",
					key:   "terraform.env",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 14, Byte: 13},
					}),
				},
			},
			1, // Invalid "terraform" attribute
		},
		{
			`terraform`,
			nil,
			1, // Missing "terraform" attribute
		},

		// var.*
		{
			`var.foo`,
			[]InterpolatedVariable{
				&UserVariable{
					Name: "foo",
					key:  "var.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 8, Byte: 7},
					}),
				},
			},
			0,
		},
		{
			`var.foo.bar`,
			[]InterpolatedVariable{
				&UserVariable{
					Name: "foo",
					key:  "var.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 12, Byte: 11},
					}),
				},
			},
			0,
		},
		{
			`var`,
			nil,
			1, // missing "var" attribute
		},

		// local.*
		{
			`local.foo`,
			[]InterpolatedVariable{
				&LocalVariable{
					Name: "foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 10, Byte: 9},
					}),
				},
			},
			0,
		},
		{
			`local.foo.bar`,
			[]InterpolatedVariable{
				&LocalVariable{
					Name: "foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 14, Byte: 13},
					}),
				},
			},
			0,
		},
		{
			`local`,
			nil,
			1, // missing "local" attribute
		},

		// module.*
		{
			`module.foo`,
			[]InterpolatedVariable{
				&ModuleVariable{
					Name: "foo",
					key:  "module.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 11, Byte: 10},
					}),
				},
			},
			0,
		},
		{
			`module.foo.bar`,
			[]InterpolatedVariable{
				&ModuleVariable{
					Name: "foo",
					key:  "module.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 15, Byte: 14},
					}),
				},
			},
			0,
		},
		{
			`module`,
			nil,
			1, // missing "module" attribute
		},

		// data.*
		{
			`data.test_thing.foo`,
			[]InterpolatedVariable{
				&ResourceVariable{
					Mode: DataResourceMode,
					Type: "test_thing",
					Name: "foo",
					key:  "data.test_thing.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 20, Byte: 19},
					}),
				},
			},
			0,
		},
		{
			`data.test_thing.foo.bar`,
			[]InterpolatedVariable{
				&ResourceVariable{
					Mode: DataResourceMode,
					Type: "test_thing",
					Name: "foo",
					key:  "data.test_thing.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 24, Byte: 23},
					}),
				},
			},
			0,
		},
		{
			`data.test_thing`,
			nil,
			1, // Incomplete resource access
		},
		{
			`data`,
			nil,
			1, // Missing "data" attribute
		},

		// managed resource references (unprefixed)
		{
			`test_thing.foo`,
			[]InterpolatedVariable{
				&ResourceVariable{
					Mode: ManagedResourceMode,
					Type: "test_thing",
					Name: "foo",
					key:  "test_thing.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 15, Byte: 14},
					}),
				},
			},
			0,
		},
		{
			`test_thing.foo.bar`,
			[]InterpolatedVariable{
				&ResourceVariable{
					Mode: ManagedResourceMode,
					Type: "test_thing",
					Name: "foo",
					key:  "test_thing.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 19, Byte: 18},
					}),
				},
			},
			0,
		},
		{
			`test_thing`,
			nil,
			1, // Incomplete resource access
		},

		// combinations and other interesting situations
		{
			`var.foo + local.bar`,
			[]InterpolatedVariable{
				&UserVariable{
					Name: "foo",
					key:  "var.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 8, Byte: 7},
					}),
				},
				&LocalVariable{
					Name: "bar",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 11, Byte: 10},
						End:   tfdiags.SourcePos{Line: 1, Column: 20, Byte: 19},
					}),
				},
			},
			0,
		},
		{
			`var.foo + var.foo`,
			[]InterpolatedVariable{
				&UserVariable{
					Name: "foo",
					key:  "var.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 8, Byte: 7},
					}),
				},
				&UserVariable{
					Name: "foo",
					key:  "var.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 11, Byte: 10},
						End:   tfdiags.SourcePos{Line: 1, Column: 18, Byte: 17},
					}),
				},
			},
			0,
		},
		{
			`var.foo["eek"]`,
			[]InterpolatedVariable{
				&UserVariable{
					Name: "foo",
					key:  "var.foo",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 8, Byte: 7},
					}),
				},
			},
			0,
		},
		{
			`upper(local.baz)`,
			[]InterpolatedVariable{
				&LocalVariable{
					Name: "baz",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 7, Byte: 6},
						End:   tfdiags.SourcePos{Line: 1, Column: 16, Byte: 15},
					}),
				},
			},
			0,
		},
		{
			`local.baz[local.baz_idx]`,
			[]InterpolatedVariable{
				&LocalVariable{
					Name: "baz",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 1, Column: 10, Byte: 9},
					}),
				},
				&LocalVariable{
					Name: "baz_idx",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 11, Byte: 10},
						End:   tfdiags.SourcePos{Line: 1, Column: 24, Byte: 23},
					}),
				},
			},
			0,
		},
		{
			`"${local.baz}"`,
			[]InterpolatedVariable{
				&LocalVariable{
					Name: "baz",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 4, Byte: 3},
						End:   tfdiags.SourcePos{Line: 1, Column: 13, Byte: 12},
					}),
				},
			},
			0,
		},
		{
			// The local scope created by a "for" expression can mask our
			// top-level variables. These should _not_ be detected, since
			// they are populated dynamically by the HCL runtime rather than
			// provided directly by Terraform.
			// (Masking top-level variables in "for" is bad style, even though
			// it's supported and tested here.)
			`[for local, module in local.thingy: "${local.foo} and ${terraform.workspace}" if local && module]`,
			[]InterpolatedVariable{
				&LocalVariable{ // the reference after the "in" keyword, into parent scope
					Name: "thingy",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 23, Byte: 22},
						End:   tfdiags.SourcePos{Line: 1, Column: 35, Byte: 34},
					}),
				},
				&TerraformVariable{ // the "terraform" ref in the value clause, since "terraform" isn't masked
					Field: "workspace",
					key:   "terraform.workspace",
					varRange: makeVarRange(tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 57, Byte: 56},
						End:   tfdiags.SourcePos{Line: 1, Column: 76, Byte: 75},
					}),
				},
				// "local" and "module" in the decl clause are not variable references,
				// and are thus not considered at all.
				//
				// "local.foo" in the value clause and "local"/"module" in the condition
				// clause refer to the for expression's child scope and are thus not
				// detected.
			},
			0,
		},
	}

	for _, test := range tests {
		t.Run(test.Expr, func(t *testing.T) {
			var diags tfdiags.Diagnostics
			expr, parseDiags := hcl2syntax.ParseExpression([]byte(test.Expr), "", hcl2.Pos{Line: 1, Column: 1})
			diags = diags.Append(parseDiags)
			body := hcl2SingleAttrBody{
				Name: "value",
				Expr: expr,
			}
			spec := &hcl2dec.AttrSpec{
				Name: "value",
				Type: cty.DynamicPseudoType,
			}

			got, varDiags := DetectVariablesHCL2(body, spec)
			diags = diags.Append(varDiags)

			if len(diags) != test.DiagCount {
				t.Errorf("wrong number of diagnostics %d; want %d", len(diags), test.DiagCount)
				for _, diag := range diags {
					desc := diag.Description()
					t.Logf("- %s: %s", desc.Summary, desc.Detail)
				}
			}

			if !reflect.DeepEqual(got, test.Want) {
				t.Errorf("wrong result\ngot: %swant: %s", spew.Sdump(got), spew.Sdump(test.Want))
			}
		})
	}
}
