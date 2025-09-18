{
  description = "Run Lua function from nats subjects";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      version = "0.5.0";
      vendorHash = "sha256-LcgdJIsn3/fHv3NGvGdfq/Y3N7CTuIH/b5Rv5tEMUg8=";

      mkCli =
        pkgs:
        let
          wasmtime = pkgs.callPackage ./nix/pkgs/wasmtime.nix { };
        in
        pkgs.buildGoModule {
          pname = "msgscript-cli";
          inherit version vendorHash;

          src = self;

          subPackages = [ "cmd/cli" ];

          nativeBuildInputs = [ pkgs.pkg-config ];

          buildInputs = [
            wasmtime.dev
            pkgs.btrfs-progs
            pkgs.gpgme
          ];

          postInstall = ''
            mv $out/bin/cli $out/bin/msgscriptcli
          '';
        };

      mkServer =
        pkgs:
        let
          wasmtime = pkgs.callPackage ./nix/pkgs/wasmtime.nix { };
        in
        pkgs.buildGoModule {
          pname = "msgscript";
          inherit version vendorHash;

          src = self;

          subPackages = [ "cmd/server" ];

          nativeBuildInputs = [ pkgs.pkg-config ];

          buildInputs = [
            wasmtime.dev
            pkgs.btrfs-progs
            pkgs.gpgme
          ];

          doCheck = false; # Requires networking, will just timeout

          postInstall = ''
            mv $out/bin/server $out/bin/msgscript
          '';
        };

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
          cli = mkCli pkgs;
          server = mkServer pkgs;
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
          cli = mkCli "x86_64-linux";
          server = mkServer "x86_64-linux";
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
        };

      overlays = final: prev: {
        msgscript-cli = self.packages.${final.system}.cli;
        msgscript-server = self.packages.${final.system}.server;
      };

      nixosModules.default = import ./nix/modules/default.nix;
    };
}
