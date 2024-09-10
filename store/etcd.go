package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	ETCD_TIMEOUT = 3 * time.Second
)

// EtcdScriptStore stores Lua scripts in etcd, supporting multiple scripts per subject
type EtcdScriptStore struct {
	client *clientv3.Client
	prefix string
}

func etcdEndpoints(endpoints string) []string {
	return strings.Split(endpoints, ",")
}

// NewEtcdScriptStore creates a new instance of EtcdScriptStore
func NewEtcdScriptStore(url string) (*EtcdScriptStore, error) {
	log.Debugf("Attempting to connect to etcd @ %s", url)

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints(url),
		DialTimeout: ETCD_TIMEOUT,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %v", err)
	}

	log.Debugf("Connected to etcd @ %s", url)

	return &EtcdScriptStore{
		client: client,
		prefix: "msgscript/scripts/",
	}, nil
}

// AddScript adds a new Lua script under the given subject with a unique ID
func (e *EtcdScriptStore) AddScript(subject, name, script string) error {
	key := fmt.Sprintf("%s%s/%s", e.prefix, subject, name)

	ctx, cancel := context.WithTimeout(context.Background(), ETCD_TIMEOUT)
	defer cancel()

	// Store script in etcd
	_, err := e.client.Put(ctx, key, script)
	if err != nil {
		return fmt.Errorf("failed to add script for subject '%s': %v", subject, err)
	}

	log.Debugf("Script added for subject %s named %s", subject, name)
	return nil
}

// GetScripts retrieves all scripts associated with a subject
func (e *EtcdScriptStore) GetScripts(subject string) ([]string, error) {
	keyPrefix := fmt.Sprintf("%s%s/", e.prefix, subject)

	ctx, cancel := context.WithTimeout(context.Background(), ETCD_TIMEOUT)
	defer cancel()

	// Fetch all scripts under the subject's prefix
	resp, err := e.client.Get(ctx, keyPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get scripts for subject '%s': %v", subject, err)
	}

	var scripts []string
	for _, kv := range resp.Kvs {
		scripts = append(scripts, string(kv.Value))
	}

	log.Debugf("Retrieved %d scripts for subject %s", len(scripts), subject)
	return scripts, nil
}

// DeleteScript deletes a specific Lua script for a subject by its scriptID
func (e *EtcdScriptStore) DeleteScript(subject, scriptID string) error {
	key := fmt.Sprintf("%s%s/%s", e.prefix, subject, scriptID)

	ctx, cancel := context.WithTimeout(context.Background(), ETCD_TIMEOUT)
	defer cancel()

	// Delete script from etcd
	_, err := e.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete script for subject '%s' with ID '%s': %v", subject, scriptID, err)
	}

	log.Debugf("Deleted script for subject %s with ID %s", subject, scriptID)
	return nil
}

// WatchScripts watches for changes to scripts for a specific subject
func (e *EtcdScriptStore) WatchScripts(subject string, onChange func(subject, scriptID, script string, deleted bool)) {
	keyPrefix := fmt.Sprintf("%s%s/", e.prefix, subject)

	ctx, cancel := context.WithTimeout(context.Background(), ETCD_TIMEOUT)
	defer cancel()

	watchChan := e.client.Watch(ctx, keyPrefix, clientv3.WithPrefix())

	for watchResp := range watchChan {
		for _, ev := range watchResp.Events {
			scriptID := string(ev.Kv.Key[len(keyPrefix):])
			switch ev.Type {
			case clientv3.EventTypePut:
				script := string(ev.Kv.Value)
				log.Debugf("Script added/updated for subject: %s, ID: %s", subject, scriptID)
				onChange(subject, scriptID, script, false)
			case clientv3.EventTypeDelete:
				log.Debugf("Script deleted for subject: %s, ID: %s", subject, scriptID)
				onChange(subject, scriptID, "", true)
			}
		}
	}
}
