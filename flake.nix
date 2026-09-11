{
  description = "Run Lua function from nats subjects";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      version = "1.0.0";
      vendorHash = "sha256-Jer/ADurA+BwvAuuTjEycJNExBnobEs72ccQMmOqV1k=";

      mkPlugin =
        pkgs: name: path:
        pkgs.buildGoModule {
          name = "msgscript-plugin-${name}";

          inherit vendorHash;

          src = self;

          subPackages = [ path ];

          doUnpack = false;
          doCheck = false;

          buildPhase = ''
            go build -buildmode=plugin -o ${name}.so ${path}/main.go
          '';

          installPhase = ''
            mkdir $out
            cp ${name}.so $out/
          '';
        };

      mkPackages =
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          lib = pkgs.lib;

          mkTest =
            testFile:
            import testFile {
              inherit pkgs;
              modules = self.nixosModules.default;
              overlays = [ self.overlays.default ];
            };
        in
        rec {
          default = server;

          cli = pkgs.callPackage ./nix/pkgs/cli.nix {
            inherit version vendorHash;
          };

          server = pkgs.callPackage ./nix/pkgs/server.nix {
            inherit version vendorHash;
          };

          worker = pkgs.callPackage ./nix/pkgs/worker.nix {
            inherit version vendorHash;
          };

          runServer = pkgs.writeScript "msgscript" ''
            #!/usr/bin/env bash
            ${self.packages.${system}.server}/bin/msgscript -plugin ${allPlugins}/ -wexec ${self.packages.${system}.worker}/bin/worker $@
          '';

          runCli = pkgs.writeScript "msgscriptcli" ''
            #!/usr/bin/env bash
            ${self.packages.${system}.cli}/bin/msgscriptcli -plugin ${allPlugins}/ -wexec ${
              system.packages.${system}.worker
            }/bin/worker $@
          '';

          allPlugins = pkgs.symlinkJoin {
            name = "msgscript-all-plugins";
            paths = lib.attrValues plugins;
          };

          plugins =
            let
              pluginDirs = lib.remove "" (
                lib.mapAttrsToList (name: kind: if kind == "directory" then name else "") (
                  builtins.readDir "${self}/plugins/"
                )
              );
            in
            lib.genAttrs pluginDirs (name: mkPlugin pkgs name "${self}/plugins/${name}");

          # Tests
          test-minimal = mkTest ./nix/tests/minimal.nix;
          test-etcd = mkTest ./nix/tests/etcd.nix;
          test-lua-file = mkTest ./nix/tests/lua-file.nix;
        };
    in
    {
      packages = {
        "x86_64-linux" = mkPackages "x86_64-linux";
        "aarch64-linux" = mkPackages "aarch64-linux";
      };
      apps =
        let
          mkApps = system: {
            server = {
              type = "app";
              program = "${self.packages.${system}.runServer}";
            };

            cli = {
              type = "app";
              program = "${self.packages.${system}.runCli}";
            };
          };
        in
        {
          "x86_64-linux" = mkApps "x86_64-linux";
          "aarch64-linux" = mkApps "aarch64-linux";
        };

      devShells.x86_64-linux.default =
        let
          pkgs = import nixpkgs { system = "x86_64-linux"; };
        in
        pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            just
            etcd
            natscli
            nats-top
            pandoc

            # wasm
            tinygo
            wasmtime
            wasmtime.dev

            # Deps for podman
            pkg-config
            btrfs-progs
            gpgme

            # Server compose
            podman-compose

            # LSPs
            gopls
            lua-language-server
          ];

          shellHook = ''
            export GOOS=linux
            export GOARCH=amd64
          '';
        };

      overlays.default = final: prev: {
        msgscript-cli = self.packages.${final.system}.cli;
        msgscript-server = self.packages.${final.system}.server;
        msgscript-worker = self.packages.${final.system}.worker;
        msgscript-plugins = self.packages.${final.system}.plugins;
      };

      nixosModules.default = import ./nix/modules/default.nix;
    };
}
