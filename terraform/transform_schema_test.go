package terraform

import "testing"

func TestAttachSchemaTransformer(t *testing.T) {
	node := &mockAttachSchema{}
	g := &Graph{Path: RootModulePath}
	g.Add(node)

	schemas := &Schemas{}

	tr := &AttachSchemaTransformer{
		Schemas: schemas,
	}

	err := tr.Transform(g)
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}

	if node.Given != schemas {
		t.Errorf("Node was given schemas at %p; want %p", node.Given, schemas)
	}
}

type mockAttachSchema struct {
	Given *Schemas
}

func (m *mockAttachSchema) AttachSchema(schemas *Schemas) {
	m.Given = schemas
}
