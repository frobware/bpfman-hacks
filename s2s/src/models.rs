use crate::schema::bpf_programs::dsl::*;
use crate::schema::{bpf_links, bpf_maps, bpf_program_maps};
use chrono::{NaiveDateTime, Utc};
use diesel::prelude::*;

#[derive(Debug, AsChangeset, Insertable, Identifiable, Selectable, Queryable)]
#[diesel(table_name = crate::schema::bpf_programs)]
pub struct BpfProgram {
    id: i64, // PRIMARY KEY
    pub name: String,
    pub description: Option<String>,
    pub programme_type: Option<String>,
    pub state: String, // attached? use an enum? some other discriminator?
    pub location_filename: Option<String>,
    pub location_url: Option<String>,
    pub location_image_pull_policy: Option<String>,
    pub location_username: Option<String>,
    pub location_password: Option<String>,
    pub map_owner_id: Option<i32>,
    pub map_pin_path: String,
    pub program_bytes: Vec<u8>,
    created_at: NaiveDateTime,
    updated_at: NaiveDateTime,
}

#[derive(Debug, AsChangeset, Insertable, Identifiable, Selectable, Queryable)]
#[diesel(belongs_to(BpfProgram, foreign_key = program_id))]
#[diesel(table_name = crate::schema::bpf_links)]
pub struct BpfLink {
    id: i64, // PRIMARY KEY
    pub program_id: i64,
    pub link_type: Option<String>,
    pub target: Option<String>,
    pub state: String,
    created_at: NaiveDateTime,
    updated_at: NaiveDateTime,
}

#[derive(Debug, AsChangeset, Insertable, Identifiable, Selectable, Queryable)]
#[diesel(table_name = crate::schema::bpf_maps)]
pub struct BpfMap {
    id: i64, // PRIMARY KEY for Identifiable
    pub name: String,
    pub map_type: Option<String>,
    pub key_size: Option<i32>,
    pub value_size: Option<i32>,
    pub max_entries: Option<i32>,
    created_at: NaiveDateTime,
    updated_at: NaiveDateTime,
}

#[derive(Debug, Queryable, Selectable, Associations)]
#[diesel(belongs_to(BpfProgram, foreign_key = program_id))]
#[diesel(belongs_to(BpfMap, foreign_key = map_id))]
#[diesel(table_name = crate::schema::bpf_program_maps)]
pub struct BpfProgramMap {
    pub program_id: i64,
    pub map_id: i64,
}

impl BpfProgram {
    /// Delete a program by its ID. Due to ON DELETE CASCADE, this
    /// will also delete:
    ///
    /// - All associated links in bpf_links
    /// - All associated program-map relationships in bpf_program_maps
    pub fn delete_by_id(conn: &mut SqliteConnection, program_id: i64) -> QueryResult<usize> {
        diesel::delete(bpf_programs.find(program_id)).execute(conn)
    }

    /// Delete all programs from the database.
    /// Warning: This will delete all programs and their associated data!
    pub fn delete_all(conn: &mut SqliteConnection) -> QueryResult<usize> {
        diesel::delete(bpf_programs).execute(conn)
    }

    pub fn id(&self) -> i64 {
        self.id
    }

    pub fn created_at(&self) -> NaiveDateTime {
        self.created_at
    }

    pub fn updated_at(&self) -> NaiveDateTime {
        self.updated_at
    }

    pub fn insert_program(
        conn: &mut SqliteConnection,
        kernel_prog_id: i64,
        mut program: BpfProgram,
    ) -> QueryResult<BpfProgram> {
        program.id = kernel_prog_id;
        program.created_at = Utc::now().naive_utc();
        program.updated_at = program.created_at;

        diesel::insert_into(crate::schema::bpf_programs::table)
            .values(&program)
            .returning(BpfProgram::as_select())
            .get_result(conn)
    }

