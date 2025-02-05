{
  description = "Development shell for BPF tooling.";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
  };

  outputs = { self, nixpkgs, ... }:

  let
    systems = [ "x86_64-linux" "aarch64-linux" ];
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
  in {
    devShells = forAllSystems (system: let
      pkgs = (import nixpkgs { inherit system; });
    in {
      default = pkgs.mkShell {
        nativeBuildInputs = [
          pkgs.just

          pkgs.bcc
          pkgs.bpftools
          pkgs.elfutils
          pkgs.libbpf
          pkgs.linuxHeaders
          pkgs.llvmPackages.clang-unwrapped
        ];

        hardeningDisable = [
          "all"
        ];

        shellHook = ''
          export CC=${pkgs.llvmPackages.clang-unwrapped}/bin/clang
          export CFLAGS="-I${pkgs.linuxHeaders}/include -I${pkgs.libbpf}/include"
          echo $CFLAGS
        '';
      };
    });
  };
}
