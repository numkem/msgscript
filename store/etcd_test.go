package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestEtcdScriptStoreListSubjects(t *testing.T) {
	store, err := NewEtcdScriptStore("localhost:2379")
	assert.Nil(t, err)

	subjects, err := store.ListSubjects(context.Background())
	assert.Nil(t, err)
	assert.NotEmpty(t, subjects)
}
