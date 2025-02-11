use anyhow::Context;
use rusqlite::{params, Connection};
use serde::{Deserialize, Serialize};
use std::env;
use std::fs::File;
use std::io::BufReader;

/// Represents a single BPF program. The 'map_ids' field is used to
/// store references to the maps this program uses.
#[derive(Debug, Serialize, Deserialize)]
struct BPFProgram {
    id: i32,
    name: Option<String>,
    pinned: Option<String>,
    #[serde(rename = "type")]
    prog_type: String,
    run_time_ns: Option<i64>,
    run_cnt: Option<i64>,
    /// If bpftool includes "map_ids", they can be stored here.
    map_ids: Option<Vec<i32>>,
}

/// Represents a single BPF map. We store its actual bpftool ID in
/// 'id', so it matches the bridging table references.
#[derive(Debug, Serialize, Deserialize)]
struct BPFMap {
    id: i32,
    name: String,
    pinned: Option<String>,
    #[serde(rename = "type")]
    map_type: String,
    key_size: Option<i32>,
    value_size: Option<i32>,
    max_entries: Option<i32>,
    bytes_used: Option<i64>,
    bytes_limit: Option<i64>,
}

/// Represents a single BPF link. It references the program ID via
/// 'prog_id' so that we can insert it properly into BPFLink.
#[derive(Debug, Serialize, Deserialize)]
struct BPFLink {
    id: i32,
    prog_id: i32,
    pinned: Option<String>,
    target: Option<String>,
    attach_type: Option<String>,
}

/// Loads JSON from a file into a Vec<T>.
fn load_json<T: for<'de> Deserialize<'de>>(filename: &str) -> anyhow::Result<Vec<T>> {
    let file =
        File::open(filename).with_context(|| format!("Failed to open file: {}", filename))?;
    let reader = BufReader::new(file);
    let data: Vec<T> = serde_json::from_reader(reader)
        .with_context(|| format!("Failed to parse JSON from file: {}", filename))?;
    Ok(data)
}

fn insert_programs(conn: &Connection, programs: &[BPFProgram]) -> anyhow::Result<()> {
    let mut stmt = conn.prepare(
        "INSERT OR REPLACE INTO BPFProgram
         (id, name, path, type, run_time_ns, run_cnt)
         VALUES (?, ?, ?, ?, ?, ?);",
    )?;

    let txn = conn.unchecked_transaction()?;

    for prog in programs {
        let prog_name = prog
            .name
            .clone()
            .unwrap_or_else(|| format!("unknown_program_{}", prog.id));

        stmt.execute(params![
            prog.id,
            prog_name,
            prog.pinned.as_deref(),
            prog.prog_type,
            prog.run_time_ns.unwrap_or(0),
            prog.run_cnt.unwrap_or(0),
        ])
        .with_context(|| format!("Failed to insert BPFProgram record (id={})", prog.id))?;
    }

    txn.commit()?;
    Ok(())
}

fn insert_maps(conn: &Connection, maps: &[BPFMap]) -> anyhow::Result<()> {
    let mut stmt = conn.prepare(
        "INSERT OR REPLACE INTO BPFMap
         (id, name, path, type,
          key_size, value_size, max_entries,
          bytes_used, bytes_limit)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);",
    )?;

    let txn = conn.unchecked_transaction()?;

    for map in maps {
        let ksize = map.key_size.unwrap_or(0);
        let vsize = map.value_size.unwrap_or(0);
        let maxe = map.max_entries.unwrap_or(0);

        stmt.execute(params![
            map.id,
            map.name,
            map.pinned.as_deref(),
            map.map_type,
            ksize,
            vsize,
            maxe,
            map.bytes_used.unwrap_or(0),
            map.bytes_limit.unwrap_or(0),
        ])
        .with_context(|| format!("Failed to insert BPFMap record (id={})", map.id))?;
    }

    txn.commit()?;
    Ok(())
}

fn insert_prog_map(conn: &Connection, programs: &[BPFProgram]) -> anyhow::Result<()> {
    let mut stmt = conn.prepare(
        "INSERT OR IGNORE INTO BPFProgramMap (program_id, map_id)
         VALUES (?, ?);",
    )?;

    let txn = conn.unchecked_transaction()?;

    for prog in programs {
        if let Some(ref map_ids) = prog.map_ids {
            for &map_id in map_ids {
                stmt.execute(params![prog.id, map_id]).with_context(|| {
                    format!(
                        "Failed to insert bridging record (prog_id={}, map_id={})",
                        prog.id, map_id
                    )
                })?;
            }
        }
    }

    txn.commit()?;
    Ok(())
}

fn insert_links(conn: &Connection, links: &[BPFLink]) -> anyhow::Result<()> {
    let mut stmt = conn.prepare(
        "INSERT OR REPLACE INTO BPFLink
         (id, program_id, path, event, attach_type)
         VALUES (?, ?, ?, ?, ?);",
    )?;

    let txn = conn.unchecked_transaction()?;

    for link in links {
        stmt.execute(params![
            link.id,
            link.prog_id,
            link.pinned.as_deref(),
            link.target,
            link.attach_type.as_deref(),
        ])
        .with_context(|| format!("Failed to insert BPFLink record (id={})", link.id))?;
    }

    txn.commit()?;
    Ok(())
}

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = env::args().collect();
    if args.len() != 5 {
        eprintln!(
            "Usage: {} <db> <bpf-programs.json> <bpf-maps.json> <bpf-links.json>",
            args[0]
        );
        std::process::exit(1);
    }

    let db_path = &args[1];
    let prog_json_path = &args[2];
    let map_json_path = &args[3];
    let link_json_path = &args[4];

    let programs: Vec<BPFProgram> = load_json(prog_json_path)?;
    let maps: Vec<BPFMap> = load_json(map_json_path)?;
    let links: Vec<BPFLink> = load_json(link_json_path)?;

    let conn = Connection::open(db_path)?;

    insert_programs(&conn, &programs)?;
    insert_maps(&conn, &maps)?;
    insert_prog_map(&conn, &programs)?;
    insert_links(&conn, &links)?;

    Ok(())
}
