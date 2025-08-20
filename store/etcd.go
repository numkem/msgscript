package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	ETCD_TIMEOUT            = 3 * time.Second
	ETCD_SESSION_TTL        = 3 // In seconds
	ETCD_SCRIPT_KEY_PREFIX  = "msgscript/scripts"
	ETCD_LIBRARY_KEY_PREFIX = "msgscript/libs"
)

// EtcdScriptStore stores Lua scripts in etcd, supporting multiple scripts per subject
type EtcdScriptStore struct {
	client  *clientv3.Client
	prefix  string
	mutexes sync.Map
}

func EtcdClient(endpoints string) (*clientv3.Client, error) {
	if e := os.Getenv("ETCD_ENDPOINTS"); e != "" {
		endpoints = e
	}

	// HACK: instead of using a global or carry over the variable everywhere,
	// we set the environment variable if it's not defined
	if os.Getenv("ETCD_ENDPOINTS") == "" {
		err := os.Setenv("ETCD_ENDPOINTS", endpoints)
		if err != nil {
			return nil, fmt.Errorf("failed to set ETCD_ENDPOINTS environment variable")
		}
	}

	return clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints(endpoints),
		DialTimeout: ETCD_TIMEOUT,
	})
}

func etcdEndpoints(endpoints string) []string {
	return strings.Split(endpoints, ",")
}

// NewEtcdScriptStore creates a new instance of EtcdScriptStore
func NewEtcdScriptStore(endpoints string) (*EtcdScriptStore, error) {
	log.Debugf("Attempting to connect to etcd @ %s", endpoints)

	client, err := EtcdClient(endpoints)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	log.Debugf("Connected to etcd @ %s", strings.Join(client.Endpoints(), ","))

	return &EtcdScriptStore{
		client:  client,
		prefix:  ETCD_SCRIPT_KEY_PREFIX,
		mutexes: sync.Map{},
	}, nil
}

func (e *EtcdScriptStore) getKey(subject, name string) string {
	return strings.Join([]string{e.prefix, subject, name}, "/")
}

// AddScript adds a new Lua script under the given subject with a unique ID
func (e *EtcdScriptStore) AddScript(ctx context.Context, subject, name string, script []byte) error {
	key := e.getKey(subject, name)

	// Store script in etcd
	_, err := e.client.Put(ctx, key, string(script))
	if err != nil {
		return fmt.Errorf("failed to add script for subject '%s': %w", subject, err)
	}

	log.Debugf("Script added for subject %s named %s", subject, name)
	return nil
}

// GetScripts retrieves all scripts associated with a subject
func (e *EtcdScriptStore) GetScripts(ctx context.Context, subject string) (map[string][]byte, error) {
	keyPrefix := strings.Join([]string{e.prefix, subject}, "/")

	// Fetch all scripts under the subject's prefix
	resp, err := e.client.Get(ctx, keyPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get scripts for subject '%s': %w", subject, err)
	}

	scripts := make(map[string][]byte)
	for _, kv := range resp.Kvs {
		scripts[string(kv.Key)] = kv.Value
	}

	log.Debugf("Retrieved %d scripts for subject %s", len(scripts), subject)
	return scripts, nil
}

// DeleteScript deletes a specific Lua script for a subject by its name
func (e *EtcdScriptStore) DeleteScript(ctx context.Context, subject, name string) error {
	key := fmt.Sprintf("%s/%s/%s", e.prefix, subject, name)

	// Delete script from etcd
	_, err := e.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete script for subject '%s' with ID '%s': %w", subject, name, err)
	}

	log.Debugf("Deleted script for subject %s with ID %s", subject, name)
	return nil
}

// WatchScripts watches for changes to scripts for a specific subject
func (e *EtcdScriptStore) WatchScripts(ctx context.Context, subject string, onChange func(subject, name string, script []byte, deleted bool)) {
	keyPrefix := fmt.Sprintf("%s/%s/", e.prefix, subject)

	watchChan := e.client.Watch(ctx, keyPrefix, clientv3.WithPrefix())

	for watchResp := range watchChan {
		for _, ev := range watchResp.Events {
			name := string(ev.Kv.Key[len(keyPrefix):])
			switch ev.Type {
			case clientv3.EventTypePut:
				log.Debugf("Script added/updated for subject: %s, ID: %s", subject, name)
				onChange(subject, name, ev.Kv.Value, false)
			case clientv3.EventTypeDelete:
				log.Debugf("Script deleted for subject: %s, ID: %s", subject, name)
				onChange(subject, name, nil, true)
			}
		}
	}
}

