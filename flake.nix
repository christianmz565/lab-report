{
  description = "UNSAReport CLI";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
        ldPath =
          with pkgs;
          lib.makeLibraryPath [
            stdenv.cc.cc
            zlib
            glib
            libxcb
            libglvnd
          ];
      in
      {
        packages.default = pkgs.buildGoModule rec {
          pname = "unsarep";
          version = "1.0.1";
          src = ./.;
          subPackages = [ "cmd/unsarep" ];

          vendorHash = "sha256-O7XiqeKIKCuv8zugr7JGwE5yR//ghgkFf8tkjIXZENM=";

          nativeBuildInputs = [
            pkgs.pkg-config
            pkgs.makeWrapper
          ];

          buildInputs = [ pkgs.fontconfig ];

          postInstall = ''
            wrapProgram $out/bin/unsarep \
              --prefix PATH : ${
                pkgs.lib.makeBinPath [
                  pkgs.typst
                  pkgs.charm-freeze
                  pkgs.imagemagick
                ]
              }
          '';

          ldflags = [
            "-s"
            "-w"
            "-X" "github.com/UNSAReport/UNSAReport/internal/ports.Version=${version}"
          ];

          meta = with pkgs.lib; {
            description = "A CLI tool for generating lab reports from markdown files.";
            homepage = "https://github.com/UNSAReport/UNSAReport";
            license = licenses.mit;
            platforms = platforms.unix ++ platforms.darwin ++ platforms.windows;
          };
        };
        apps.default = flake-utils.lib.mkApp {
          drv = self.packages.${system}.default;
        };

        devShells.default = pkgs.mkShell {
          LD_LIBRARY_PATH = ldPath;

          packages = with pkgs; [
            go
            gopls
            gotools
            golangci-lint

            typst
            tinymist

            fontconfig
            charm-freeze
            imagemagick

            pnpm
            nodejs
          ];

          buildInputs = [ pkgs.bashInteractive ];

          env = {
            GOPROXY = "https://proxy.golang.org,direct";
            GOSUMDB = "sum.golang.org";
          };

          shellHook = ''
            export GOPATH="$PWD/.go"
            export GOBIN="$GOPATH/bin"

            mkdir -p "$GOBIN"
            export PATH="$GOBIN:$PATH"
          '';
        };
      }
    );
}
