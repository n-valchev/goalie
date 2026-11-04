{
  description = "godry finds Go functions with the same structure";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    (flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          godry = pkgs.callPackage ./package.nix { };
          default = self.packages.${system}.godry;
        };

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.git ];
        };
      }
    )) // {
      overlays.default = final: prev: {
        godry = final.callPackage ./package.nix { };
      };
    };
}
