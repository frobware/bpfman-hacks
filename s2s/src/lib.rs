pub mod models;
pub mod schema;
pub mod u64blob;

use diesel::prelude::*;
use diesel::sqlite::SqliteConnection;
use diesel_migrations::{embed_migrations, EmbeddedMigrations, MigrationHarness};

pub const MIGRATIONS: EmbeddedMigrations = embed_migrations!("migrations");

#[derive(Debug)]
pub enum ConnectionError {
    Connection(diesel::ConnectionError),
    Migration(Box<dyn std::error::Error + Send + Sync>),
}

impl std::fmt::Display for ConnectionError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ConnectionError::Connection(e) => write!(f, "Database connection error: {}", e),
            ConnectionError::Migration(e) => write!(f, "Migration error: {}", e),
        }
    }
}

impl std::error::Error for ConnectionError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            ConnectionError::Connection(e) => Some(e),
            ConnectionError::Migration(e) => Some(e.as_ref()),
        }
    }
}

impl From<diesel::ConnectionError> for ConnectionError {
    fn from(err: diesel::ConnectionError) -> Self {
        ConnectionError::Connection(err)
    }
}

pub fn establish_connection(database_url: &str) -> Result<SqliteConnection, ConnectionError> {
    let mut connection = SqliteConnection::establish(database_url)?;

    let applied_migrations = connection
        .run_pending_migrations(MIGRATIONS)
        .map_err(ConnectionError::Migration)?;

    if applied_migrations.is_empty() {
        eprintln!("No new migrations were applied.");
    } else {
        eprintln!("Applied migrations:");
        for migration in applied_migrations {
            eprintln!("- {}", migration);
        }
    }

    Ok(connection)
}
