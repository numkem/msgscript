package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/openbao/openbao/api"
	vaultapi "github.com/openbao/openbao/api" // Use truly opensource fork
	log "github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

const DEFAULT_VAULT_TIMEOUT = 5 * time.Second

func Preload(L *lua.LState) {
	L.PreloadModule("vault", func(L *lua.LState) int {
		mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
			"new": new,
		})
		L.Push(mod)

		mt := L.NewTypeMetatable("VaultKV")
		L.SetGlobal("VaultKV", mt)
		L.SetField(mt, "new", L.NewFunction(new))
		L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
			"read":   read,
			"write":  write,
			"delete": delete,
			"list":   list,
		}))

		return 1
	})
}

func new(L *lua.LState) int {
	v, err := newVaultKVLuaClient(L.CheckString(1), L.CheckString(2))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("failed to create new vault client: %v", err)))
		return 2
	}

	ud := L.NewUserData()
	ud.Value = v
	L.SetMetatable(ud, L.GetTypeMetatable("VaultKV"))

	L.Push(ud)
	L.Push(lua.LNil)
	return 2
}

type vaultKVLuaClient struct {
	client *vaultapi.Client
}

func newVaultKVLuaClient(address, token string) (*vaultKVLuaClient, error) {
	config := api.DefaultConfig()

	// Configure TLS to skip verification
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// Set the TLS config on the HTTP client
	config.HttpClient.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client, err := vaultapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to vault: %v", err)
	}
	client.SetToken(token)
	client.SetAddress(address)

	return &vaultKVLuaClient{client: client}, nil
}

func write(L *lua.LState) int {
	c := L.CheckUserData(1).Value.(*vaultKVLuaClient)
	path := L.CheckString(2)
	ldata := L.CheckTable(3)
	mountPath := L.CheckString(4)

	data := make(map[string]interface{})
	ldata.ForEach(func(k, v lua.LValue) {
		data[k.String()] = v.String()

		if k.Type().String() != "string" {
			log.WithField("plugin", "vault").Errorf("unsupported type in read data: %v", k.Type().String())
		}
	})

	_, err := c.client.Logical().Write(fmt.Sprintf("%s/data%s", mountPath, path), map[string]interface{}{"data": data})
	if err != nil {
		L.Push(lua.LString(fmt.Sprintf("failed to write to %s: %v", path, err)))
		return 1
	}

	L.Push(lua.LNil)
	return 1
}

func read(L *lua.LState) int {
	c := L.CheckUserData(1).Value.(*vaultKVLuaClient)
	path := L.CheckString(2)
	mountPath := L.CheckString(3)

	s, err := c.client.Logical().Read(fmt.Sprintf("%s/data%s", mountPath, path))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("failed to read path %s: %v", path, err)))
		return 2
	}

	if s == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("path %s not found", path)))
		return 2
	}

	t := L.NewTable()
	for k, v := range s.Data["data"].(map[string]interface{}) {
		t.RawSetString(k, lua.LString(v.(string)))
	}

	L.Push(t)
	L.Push(lua.LNil)
	return 2
}

func delete(L *lua.LState) int {
	c := L.CheckUserData(1).Value.(*vaultKVLuaClient)
	path := L.CheckString(2)
	mountPath := L.CheckString(3)

	_, err := c.client.Logical().Delete(fmt.Sprintf("%s/metadata%s", mountPath, path))
	if err != nil {
		L.Push(lua.LString(fmt.Sprintf("failed to delete %s: %v", path, err)))
		return 1
	}

	L.Push(lua.LNil)
	return 1
}

func list(L *lua.LState) int {
	c := L.CheckUserData(1).Value.(*vaultKVLuaClient)
	path := L.CheckString(2)
	mountPath := L.CheckString(3)

	s, err := c.client.Logical().List(fmt.Sprintf("%s/metadata%s", mountPath, path))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("failed to list %s: %v", path, err)))
		return 2
	}

	l := L.NewTable()
	for _, k := range s.Data["keys"].([]interface{}) {
		l.Append(lua.LString(k.(string)))
	}

	L.Push(l)
	L.Push(lua.LNil)
	return 2
}
