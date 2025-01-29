{
  description = "A Rust development shell.";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    rust-overlay.url = "github:oxalica/rust-overlay";
  };

  outputs = { self, nixpkgs, rust-overlay, ... }:
  let
    supportedSystems = [
      "x86_64-linux"
      "aarch64-darwin"
    ];

    forAllSystems = f: nixpkgs.lib.genAttrs supportedSystems (system: f system);

    mkDevShell = system: let
      overlays = [ (import rust-overlay) ];
      pkgs = import nixpkgs {
        inherit system overlays;
      };

      stableToolchain = pkgs.rust-bin.stable.latest.default.override {
        extensions = [
          "rust-src"
          "rust-analyzer"
          "clippy"
          "rustfmt"
          "llvm-tools-preview"
        ];
        targets = [
          "x86_64-unknown-linux-gnu"
          "wasm32-unknown-unknown"
          "aarch64-apple-darwin"
        ];
      };

      nightlyRustfmt = pkgs.rust-bin.nightly.latest.rustfmt;

      darwinOnly = pkgs.lib.optionals pkgs.stdenv.isDarwin;
    in
    with pkgs; mkShell {
      buildInputs = [
        stableToolchain
        nightlyRustfmt # This installs rustfmt from nightly

        # Build essentials.
        clang
        cmake
        llvmPackages.libclang
        mold                # Fast linker
        ninja
        pkg-config

        # Development tools.
        cargo-audit         # Security audit
        cargo-expand        # Macro expansion
        cargo-tarpaulin     # Code coverage
        cargo-watch         # Auto-rebuild
        gdb
        lldb
        rust-analyzer
        valgrind

        # Libraries.
        openssl
        openssl.dev
        sqlite
      ] ++ (darwinOnly [
        pkgs.darwin.apple_sdk.frameworks.Security
        pkgs.darwin.apple_sdk.frameworks.CoreFoundation
        pkgs.darwin.apple_sdk.frameworks.SystemConfiguration
      ]);

      shellHook = ''
        if [[ "$(uname)" == "Darwin" ]]; then
        export RUSTFLAGS="-C link-arg=-fuse-ld=/usr/bin/ld"
        else
        export RUSTFLAGS="-C link-arg=-fuse-ld=mold"
        fi

        # For bindgen.
        export LIBCLANG_PATH="${pkgs.libclang.lib}/lib"

        # For rust-openssl.
        export PKG_CONFIG_PATH="${pkgs.openssl.dev}/lib/pkgconfig"

        # For better backtraces.
        export RUST_BACKTRACE=1

        # Ensure rustfmt is used from nightly
        export PATH="${nightlyRustfmt}/bin:$PATH"

        echo "🦀🦀🦀 Welcome to a Rust development shell 🦀🦀🦀"
        echo "Rust version: $(rustc --version)"
        echo "Nightly rustfmt version: $(rustfmt --version)"
      '';
    };
  in {
    devShells = forAllSystems (system: {
      default = mkDevShell system;
    });
  };
}
