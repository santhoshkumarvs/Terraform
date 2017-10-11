package config

import (
	"fmt"
	"strings"

	hcl2 "github.com/hashicorp/hcl2/hcl"
	hcl2dec "github.com/hashicorp/hcl2/hcldec"
	"github.com/hashicorp/terraform/tfdiags"
)

// DetectVariablesHCL2 is the HCL2-flavored version of DetectVariables.
//
// Since HCL2's model of variables is slightly different -- in particular,
// a period is a real attribute access operator rather than part of a variable
// name -- this produces some slightly different results than for
// DetectVariables, including some variable instances that are not possible at
// all for old HCL/HIL such as whole resources/modules as objects.
//
// Consequently, it's not valid to mix different parsing and interpolation
// codepaths: expressions coming from HCL2 must always be interpolated with
// the HCL2 interpolator, and expressions from old-school HCL/HIL must always
// be interpolated with the old-school interpolator.
func DetectVariablesHCL2(body hcl2.Body, spec hcl2dec.Spec) ([]InterpolatedVariable, tfdiags.Diagnostics) {
	var vars []InterpolatedVariable
	var diags tfdiags.Diagnostics

	traversals := hcl2dec.Variables(body, spec)
	for _, trav := range traversals {
		if len(trav) == 0 {
			// Weird degenerate traversal... should never happen but we'll
			// at least avoid crashing in this case.
			continue
		}

		v, vDiags := NewInterpolatedVariableHCL2(trav)
		diags = diags.Append(vDiags)
		if v != nil {
			vars = append(vars, v)
		}
	}

	return vars, diags
}

func NewInterpolatedVariableHCL2(trav hcl2.Traversal) (InterpolatedVariable, tfdiags.Diagnostics) {
	// For our purposes here we only care about "root" and "attribute"
	// traversals, so we'll make life easier for our traversal parsers
	// by simplifying to an []string of just these names first.
	names := make([]string, 0, len(trav))
	names = append(names, trav.RootName())
	startRange := trav[0].(hcl2.TraverseRoot).SrcRange
	endRange := startRange
	for _, step := range trav[1:] {
		attrStep, isAttr := step.(hcl2.TraverseAttr)
		if !isAttr {
			// Stop if we encounter something non-attributey, like a [ ... ]
			// index step. (These will be taken care of at evaluation time,
			// by inserting a list, map, etc into the scope.)
			break
		}
		names = append(names, attrStep.Name)
		endRange = attrStep.SrcRange
	}

	// The range we're passing in to these variable constructors is actually
	// for the full path of segments, even though the constructors will often
	// use only a portion of the path (e.g. var.foo.bar will only use var.foo).
	// For now we're accepting this limitation and that it will cause some
	// slightly-oversize ranges to be reported in error messages about invalid
	// variable references.
	rng := hcl2.RangeBetween(startRange, endRange)

	switch trav.RootName() {
	case "count":
		return newCountVariableHCL2(names, rng)
	case "path":
		return newPathVariableHCL2(names, rng)
	case "self":
		return newSelfVariableHCL2(names, rng)
	case "terraform":
		return newTerraformVariableHCL2(names, rng)
	case "var":
		return newUserVariableHCL2(names, rng)
	case "local":
		return newLocalVariableHCL2(names, rng)
	case "module":
		return newModuleVariableHCL2(names, rng)
	default:
		return newResourceVariableHCL2(names, rng)
	}
}

func newCountVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if len(names) < 2 { // too many is okay because we'll fail at eval time trying to attribute a number
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Missing \"count\" attribute",
			Detail:   "The name \"count\" does not have a direct value; access an attribute of this object, such as count.index.",
			Subject:  &rng,
		})
		return nil, diags
	}

	attrName := names[1]
	var attr CountValueType
	switch attrName {
	case "index":
		attr = CountValueIndex
	default:
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Invalid \"count\" attribute",
			Detail:   fmt.Sprintf("The name \"count\" does not have an attribute named %q. The only available attribute is \"index\".", attrName),
			Subject:  &rng,
		})
	}

	return &CountVariable{
		Type:     attr,
		key:      "count." + attrName,
		varRange: makeVarRangeHCL2(rng),
	}, diags
}

func newPathVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if len(names) < 2 { // too many is okay because we'll fail at eval time trying to attribute a string
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Missing \"path\" attribute",
			Detail:   "The name \"path\" does not have a direct value; access an attribute of this object, such as path.module.",
			Subject:  &rng,
		})
		return nil, diags
	}

	attrName := names[1]
	var attr PathValueType
	switch attrName {
	case "cwd":
		attr = PathValueCwd
	case "module":
		attr = PathValueModule
	case "root":
		attr = PathValueRoot
	default:
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Invalid \"path\" attribute",
			Detail:   fmt.Sprintf("The name \"path\" does not have an attribute named %q.", attrName),
			Subject:  &rng,
		})
	}

	return &PathVariable{
		Type:     attr,
		key:      "path." + attrName,
		varRange: makeVarRangeHCL2(rng),
	}, diags
}

func newSelfVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	// For HCL2 we generate a self variable with no "field", to suggest that
	// we want to put the entire object in the scope under that name, rather
	// than try to cherry-pick specific attributes.

	return &SelfVariable{
		key:      "self",
		varRange: makeVarRangeHCL2(rng),
	}, nil
}

func newTerraformVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if len(names) < 2 { // too many is okay because we'll fail at eval time trying to attribute a string
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Missing \"terraform\" attribute",
			Detail:   "The name \"terraform\" does not have a direct value; access an attribute of this object, such as terraform.workspace.",
			Subject:  &rng,
		})
		return nil, diags
	}

	attrName := names[1]
	switch attrName {
	case "workspace":
		// okay
	default:
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Invalid \"terraform\" attribute",
			Detail:   fmt.Sprintf("The name \"terraform\" does not have an attribute named %q. The only available attribute is \"workspace\".", attrName),
			Subject:  &rng,
		})
	}

	return &TerraformVariable{
		Field:    attrName,
		key:      "terraform." + attrName,
		varRange: makeVarRangeHCL2(rng),
	}, diags
}

func newUserVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if len(names) < 2 {
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Missing \"var\" attribute",
			Detail:   "The name \"var\" does not have a direct value; access an attribute of this object, named after one of the variables in this module.",
			Subject:  &rng,
		})
		return nil, diags
	}

	return &UserVariable{
		Name:     names[1],
		key:      "var." + names[1],
		varRange: makeVarRangeHCL2(rng),
	}, diags
}

func newLocalVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if len(names) < 2 {
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Missing \"local\" attribute",
			Detail:   "The name \"local\" does not have a direct value; access an attribute of this object, named after one of the local values in this module.",
			Subject:  &rng,
		})
		return nil, diags
	}

	return &LocalVariable{
		Name:     names[1],
		varRange: makeVarRangeHCL2(rng),
	}, diags
}

func newModuleVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if len(names) < 2 {
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Missing \"module\" attribute",
			Detail:   "The name \"module\" does not have a direct value; access an attribute of this object, named after one of the child modules of this module.",
			Subject:  &rng,
		})
		return nil, diags
	}

	// We generate a ModuleVariable with an empty field, indicating to the
	// HCL2-flavored scope builder that we want to place the entire object
	// in the scope.
	return &ModuleVariable{
		Name:     names[1],
		key:      "module." + names[1],
		varRange: makeVarRangeHCL2(rng),
	}, diags
}

func newResourceVariableHCL2(names []string, rng hcl2.Range) (InterpolatedVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var mode ResourceMode
	var parts []string
	switch names[0] {
	case "data":
		mode = DataResourceMode
		parts = names[1:] // trim "data" from the front for our remaining processing
		if len(names) > 3 {
			names = names[:3] // only need to go as deep as the data resource itself
		}
	default:
		mode = ManagedResourceMode
		parts = names
		if len(names) > 2 {
			names = names[:2] // only need to go as deep as the managed resource itself
		}
	}

	if len(parts) == 0 {
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Missing \"data\" attribute",
			Detail:   "The name \"data\" does not have a direct value; access an attribute of this object, named after one of the data sources used in this module.",
			Subject:  &rng,
		})
		return nil, diags
	}
	if len(parts) < 2 {
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Incomplete resource access",
			Detail:   "Access of a resource or data source must include both a type and a name.",
			Subject:  &rng,
		})
		return nil, diags
	}

	// For HCL2 resource access, we populate only the mode, type and name of
	// the resource since the HCL2 scope builder will place the entire object
	// (possibly a list of objects, if count is set) in the scope, allowing
	// the remainder to be accessed via the attribute, index, and splat
	// operators.
	return &ResourceVariable{
		Mode:     mode,
		Type:     parts[0],
		Name:     parts[1],
		key:      strings.Join(names, "."),
		varRange: makeVarRangeHCL2(rng),
	}, diags
}

func makeVarRangeHCL2(rng hcl2.Range) varRange {
	return varRange{tfdiags.SourceRangeFromHCL(rng)}
}
