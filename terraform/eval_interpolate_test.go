package terraform

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform/config/configschema"

	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hcltest"

	"github.com/hashicorp/terraform/config"
)

func TestEvalInterpolate_impl(t *testing.T) {
	var _ EvalNode = new(EvalInterpolate)
}

func TestEvalInterpolate(t *testing.T) {
	config, err := config.NewRawConfig(map[string]interface{}{})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	var actual *ResourceConfig
	n := &EvalInterpolate{Config: config, Output: &actual}
	result := testResourceConfig(t, map[string]interface{}{})
	ctx := &MockEvalContext{InterpolateConfigResult: result}
	if _, err := n.Eval(ctx); err != nil {
		t.Fatalf("err: %s", err)
	}
	if actual != result {
		t.Fatalf("bad: %#v", actual)
	}

	if !ctx.InterpolateCalled {
		t.Fatal("should be called")
	}
	if !reflect.DeepEqual(ctx.InterpolateConfig, config) {
		t.Fatalf("bad: %#v", ctx.InterpolateConfig)
	}
}

func TestEvalInterpolateWithSchema(t *testing.T) {
	config := config.NewRawConfigHCL2(hcltest.MockBody(&hcl.BodyContent{}))

	schema := &configschema.Block{}

	var got *ResourceConfig
	n := &EvalInterpolate{
		Config: config,
		Output: &got,
		Schema: &schema,
	}
	result := testResourceConfig(t, map[string]interface{}{})
	ctx := &MockEvalContext{InterpolateConfigResult: result}
	if _, err := n.Eval(ctx); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !ctx.InterpolateCalled {
		t.Fatal("should be called")
	}
	if ctx.InterpolateConfig.Body != config.Body {
		t.Fatalf("wrong body passed to Interpolate: %#v", ctx.InterpolateConfig.Body)
	}
	if ctx.InterpolateSchema != schema {
		t.Fatalf("wrong schema passed to Interpolate: %#v", ctx.InterpolateSchema)
	}
}
