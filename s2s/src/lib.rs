pub mod models;
pub mod schema;
pub mod uintblob;

use diesel::{prelude::*, sqlite::SqliteConnection};
use diesel_migrations::{EmbeddedMigrations, MigrationHarness, embed_migrations};
use thiserror::Error;

const MIGRATIONS: EmbeddedMigrations = embed_migrations!("migrations");

#[derive(Debug, Error)]
pub enum ConnectionError {
    #[error("Database connection error: {0}")]
    Connection(#[from] diesel::ConnectionError),

    #[error("Migration error: {0}")]
    Migration(#[from] Box<dyn std::error::Error + Send + Sync>),
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
