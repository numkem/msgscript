package lua

import (
	"context"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	glua "github.com/yuin/gopher-lua"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/numkem/msgscript/store"
)

func testingEtcdClient() *clientv3.Client {
	client, err := store.EtcdClient("127.0.0.1:2379")
	if err != nil {
		log.Errorf("failed to connect to etcd: %w", err)
	}

	return client
}

func TestLuaEtcdPut(t *testing.T) {
	etcdKey := "msgscript/test/put"
	etcdValue := "foobar"

	luaScript := fmt.Sprintf(`
etcd = require("etcd")

local err = etcd.put("%s", "%s")
assert(err == nil)
`, etcdKey, etcdValue)

	L := glua.NewState()
	defer L.Close()

	PreloadEtcd(L)

	err := L.DoString(luaScript)
	assert.Nil(t, err)

	// Check that the key actually exists
	resp, err := testingEtcdClient().Get(context.Background(), etcdKey)
	assert.Nil(t, err)
	assert.Len(t, resp.Kvs, 1)
	assert.Equal(t, etcdValue, string(resp.Kvs[0].Value))

	teardown()
}

func TestLuaEtcdGet(t *testing.T) {
	etcdKey := "msgscript/test/get"
	etcdValue := "foobar"

	// Set the key
	client := testingEtcdClient()
	_, err := client.Put(context.Background(), etcdKey, etcdValue)
	assert.Nil(t, err)

	luaScript := fmt.Sprintf(`
etcd = require("etcd")

local resp, err = etcd.get("%s", false)
assert(err == nil)
return resp[1]:getValue()
`, etcdKey)

	L := glua.NewState()
	defer L.Close()

	PreloadEtcd(L)

	err = L.DoString(luaScript)
	assert.Nil(t, err)

	// Fetch the value from the lua script
	result := L.Get(-1)
	val, ok := result.(glua.LString)
	if !ok {
		assert.Fail(t, "script should return a string")
	}
	assert.Equal(t, etcdValue, val.String())

	teardown()
}

func TestLuaEtcdDelete(t *testing.T) {
	etcdKey := "msgscript/test/delete"
	etcdValue := "foobar"

	// Set a key to be deleted
	testingEtcdClient().Put(context.Background(), etcdKey, etcdValue)

	luaScript := fmt.Sprintf(`
etcd = require("etcd")

local err = etcd.delete("%s")
assert(err == nil)
`, etcdKey)

	L := glua.NewState()
	defer L.Close()

	PreloadEtcd(L)

	err := L.DoString(luaScript)
	assert.Nil(t, err)

	// Make sure the key is really gone
	resp, err := testingEtcdClient().Get(context.Background(), etcdKey)
	assert.Nil(t, err)
	assert.Empty(t, resp.Kvs)

	teardown()
}

func teardown() {
	_, err := testingEtcdClient().Delete(context.Background(), "msgscript/test", clientv3.WithPrefix())
	if err != nil {
		log.Fatalf("failed to delete etcd testing keys: %w", err)
	}
}
