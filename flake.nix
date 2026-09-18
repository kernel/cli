{
  description = "Kernel CLI — cloud browser infrastructure for AI agents";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = "0.19.0";
      in {
        packages.default = pkgs.buildGoModule {
          pname = "kernel";
          inherit version;

          src = ./.;

          vendorHash = "sha256-l/r3aRCu3cViGmb7wt0GMwnwRwXytTo8C74zL6rAMlU=";

          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
          ];

          meta = with pkgs.lib; {
            description = "Kernel CLI — cloud browser infrastructure for AI agents";
            homepage = "https://www.kernel.sh";
            license = licenses.asl20;
            mainProgram = "kernel";
            platforms = platforms.unix;
          };
        };

        apps.default = flake-utils.lib.mkApp {
          drv = self.packages.${system}.default;
        };
      });
}
