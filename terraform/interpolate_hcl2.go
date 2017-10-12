package terraform

import (
	"fmt"
	"os"

	"github.com/hashicorp/terraform/config/hcl2shim"

	"github.com/hashicorp/terraform/helper/didyoumean"

	hcl2 "github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/terraform/config"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// This file is a companion to interpolate.go that has a parallel codepath
// targeting the HCL2-based evaluator.
//
// The code in here is used in preference to that in interpolate.go when
// we're preparing a scope to evaluate an HCL2 body.

// ValuesCty is a version of Values that produces values using the type system
// exposed by the "cty" package, which is the type system underlying HCL2.
func (i *Interpolater) ValuesCty(
	scope *InterpolationScope,
	vars []config.InterpolatedVariable,
) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	result := map[string]cty.Value{}

	// During processing we'll use separate maps for each "namespace" within
	// the scope, and then we'll flatten everything into the single result
	// map at the end. This is easier, because cty values are immutable so
	// we can avoid copying by keeping with mutable Go maps until we have
	// the final values.
	variables := map[string]cty.Value{}
	locals := map[string]cty.Value{}
	modules := map[string]cty.Value{}
	dataResources := map[string]map[string]cty.Value{}
	managedResources := map[string]map[string]cty.Value{}

	for _, vr := range vars {
		switch tvr := vr.(type) {
		case *config.CountVariable:
			if _, defined := result["count"]; !defined {
				var varDiags tfdiags.Diagnostics
				result["count"], varDiags = i.valueCtyCountVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		case *config.ModuleVariable:
			if _, defined := modules[tvr.Name]; !defined {
				var varDiags tfdiags.Diagnostics
				modules[tvr.Name], varDiags = i.valueCtyModuleVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		case *config.PathVariable:
			if _, defined := result["path"]; !defined {
				var varDiags tfdiags.Diagnostics
				result["path"], varDiags = i.valueCtyPathVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		case *config.ResourceVariable:
			var m map[string]map[string]cty.Value
			switch tvr.Mode {
			case config.DataResourceMode:
				m = dataResources
			case config.ManagedResourceMode:
				m = managedResources
			default:
				diags = diags.Append(fmt.Errorf("Unsupported resource mode %s; this is a bug in Terraform and should be reported", tvr.Mode))
				continue
			}

			if _, defined := m[tvr.Type]; !defined {
				m[tvr.Type] = map[string]cty.Value{}
			}
			if _, defined := m[tvr.Type][tvr.Name]; !defined {
				var varDiags tfdiags.Diagnostics
				m[tvr.Type][tvr.Name], varDiags = i.valueCtyResourceVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		case *config.SelfVariable:
			if _, defined := result["self"]; !defined {
				var varDiags tfdiags.Diagnostics
				result["self"], varDiags = i.valueCtySelfVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		case *config.TerraformVariable:
			if _, defined := result["terraform"]; !defined {
				var varDiags tfdiags.Diagnostics
				result["terraform"], varDiags = i.valueCtyTerraformVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		case *config.LocalVariable:
			if _, defined := locals[tvr.Name]; !defined {
				var varDiags tfdiags.Diagnostics
				locals[tvr.Name], varDiags = i.valueCtyLocalVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		case *config.UserVariable:
			if _, defined := locals[tvr.Name]; !defined {
				var varDiags tfdiags.Diagnostics
				variables[tvr.Name], varDiags = i.valueCtyUserVar(tvr, scope)
				diags = diags.Append(varDiags)
			}
		default:
			diags = diags.Append(fmt.Errorf("Unsupported internal variable type %T; this is a bug in Terraform and should be reported", vr))
			continue
		}
	}

	// Now we'll wrap up our intermediate maps and place them in the result map
	if len(variables) > 0 {
		result["var"] = cty.ObjectVal(variables)
	}
	if len(locals) > 0 {
		result["local"] = cty.ObjectVal(locals)
	}
	if len(modules) > 0 {
		result["module"] = cty.ObjectVal(modules)
	}
	if len(dataResources) > 0 {
		// Need to flatten down the data resources in two steps, to
		// take care of the extra "type" level.
		dataResourcesByType := map[string]cty.Value{}

		for typeName, resources := range dataResources {
			dataResourcesByType[typeName] = cty.ObjectVal(resources)
		}

		result["data"] = cty.ObjectVal(dataResourcesByType)
	}
	if len(managedResources) > 0 {
		// Managed resources now just go into the top-level map, finally.
		for typeName, resources := range managedResources {
			result[typeName] = cty.ObjectVal(resources)
		}
	}

	return result, diags
}

func (i *Interpolater) valueCtyCountVar(vr *config.CountVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var index cty.Value

	if scope.Resource != nil {
		index = cty.NumberIntVal(int64(scope.Resource.CountIndex))
	} else {
		index = cty.UnknownVal(cty.Number)
		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Invalid use of \"count\"",
			Detail:   "Attributes of \"count\" be used only within a \"resource\" or \"data\" block.",
			Subject:  vr.SourceRange().ToHCL().Ptr(),
		})
	}

	return cty.ObjectVal(map[string]cty.Value{
		"index": index,
	}), diags
}

func (i *Interpolater) valueCtyModuleVar(vr *config.ModuleVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Build the path to the child module we want
	path := make([]string, 0, len(scope.Path)+1)
	path = append(path, scope.Path...)
	path = append(path, vr.Name)

	i.StateLock.RLock()
	defer i.StateLock.RUnlock()

	// Need to first implement the construction of a schema for a module,
	// so we'll know what object type to return.
	panic("valueCtyModuleVar not yet implemented")

	return cty.DynamicVal, diags
}

func (i *Interpolater) valueCtyPathVar(vr *config.PathVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	var cwd, modDir, rootDir string
	{
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			diags = diags.Append(&hcl2.Diagnostic{
				Severity: hcl2.DiagError,
				Summary:  "Unable to get current working directory",
				Detail:   fmt.Sprintf("Failed to determine working directory for path.cwd: %s.", err),
				Subject:  vr.SourceRange().ToHCL().Ptr(),
			})
		}
	}
	{
		mod := i.Module.Child(scope.Path[1:])
		if mod != nil {
			modDir = mod.Config().Dir
		} else {
			diags = diags.Append(&hcl2.Diagnostic{
				Severity: hcl2.DiagError,
				Summary:  "Unable to get path for current module",
				Detail:   "The path for the current module is not available. This is a bug in Terraform and should be reported.",
				Subject:  vr.SourceRange().ToHCL().Ptr(),
			})
		}
	}
	{
		rootDir = i.Module.Config().Dir
	}

	return cty.ObjectVal(map[string]cty.Value{
		"cwd":    cty.StringVal(cwd),
		"module": cty.StringVal(modDir),
		"root":   cty.StringVal(rootDir),
	}), diags
}

func (i *Interpolater) valueCtyResourceVar(vr *config.ResourceVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Need to first gather resource schemata so we know what type we're
	// returning here.
	panic("valueCtyResourceVar not yet implemented")

	return cty.DynamicVal, diags
}

func (i *Interpolater) valueCtySelfVar(vr *config.SelfVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Need to first gather resource schemata so we know what type we're
	// returning here.
	panic("valueCtySelfVar not yet implemented")

	return cty.DynamicVal, diags
}

func (i *Interpolater) valueCtyTerraformVar(vr *config.TerraformVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	return cty.ObjectVal(map[string]cty.Value{
		// This is called "Env" as a leftover remnant of the old term for
		// "workspace", which was "state environment".
		"workspace": cty.StringVal(i.Meta.Env),
	}), nil
}

func (i *Interpolater) valueCtyLocalVar(vr *config.LocalVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	modTree := i.Module
	if len(scope.Path) > 1 {
		modTree = i.Module.Child(scope.Path[1:])
	}

	// Get the declaration from the configuration so we can verify
	// that the name is declared and so we can access the configuration
	// if we need to.
	var cl *config.Local
	for _, l := range modTree.Config().Locals {
		if l.Name == vr.Name {
			cl = l
			break
		}
	}

	if cl == nil {
		var suggestions []string
		for _, l := range modTree.Config().Locals {
			suggestions = append(suggestions, l.Name)
		}
		suggestion := didyoumean.NameSuggestion(vr.Name, suggestions)
		if suggestion != "" {
			suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
		}

		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Reference to undefined local value",
			Detail:   fmt.Sprintf("This module defines no local value named %q.%s", vr.Name, suggestion),
			Subject:  vr.SourceRange().ToHCL().Ptr(),
		})
		return cty.DynamicVal, diags
	}

	// Get the relevant module
	module := i.State.ModuleByPath(scope.Path)
	if module == nil {
		return cty.DynamicVal, diags
	}

	rawV, exists := module.Locals[vr.Name]
	if !exists {
		return cty.DynamicVal, diags
	}

	return hcl2shim.HCL2ValueFromConfigValue(rawV), diags
}

func (i *Interpolater) valueCtyUserVar(vr *config.UserVariable, scope *InterpolationScope) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	i.VariableValuesLock.Lock()
	defer i.VariableValuesLock.Unlock()

	modTree := i.Module
	if len(scope.Path) > 1 {
		modTree = i.Module.Child(scope.Path[1:])
	}

	// Get the declaration from the configuration so we can verify
	// that the name is declared and so we can access the configuration
	// if we need to.
	var cv *config.Variable
	for _, cfg := range modTree.Config().Variables {
		if cfg.Name == vr.Name {
			cv = cfg
			break
		}
	}

	if cv == nil {
		var suggestions []string
		for _, cfg := range modTree.Config().Variables {
			suggestions = append(suggestions, cfg.Name)
		}
		suggestion := didyoumean.NameSuggestion(vr.Name, suggestions)
		if suggestion != "" {
			suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
		}

		diags = diags.Append(&hcl2.Diagnostic{
			Severity: hcl2.DiagError,
			Summary:  "Reference to undefined variable",
			Detail:   fmt.Sprintf("This module declares no variable named %q.%s", vr.Name, suggestion),
			Subject:  vr.SourceRange().ToHCL().Ptr(),
		})
		return cty.DynamicVal, diags
	}

	// During the validate walk, all variables are "unknown".
	if i.Operation == walkValidate {
		// Variables are typed based on user input, so we can't know final yet.
		return cty.DynamicVal, diags
	}

	if val, ok := i.VariableValues[vr.Name]; ok {
		return hcl2shim.HCL2ValueFromConfigValue(val), diags
	}

	// If it isn't set then we'll try for a default.
	if cv.Default != nil {
		return hcl2shim.HCL2ValueFromConfigValue(cv.Default), diags
	}

	// We should never get here for a _valid_ config, but we may be in the
	// process of validating!
	return cty.DynamicVal, diags
}
