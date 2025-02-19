use anyhow::Error;
use s2s::establish_connection;

fn main() -> Result<(), Error> {
    let _ = establish_connection(":memory:")?;
    println!("connection to SQLite established");
    Ok(())
}