func (e *EtcdScriptStore) acquireLock(ctx context.Context, lockKey string, ttl int) (*concurrency.Mutex, error) {
	// Create a lease
	sess, err := concurrency.NewSession(e.client, concurrency.WithTTL(ttl), concurrency.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	fields := log.Fields{
		"lockKey": lockKey,
	}
	log.WithFields(fields).Debugf("etcdStore: Acquiring lock")

	l := concurrency.NewMutex(sess, lockKey)
	err = l.TryLock(ctx)
	if err != nil {
		if err == context.Canceled {
			return nil, concurrency.ErrLocked
		}

		return nil, err
	}

	log.WithFields(fields).Debug("etcdStore: Acquired lock")

	return l, nil
}

func (e *EtcdScriptStore) ReleaseLock(ctx context.Context, path string) error {
	fields := log.Fields{
		"path": path,
	}
	v, ok := e.mutexes.Load(path)
	if !ok {
		// We don't have a lock for that path
		log.WithFields(fields).Debug("etcdStore: failed to find a locking mutex for timer")
		return nil
	}

	l := v.(*lock)
	err := l.Mutex.Unlock(ctx)
	if err != nil {
		return fmt.Errorf("etcdStore: failed to release lock: %w", err)
	}
	log.WithFields(fields).Debug("etcdStore: Released the lock")

	// Stop the timer
	l.Timer.Stop()
	e.mutexes.Delete(path)

	return nil
}

type lock struct {
	Mutex *concurrency.Mutex
	Timer *time.Timer
}

func (e *EtcdScriptStore) TakeLock(ctx context.Context, path string) (bool, error) {
	lockKey := path + "_lock"
	mu, err := e.acquireLock(ctx, lockKey, ETCD_SESSION_TTL)
	if err != nil {
		if err == concurrency.ErrLocked {
			return false, fmt.Errorf("already locked")
		}

		return false, fmt.Errorf("failed to get lock on key %s: %w", lockKey, err)
	}

	// Remove the mutex from the map after 1 second more than the session's TTL in case it's never unlocked
	timer := time.AfterFunc((ETCD_SESSION_TTL+1)*time.Second, func() {
		log.WithField("path", path).Debug("Releasing lock on timeout")
		e.ReleaseLock(context.Background(), lockKey)
	})

	e.mutexes.Store(path, &lock{
		Mutex: mu,
		Timer: timer,
	})

	return true, nil
}

func (e *EtcdScriptStore) ListSubjects(ctx context.Context) ([]string, error) {
	resp, err := e.client.KV.Get(ctx, ETCD_SCRIPT_KEY_PREFIX, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	var subjects []string
	for _, kv := range resp.Kvs {
		ss := strings.Split(strings.Replace(string(kv.Key), ETCD_SCRIPT_KEY_PREFIX, "", 1), "/")
		subjects = append(subjects, ss[1])
	}

	return subjects, nil
}

func (e *EtcdScriptStore) LoadLibrairies(ctx context.Context, libraryPaths []string) ([][]byte, error) {
	var libraries [][]byte
	for _, path := range libraryPaths {
		key := strings.Join([]string{ETCD_LIBRARY_KEY_PREFIX, path}, "/")

		resp, err := e.client.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to read key %s: %w", key, err)
		}

		if len(resp.Kvs) != 1 {
			return nil, fmt.Errorf("key %s doesn't exists", key)
		}

		libraries = append(libraries, resp.Kvs[0].Value)
	}

	return libraries, nil
}

func (e *EtcdScriptStore) AddLibrary(ctx context.Context, content []byte, path string) error {
	key := strings.Join([]string{ETCD_LIBRARY_KEY_PREFIX, path}, "/")
	_, err := e.client.Put(ctx, key, string(content))
	if err != nil {
		return fmt.Errorf("failed to store library key %s: %w", key, err)
	}

	return nil
}

func (e *EtcdScriptStore) RemoveLibrary(ctx context.Context, path string) error {
	key := strings.Join([]string{ETCD_LIBRARY_KEY_PREFIX, path}, "/")
	_, err := e.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete library key %s: %w", key, err)
	}

	return nil
}
