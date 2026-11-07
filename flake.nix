{
  description = "goalie: checks that keep coding agents from adding poor code";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    (flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        goalie = pkgs.callPackage ./package.nix { };
      in
      {
        packages = {
          inherit goalie;
          default = goalie;
        };

        apps = {
          goalie = { type = "app"; program = "${goalie}/bin/goalie"; };
          godeps = { type = "app"; program = "${goalie}/bin/godeps"; };
        };

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.git ];
        };
      }
    )) // {
      overlays.default = final: prev: {
        goalie = final.callPackage ./package.nix { };
      };
    };
}
