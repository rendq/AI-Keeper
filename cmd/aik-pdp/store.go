package main

import (
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

// newInMemoryStore creates an OPA in-memory store from a data map.
func newInMemoryStore(data map[string]interface{}) storage.Store {
	return inmem.NewFromObject(data)
}
