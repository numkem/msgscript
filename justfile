test-etcd:
    DEBUG=1 go run ./cmd/server/ -library ../msgscript-scripts/libs -script ../msgscript-scripts/ -backend etcd -etcdurl localhost:2379 -natsurl localhost:4222

all-plugins:
    nix build .#allPlugins -o all_plugins
