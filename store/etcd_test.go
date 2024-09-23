package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEtcdScriptStoreListSubjects(t *testing.T) {
	store, err := NewEtcdScriptStore("localhost:2379")
	assert.Nil(t, err)

	subjects, err := store.ListSubjects(context.Background())
	assert.Nil(t, err)
	assert.NotEmpty(t, subjects)
}
