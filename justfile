run-etcd: worker
    DEBUG=1 go run ./cmd/server/ -library ./examples/libs -script ./examples/ -backend etcd -etcdurl localhost:2379 -natsurl localhost:4222

run: worker
    DEBUG=1 go run ./cmd/server/ -library ./examples/libs -script ./examples/

all-plugins:
    nix build .#allPlugins -o all_plugins

worker:
    go build ./cmd/worker
