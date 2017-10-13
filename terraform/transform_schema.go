package terraform

import "log"

// GraphNodeAttachSchema is implemented by nodes that want to have an
// opportunity to inspect schemas during graph construction.
//
// The main reason to implement this interface is if a node represents an
// object that has its own schema, in order to obtain that schema and
// store it for later use. For example, objects that have configuration that
// may contain interpolations need their schema in order to implement
// GraphNodeReferencer.
//
// The node is given a pointer to the entire schema repository, but it will
// generally only save the portion of it that is most relevant. Implementations
// of this interface are not permitted to modify the given schema repository.
type GraphNodeAttachSchema interface {
	AttachSchema(schemas *Schemas)
}

// AttachSchemaTransformer passes the given schema repository to all nodes
// that implement GraphNodeAttachSchema.
//
// Since the main use for schemas on graph nodes is to implement
// GraphNodeReferencer, this transformer should appear _before_
// ReferenceTransformer in the list of graph transform steps.
type AttachSchemaTransformer struct {
	Schemas *Schemas
}

var _ GraphTransformer = AttachSchemaTransformer{}

func (t AttachSchemaTransformer) Transform(g *Graph) error {
	if t.Schemas == nil {
		log.Printf("[WARNING] AttachSchemaTransformer run with nil schema repository")
		return nil
	}

	log.Printf("[TRACE] AttachSchemaTransformer starting")
	defer func() {
		log.Printf("[TRACE] AttachSchemaTransformer complete")
	}()

	type Named interface {
		Name() string
	}

	for _, v := range g.Vertices() {
		asn, wantsSchema := v.(GraphNodeAttachSchema)
		if !wantsSchema {
			continue
		}

		if named, hasName := v.(Named); hasName {
			log.Printf("[TRACE] Offering schemas to %T %s", v, named.Name())
		} else {
			log.Printf("[TRACE] Offering schemas to unnamed %T", v)
		}

		asn.AttachSchema(t.Schemas)
	}

	return nil
}
