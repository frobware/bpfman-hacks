{
  description = "QEMU development environment for bpfman";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
  flake-utils.lib.eachDefaultSystem (system:
  let
    pkgs = nixpkgs.legacyPackages.${system};

    scriptDeps = with pkgs; [
      qemu
      virtiofsd
      cdrkit          # provides genisoimage
      util-linux      # provides lscpu
      coreutils       # provides realpath
      cloud-utils     # cloud-init tools
      openssh         # ssh client
    ];

    bpfman-dev-qemu = pkgs.writeShellApplication {
      name = "bpfman-dev-qemu";
      runtimeInputs = scriptDeps;
      text = builtins.readFile ./bpfman-dev-qemu;
    };

    bpfman-cleanup-integration-test = pkgs.writeShellApplication {
      name = "bpfman-cleanup-integration-test";
      runtimeInputs = scriptDeps;
      text = builtins.readFile ./bpfman-cleanup-integration-test;
    };

    bpfman-gen-config = pkgs.writeShellApplication {
      name = "bpfman-gen-config";
      runtimeInputs = scriptDeps;
      text = builtins.readFile ./bpfman-gen-config;
    };
  in
  {
    packages = {
      inherit bpfman-dev-qemu bpfman-cleanup-integration-test bpfman-gen-config;
      default = bpfman-dev-qemu;
    };

    apps = {
      bpfman-dev-qemu = flake-utils.lib.mkApp { drv = bpfman-dev-qemu; };
      bpfman-cleanup-integration-test = flake-utils.lib.mkApp { drv = bpfman-cleanup-integration-test; };
      bpfman-gen-config = flake-utils.lib.mkApp { drv = bpfman-gen-config; };
      default = self.apps.${system}.bpfman-dev-qemu;
    };

    devShells.default = pkgs.mkShell {
      buildInputs = scriptDeps;

      shellHook = ''
        echo "QEMU development environment loaded"
        echo "Available tools:"
        echo "  - qemu-system-x86_64 (VM execution)"
        echo "  - virtiofsd (VirtIO filesystem daemon)"
        echo "  - genisoimage (ISO creation)"
        echo "  - lscpu, realpath (system utilities)"
        echo "  - cloud-localds (cloud-init utilities)"
        echo ""
        echo "Available scripts:"
        echo "  - bpfman-dev-qemu"
        echo "  - bpfman-cleanup-integration-test"
        echo "  - bpfman-gen-config"
      '';
    };
  }
  );
}
