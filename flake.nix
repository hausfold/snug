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
          # Pinned, never null. `null` makes buildGoModule fetch the module
          # graph at build time, which works on a laptop with network and is a
          # hard failure in a sandboxed/CI build. Re-take it from the mismatch
          # error whenever go.sum changes.
          vendorHash = "sha256-iiJJ2y22zizvc4tVcaDYBuHrmF45hWH68Zt//KoMvyw=";

          # Build only the CLI. The library is compiled as its dependency, which
          # is the only thing this derivation has to COMPILE.
          subPackages = [ "cmd/snug" ];

          # …but not the only thing it installs. `share/ui.sh` is the bash half
          # of the same spec — the painter for a machine that cannot see
          # `bin/snug`: an older generation, a launchd job off a thin PATH, a
          # checkout on a Mac that never installed the layer. It ships INSIDE
          # this derivation so a consumer reads it straight off the store path
          # (`${snug}/share/ui.sh`) with no vendored copy and no drift check —
          # which is the whole reason it lives in this repo rather than in the
          # workshop, where `haus.sh` (read into a store binary, on a machine
          # with no workshop checkout) could never have sourced it.
          #
          # Not a `passthru`: it has to survive being the ONLY thing a caller
          # takes, so it goes in $out beside the binary it stands in for.
          postInstall = ''
            install -Dm444 share/ui.sh $out/share/ui.sh
          '';

          meta = with pkgs.lib; {
            description = "Terminal presentation for the hausfold family";
            homepage = "https://github.com/hausfold/snug";
            license = licenses.asl20;
            mainProgram = "snug";
            platforms = platforms.unix;
          };
        };
      });

      # The overlay consumers take, so `pkgs.snug` is the CLI wherever this
      # flake is an input — the same shape pounce, perch, trill and hausfold/holt
      # ship, which is what lets a haus module put it on PATH without knowing it
      # came from a separate flake. (haus calls that last input `scruff`; the
      # repo keeps the old name until its own 1.0.0. Input names and repo names
      # are different questions.) The Go PACKAGE needs none of this: a Go
      # consumer imports `github.com/hausfold/snug` and never sees Nix.
      #
      # ⚠️ `self.packages` is built from THIS flake's nixpkgs, so `final` here
      # supplies the system string and nothing else. A consumer that doesn't set
      # `inputs.snug.inputs.nixpkgs.follows` evaluates and realises a second
      # nixpkgs and a second Go toolchain for one 2 MB binary. haus sets it.
      overlays.default = final: _prev: {
        snug = self.packages.${final.stdenv.hostPlatform.system}.default;
      };

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
