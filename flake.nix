{
  description = "Run Lua function from nats subjects";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      version = "0.1.7";
      vendorHash = "sha256-K5VF5qTrJ3Ia+f/X19xKLRgwcVjMmYqiIO3Ncgz+Vz4=";

      mkCli =
        pkgs:
        pkgs.buildGoModule {
          pname = "msgscript-cli";
          inherit version vendorHash;

          src = self;

          subPackages = [ "cmd/cli" ];

          postInstall = ''
            mv $out/bin/cli $out/bin/msgscriptcli
          '';
        };

      mkServer =
        pkgs:
        pkgs.buildGoModule {
          pname = "msgscript";
          inherit version vendorHash;

          src = self;

          subPackages = [ "cmd/server" ];

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

      mkOverlay = system: {
        default = (
          final: prev: {
            msgscript-cli = self.packages.${system}.cli;
            msgscript-server = self.packages.${system}.server;
          }
        );
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
      packages.aarch64-linux = rec {
        cli = mkCli "x86_64-linux";
        server = mkServer "x86_64-linux";
        default = server;
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

            # Server compose
            arion

            # LSPs
            gopls
            lua-language-server
          ];
        };

      overlays = {
        aarch64-linux = mkOverlay "aarch64-linux";
        x86_64-linux = mkOverlay "x86_64-linux";
      };

      nixosModules.default = import ./nix/modules/default.nix;
    };
}