    pub fn attach_link(
        &mut self,
        conn: &mut SqliteConnection,
        kernel_link_id: i64,
        new_link: BpfLink,
    ) -> QueryResult<BpfLink> {
        conn.transaction(|conn| {
            let inserted_link: BpfLink = BpfLink::link_insert(conn, kernel_link_id, new_link)?;
            self.state = "attached".to_string();
            *self = self.save_changes(conn)?;

            Ok(inserted_link)
        })
    }

    /// Attaches a new BpfMap to this program by inserting the map and
    /// then creating the relationship in the join table.
    pub fn attach_map(
        &mut self,
        conn: &mut SqliteConnection,
        mut new_map: BpfMap,
        kernel_map_id: i64,
    ) -> QueryResult<BpfMap> {
        conn.transaction(|conn| {
            // Insert the new map; BpfMap::insert_map expects a
            // kernel_map_id of type i64.
            new_map = BpfMap::insert_map(conn, kernel_map_id, new_map)?;

            // Insert into the join table to associate the map with
            // this program.
            diesel::insert_into(bpf_program_maps::table)
                .values((
                    bpf_program_maps::program_id.eq(self.id),
                    bpf_program_maps::map_id.eq(new_map.id),
                ))
                .execute(conn)?;

            // other related properties...?
            // *self = self.save_changes(conn)?;

            Ok(new_map)
        })
    }
}

impl BpfMap {
    pub fn id(&self) -> i64 {
        self.id
    }

    pub fn created_at(&self) -> NaiveDateTime {
        self.created_at
    }

    pub fn updated_at(&self) -> NaiveDateTime {
        self.updated_at
    }

    pub fn insert_map(
        conn: &mut SqliteConnection,
        kernel_map_id: i64,
        mut map: BpfMap,
    ) -> QueryResult<BpfMap> {
        map.id = kernel_map_id;
        map.created_at = Utc::now().naive_utc();
        map.updated_at = map.created_at;

        diesel::insert_into(bpf_maps::table)
            .values(&map)
            .returning(BpfMap::as_select())
            .get_result(conn)
    }
}

impl BpfLink {
    pub fn id(&self) -> i64 {
        self.id
    }

    pub fn created_at(&self) -> NaiveDateTime {
        self.created_at
    }

    pub fn updated_at(&self) -> NaiveDateTime {
        self.updated_at
    }

    pub fn link_insert(
        conn: &mut SqliteConnection,
        kernel_link_id: i64,
        mut link: BpfLink,
    ) -> QueryResult<BpfLink> {
        link.id = kernel_link_id;
        link.created_at = Utc::now().naive_utc();
        link.updated_at = link.created_at;

        diesel::insert_into(bpf_links::table)
            .values(&link)
            .returning(BpfLink::as_select())
            .get_result(conn)
    }
}

impl Default for BpfProgram {
    fn default() -> Self {
        Self {
            id: 0, // Indicates an unsaved record
            name: String::new(),
            description: None,
            programme_type: None,
            state: "pre_load".to_string(), // Default initial state
            location_filename: None,
            location_url: None,
            location_image_pull_policy: None,
            location_username: None,
            location_password: None,
            map_owner_id: None,
            map_pin_path: String::new(),
            program_bytes: Vec::new(),
            created_at: Default::default(),
            updated_at: Default::default(),
        }
    }
}

impl Default for BpfLink {
    fn default() -> Self {
        Self {
            id: 0, // Indicates an unsaved record
            program_id: 0,
            link_type: None,
            target: None,
            state: "".to_string(),
            created_at: Default::default(),
            updated_at: Default::default(),
        }
    }
}

impl Default for BpfMap {
    fn default() -> Self {
        Self {
            id: 0, // Indicates an unsaved record
            name: "".to_string(),
            map_type: None,
            key_size: None,
            value_size: None,
            max_entries: None,
            created_at: Default::default(),
            updated_at: Default::default(),
        }
    }
}
