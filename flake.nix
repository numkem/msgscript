{
  description = "Run Lua function from nats subjects";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      version = "0.7.1";
      vendorHash = "sha256-IsLAIKYuKhlD71fad8FuayTFbdQJla4ifjs8TexXDYQ=";

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
    in
    {
      packages.x86_64-linux =
        let
          pkgs = import nixpkgs { system = "x86_64-linux"; };
          lib = pkgs.lib;
        in
        rec {
          cli = pkgs.callPackage ./nix/pkgs/cli.nix {
            inherit version vendorHash;
          };
          server = pkgs.callPackage ./nix/pkgs/server.nix {
            inherit version vendorHash;
          };
          default = server;

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
        };
      packages.aarch64-linux =
        let
          pkgs = import nixpkgs { system = "aarch64-linux"; };
          lib = pkgs.lib;
        in
        rec {
          cli = pkgs.callPackage ./nix/pkgs/cli.nix {
            inherit version vendorHash;
          };
          server = pkgs.callPackage ./nix/pkgs/server.nix {
            inherit version vendorHash;
          };
          default = server;

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
            arion

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
      };

      nixosModules.default = import ./nix/modules/default.nix;
    };
}
