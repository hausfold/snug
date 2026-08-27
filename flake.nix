{
  description = "snug — how the hausfold family puts a line on screen";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ./VERSION);
    in
    {
      packages = forAll (pkgs: {
        default = pkgs.buildGoModule {
          pname = "snug";
          inherit version;
          src = ./.;
          vendorHash = null; # filled by the first build; see AGENTS.md

          # Build only the CLI. The library is compiled as its dependency, which
          # is the only thing this derivation has to install.
          subPackages = [ "cmd/snug" ];

          meta = with pkgs.lib; {
            description = "Terminal presentation for the hausfold family";
            homepage = "https://github.com/hausfold/snug";
            license = licenses.asl20;
            mainProgram = "snug";
            platforms = platforms.unix;
          };
        };
      });

      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.gotools
          ];
        };
      });

      formatter = forAll (pkgs: pkgs.nixfmt-rfc-style);
    };
}
