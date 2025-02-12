use std::{fs, path::Path, process::Command};

fn main() {
    // Tell Cargo to rerun if anything in the migrations directory changes.
    println!("cargo:rerun-if-changed=migrations/");

    // Run "diesel print-schema"
    let output = Command::new("diesel")
        .args(&["print-schema"])
        .output()
        .expect("Failed to run diesel print-schema");

    if !output.status.success() {
        panic!(
            "diesel print-schema failed: {}",
            String::from_utf8_lossy(&output.stderr)
        );
    }

    let new_schema = output.stdout;
    let schema_path = Path::new("src/schema.rs");

    // Only write if the file doesn't exist or its contents differ.
    let write_schema = match fs::read(schema_path) {
        Ok(existing) => existing != new_schema,
        Err(_) => true,
    };

    if write_schema {
        fs::write(schema_path, &new_schema)
            .expect("Failed to write generated schema to src/schema.rs");
        println!("Generated new schema.");
    }
}
