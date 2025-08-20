package lua

import (
	"context"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/yuin/gopher-lua"
	clientv3 "go.etcd.io/etcd/client/v3"

	msgstore "github.com/numkem/msgscript/store"
)

func PreloadEtcd(L *lua.LState) {
	L.PreloadModule("etcd", etcdLoader)
}

func etcdLoader(L *lua.LState) int {
	c, err := msgstore.EtcdClient("127.0.0.1:2379") // Set a default value
	if err != nil {
		log.WithField("endpoints", os.Getenv("ETCD_ENDPOINTS")).Errorf("failed to connect to etcd: %v", err)
	}
	l := luaEtcd{client: c}

	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"put":    l.Put,
		"get":    l.Get,
		"delete": l.Delete,
	})

	mt := L.NewTypeMetatable("EtcdKV")
	L.SetGlobal("EtcdKV", mt)
	L.SetField(mt, "new", L.NewFunction(newEtcdKV))
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"getKey":   luaEtcdKvGetKey,
		"getValue": luaEtcdKvGetValue,
	}))

	L.Push(mod)
	return 1
}

func luaEtcdKvGetKey(L *lua.LState) int {
	L.Push(lua.LString(L.CheckUserData(1).Value.(*luaEtcdKVs).Key))
	return 1
}

func luaEtcdKvGetValue(L *lua.LState) int {
	L.Push(lua.LString(L.CheckUserData(1).Value.(*luaEtcdKVs).Value))
	return 1
}

func newEtcdKV(L *lua.LState) int {
	key := L.CheckString(1)
	value := L.CheckString(2)
	kv := &luaEtcdKVs{
		Key:   key,
		Value: value,
	}

	ud := L.NewUserData()
	ud.Value = kv
	L.SetMetatable(ud, L.GetTypeMetatable("EtcdKV"))
	L.Push(ud)
	return 1
}

type luaEtcd struct {
	client *clientv3.Client
}

type luaEtcdKVs struct {
	Key   string
	Value string
}

func (l *luaEtcd) Get(L *lua.LState) int {
	key := L.CheckString(1)
	prefix := L.CheckBool(2)

	var resp *clientv3.GetResponse
	var err error
	if prefix {
		resp, err = l.client.Get(context.TODO(), key, clientv3.WithPrefix())
	} else {
		resp, err = l.client.Get(context.TODO(), key)
	}
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	if resp.Count == 0 {
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 2
	}

	kvTable := L.NewTable()
	for _, kv := range resp.Kvs {
		ud := L.NewUserData()
		ud.Value = &luaEtcdKVs{
			Key:   string(kv.Key),
			Value: string(kv.Value),
		}
		L.SetMetatable(ud, L.GetTypeMetatable("EtcdKV"))
		kvTable.Append(ud)
	}

	L.Push(kvTable)
	L.Push(lua.LNil)
	return 2
}

func (l *luaEtcd) Put(L *lua.LState) int {
	key := L.CheckString(1)
	value := L.CheckString(2)
	_, err := l.client.Put(context.TODO(), key, value)
	if err != nil {
		L.Push(lua.LString(err.Error()))
		return 1
	}

	L.Push(lua.LNil)
	return 1
}

func (l *luaEtcd) Delete(L *lua.LState) int {
	key := L.CheckString(1)
	_, err := l.client.Delete(context.TODO(), key)
	if err != nil {
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LNil)
	return 1
}
