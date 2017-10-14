package terraform

import (
	"log"
	"strings"

	"github.com/hashicorp/terraform/config"
	"github.com/hashicorp/terraform/config/configschema"
	"github.com/hashicorp/terraform/config/module"
)

// schemas returns a structure containing the schemas for providers,
// resource types, data sources and provisioners that appear in the
// configuration.
//
// If the configuration contains references to things that don't actually
// exist, they are omitted or nil in the returned maps on the assumption
// that later checks will detect such errors before we attempt to
// use the schemas.
//
// The result may currently also omit schemas for objects that _do_ exist, if
// the provider/provisioner in question is not new enough to support schemas.
// Consumers of the result must therefore be prepared for certain objects to
// have no schema at all.
//
// This method is not concurrency-safe, and so it should not be called during
// graph walks.
func (c *Context) schemas() *Schemas {
	if c.schemasCache != nil {
		return c.schemasCache
	}

	result := &Schemas{
		Providers: c.providerSchemas(),
	}

	c.schemasCache = result
	return result
}

// providerSchemas returns a structure containing the schemas for provider and
// resource types that appear in the configuration.
//
// If the configuration contains references to provider or resource types
// that don't actually exist, they are omitted from the returned map on the
// assumption that later checks will detect such errors before we attempt to
// use the schemas.
//
// The result may currently also omit schemas for objects that _do_ exist, if
// the provider in question is not new enough to support schemas. Consumers
// of the result must therefore be prepared for certain objects to have no
// schema at all.
//
// This method is not concurrency-safe, and so it should not be called during
// graph walks.
func (c *Context) providerSchemas() ProviderSchemas {
	result := ProviderSchemas{}

	// Here we analyze the configuration to see which providers, resource types
	// and data sources we need configuration schemas for.
	// First we'll fill out the result structure with some stubs that represent
	// what we need to fill in using data from the provider.
	findNeededProviderSchemas(c.Module(), result)

	close := func(name string, provider ResourceProvider) {
		if p, ok := provider.(ResourceProviderCloser); ok {
			log.Printf("[TRACE] closing provider %q after schema fetch", name)
			p.Close()
		}
	}

	// Now for each of the discovered providers we'll instantiate it and ask
	// it for its schema information. Currently we create throwaway instances
	// of the providers here because we don't wish to disturb the graph-driven
	// provider initialization used elsewhere; in future we may try to cache
	// these instances to avoid launching multiple times.
	for name, stubSchema := range result {
		log.Printf("[TRACE] fetching schema for provider %q...", name)
		provider, err := c.components.ResourceProvider(name, "<schemafetch>")
		if err != nil {
			// ignore missing providers; we'll catch them in later validation
			log.Printf("[WARNING] provider %q failed to start up: %s", name, err)
			continue
		}

		// Not all providers support schemas yet. That's okay, since we'll
		// catch that later if the user tries to use a schema-requiring feature
		// on an old provider.
		if !providerSupportsSchema(provider) {
			log.Printf("[WARNING] provider %q does not support schema", name)
			close(name, provider)
			continue
		}

		resourceTypeNames := make([]string, 0, len(stubSchema.ResourceTypes))
		for rn := range stubSchema.ResourceTypes {
			resourceTypeNames = append(resourceTypeNames, rn)
		}
		dataSourceNames := make([]string, 0, len(stubSchema.DataSources))
		for rn := range stubSchema.DataSources {
			dataSourceNames = append(dataSourceNames, rn)
		}

		schema, err := provider.GetSchema(&ProviderSchemaRequest{
			ResourceTypes: resourceTypeNames,
			DataSources:   dataSourceNames,
		})
		if err != nil {
			// For now we will ignore this, since only experimental features
			// require schema anyway and so we'd rather not raise errors that
			// don't matter to most users.
			log.Printf("[WARNING] failed to load schema for provider %q: %s", name, err)
			close(name, provider)
			continue
		}

		result[name] = schema
		log.Printf("[TRACE] successfully fetched schema for provider %q", name)
		close(name, provider)
	}

	return result
}

// findNeededProviderSchemas analyses the given module and all of its
// descendents for provider, managed resource and data resource configs
// that need schema to decode, populating the given ProviderSchemas structure
// with stubs (filling the maps with keys pointing to nil) that a caller
// must then fill in with actual schemas.
//
// This method is not able to check for the validity of provider and resource
// references, so it will return everything mentioned in the config even if
// it's nonsensical.
func findNeededProviderSchemas(mod *module.Tree, sch ProviderSchemas) {
	if mod == nil {
		return
	}

	cfg := mod.Config()

	for _, providerCfg := range cfg.ProviderConfigs {
		if _, exists := sch[providerCfg.Name]; !exists {
			sch[providerCfg.Name] = &ProviderSchema{
				ResourceTypes: map[string]*configschema.Block{},
				DataSources:   map[string]*configschema.Block{},
			}
		}
	}

	for _, resourceCfg := range cfg.Resources {
		providerName := resourceCfg.ProviderFullName()

		// providerName may be something like aws.foo, in which case we
		// only care about the "aws" portion.
		if idx := strings.Index(providerName, "."); idx > -1 {
			providerName = providerName[:idx]
		}

		if _, exists := sch[providerName]; !exists {
			sch[providerName] = &ProviderSchema{
				ResourceTypes: map[string]*configschema.Block{},
				DataSources:   map[string]*configschema.Block{},
			}
		}

		switch resourceCfg.Mode {
		case config.ManagedResourceMode:
			sch[providerName].ResourceTypes[resourceCfg.Type] = nil
		case config.DataResourceMode:
			sch[providerName].DataSources[resourceCfg.Type] = nil
		}
	}

	for _, childMod := range mod.Children() {
		findNeededProviderSchemas(childMod, sch)
	}
}

func providerSupportsSchema(provider ResourceProvider) bool {
	// Since the "ProviderSchema" function was added to ResourceProvider
	// without a change to the provider plugin protocol version, we must
	// sniff for support of this new feature, which we do by verifying
	// that at least one resource or data source has the SchemaAvailable
	// flag set. This weird sniffing protocol is designed to work within
	// the pre-existing set of methods so a breaking change could be avoided.
	if resources := provider.Resources(); len(resources) > 0 {
		return resources[0].SchemaAvailable
	}
	if dataSources := provider.DataSources(); len(dataSources) > 0 {
		return dataSources[0].SchemaAvailable
	}

	// (a provider with no resources or data sources can't support schema
	// per this sniffing approach, but an empty provider would be useless anyway.)

	return false
}

// providerSchemaKey takes a provider name as returned by node.ProvidedBy
// and returns the key that should be used to look up items from its
// schema.
func providerSchemaKey(name string) string {
	// We may be given an alias form, like "aws.foo". It's the "aws" prefix
	// we actually care about here, since we require that all instances of
	// the same provider have the same schema.
	if idx := strings.IndexRune(name, '.'); idx >= 0 {
		return name[:idx]
	}
	return name
}
