use anyhow::Error;
use diesel::prelude::*;
use diesel::sqlite::SqliteConnection;
use s2s::{
    establish_connection,
    models::{BpfLink, BpfMap, BpfProgram, BpfProgramMap},
};

fn main() -> Result<(), Error> {
    let db_file = std::env::args().nth(1).unwrap_or_else(|| {
        eprintln!("Usage: <filename.db>");
        std::process::exit(1);
    });

    let mut conn = establish_connection(&db_file)?;

    let mut program = BpfProgram::default();
    program.name = "my_program".to_string();
    program.state = "pre_load".to_string();
    program.map_pin_path = "/tmp".to_string();
    program.program_bytes = vec![];

    // Insert the new program. (Here 42 is a kernel-assigned ID.)
    program = BpfProgram::insert_program(&mut conn, 42, program)?;
    println!("Inserted program: {:?}", program);

    // Update the program.
    program.programme_type = Some("foo".to_string());
    program = program.save_changes(&mut conn)?;
    println!("Updated program: {:?}", program);

    // Attach a link to the program.
    let mut link = BpfLink::default();
    link.link_type = Some("xdp".to_string());
    link.target = Some("eth0".to_string());
    link.state = "pre_attach".to_string();
    link = BpfLink::link_insert(&mut conn, link.id(), link)?;
    println!("Created link {:?}", link);

    link = program.attach_link(&mut conn, 100, link)?;
    println!("Attached link {} to program {}", link.id(), link.program_id);

    // Attach 10 maps to the program.
    for i in 0..10 {
        let mut map = BpfMap::default();
        map.name = format!("example_map_{}", i);
        map.map_type = Some("hash".to_string());
        map.key_size = Some(4);
        map.value_size = Some(8);
        map.max_entries = Some(1024);
        // Use a unique kernel_map_id per iteration.
        let kernel_map_id: i64 = (u32::MAX - i) as i64;
        let attached_map = program.attach_map(&mut conn, map, kernel_map_id)?;
        println!("Attached map {}: {:?}", i, attached_map);
    }

    // Query and print programs.
    let programs = query_programs(&mut conn)?;
    println!("{} bpf_program(s)", programs.len());
    for prog in &programs {
        println!("Program ID: {}", prog.id());
        println!("Program updated_at: {}", prog.updated_at());
        println!("Program type: {:?}", prog.programme_type);
        println!("Program state: {}", prog.state);
        println!("-----------------------------------");
    }

    // Query and print links.
    let links = query_links(&mut conn)?;
    println!("{} bpf_link(s)", links.len());
    for link in &links {
        println!("Link ID: {}", link.id());
        println!("Link program_id: {}", link.program_id);
        println!("Link type: {:?}", link.link_type);
        println!("Link target: {:?}", link.target);
        println!("Link state: {}", link.state);
        println!("-----------------------------------");
    }

    // Query and print maps.
    let maps = query_maps(&mut conn)?;
    println!("{} bpf_map(s)", maps.len());
    for map in &maps {
        println!("Map ID: {}", map.id());
        println!("Map name: {}", map.name);
        println!("Map type: {:?}", map.map_type);
        println!("Key size: {:?}", map.key_size);
        println!("Value size: {:?}", map.value_size);
        println!("Max entries: {:?}", map.max_entries);
        println!("-----------------------------------");
    }

    // Query and print associations.
    let associations = query_associations(&mut conn)?;
    println!("{} bpf_program_map(s)", associations.len());
    for assoc in &associations {
        println!(
            "Association: Program {} is associated with Map {}",
            assoc.program_id, assoc.map_id
        );
    }

    Ok(())
}

/// Query all programs using the local DSL import.
fn query_programs(conn: &mut SqliteConnection) -> QueryResult<Vec<BpfProgram>> {
    use s2s::schema::bpf_programs::dsl::*;
    bpf_programs.select(BpfProgram::as_select()).load(conn)
}

/// Query all links using the local DSL import.
fn query_links(conn: &mut SqliteConnection) -> QueryResult<Vec<BpfLink>> {
    use s2s::schema::bpf_links::dsl::*;
    bpf_links.select(BpfLink::as_select()).load(conn)
}

/// Query all maps using the local DSL import.
fn query_maps(conn: &mut SqliteConnection) -> QueryResult<Vec<BpfMap>> {
    use s2s::schema::bpf_maps::dsl::*;
    bpf_maps.select(BpfMap::as_select()).load(conn)
}

/// Query all program-map associations using the local DSL import.
fn query_associations(conn: &mut SqliteConnection) -> QueryResult<Vec<BpfProgramMap>> {
    use s2s::schema::bpf_program_maps::dsl::*;
    bpf_program_maps
        .select(BpfProgramMap::as_select())
        .load(conn)
}
