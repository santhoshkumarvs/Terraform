package terraform

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform/config/configschema"
)

func TestContextProviderSchemas(t *testing.T) {
	fooP := testProvider("foo")
	barP := testProvider("bar")
	bazP := testProvider("baz")
	oldP := testProvider("old")
	m := testModule(t, "context-provider-schemas")
	ctx := testContext2(t, &ContextOpts{
		Module: m,
		ProviderResolver: ResourceProviderResolverFixed(
			map[string]ResourceProviderFactory{
				"foo": testProviderFuncFixed(fooP),
				"bar": testProviderFuncFixed(barP),
				"baz": testProviderFuncFixed(bazP),
				"old": testProviderFuncFixed(oldP),
			},
		),
	})

	fooP.ResourcesReturn = []ResourceType{{SchemaAvailable: true}}
	fooP.DataSourcesReturn = nil
	fooP.GetSchemaReturn = &ProviderSchema{
		Provider: &configschema.Block{},
	}

	barP.ResourcesReturn = nil
	barP.DataSourcesReturn = []DataSource{{SchemaAvailable: true}}
	barP.GetSchemaReturn = &ProviderSchema{
		Provider: &configschema.Block{},
	}

	bazP.ResourcesReturn = []ResourceType{{SchemaAvailable: true}}
	bazP.DataSourcesReturn = nil
	bazP.GetSchemaReturn = &ProviderSchema{
		Provider: &configschema.Block{},
	}

	oldP.GetSchemaReturnError = errors.New("not supported")

	got := ctx.providerSchemas()
	want := ProviderSchemas{
		"foo": fooP.GetSchemaReturn,
		"bar": barP.GetSchemaReturn,
		"baz": bazP.GetSchemaReturn,
		"old": &ProviderSchema{
			ResourceTypes: map[string]*configschema.Block{},
			DataSources:   map[string]*configschema.Block{},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrong result\ngot: %swant: %s", spew.Sdump(got), spew.Sdump(want))
	}

	{
		got := fooP.GetSchemaRequest
		want := &ProviderSchemaRequest{
			ResourceTypes: []string{}, // only the provider schema is needed
			DataSources:   []string{},
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("wrong request for 'foo'\ngot: %swant: %s", spew.Sdump(got), spew.Sdump(want))
		}
	}
	{
		got := barP.GetSchemaRequest
		want := &ProviderSchemaRequest{
			ResourceTypes: []string{"bar_bar", "foo_bar"}, // foo_bar has "provider" set to override to "bar"
			DataSources:   []string{},
		}
		sort.Strings(got.ResourceTypes)

		if !reflect.DeepEqual(got, want) {
			t.Errorf("wrong request for 'bar'\ngot: %swant: %s", spew.Sdump(got), spew.Sdump(want))
		}
	}
	{
		got := bazP.GetSchemaRequest
		want := &ProviderSchemaRequest{
			ResourceTypes: []string{},
			DataSources:   []string{"baz_bar"},
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("wrong request for 'baz'\ngot: %swant: %s", spew.Sdump(got), spew.Sdump(want))
		}
	}

	if oldP.GetSchemaCalled {
		t.Errorf("GetSchema was called on 'old'; should not have been")
	}

	if !fooP.CloseCalled {
		t.Errorf("Provider 'foo' was not closed; should have been")
	}

}
