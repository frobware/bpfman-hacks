use std::collections::{BTreeMap};
use serde::Serialize;
use sled;
use serde_json::{ser::PrettyFormatter, ser::CompactFormatter, Serializer};

static COMPACT_JSON: bool = false;

fn main() -> sled::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 2 {
        eprintln!("Usage: {} <database-path>", args[0]);
        std::process::exit(1);
    }

    let db = sled::open(&args[1])?;
    let mut tree_groups: BTreeMap<String, BTreeMap<String, sled::Tree>> = BTreeMap::new();

    for tree_name in db.tree_names() {
        let tree_name_str = String::from_utf8(tree_name.to_vec())
            .unwrap_or_else(|_| "unknown".to_string());

        let (category, subpath) = if let Some(id) = tree_name_str.strip_prefix("program_") {
            ("Programs", format!("Program:{}", id))
        } else if let Some(id) = tree_name_str.strip_prefix("map_") {
            ("Maps", format!("Map:{}", id))
        } else if let Some(id) = tree_name_str.strip_prefix("tc_dispatcher_") {
            let structured_path = id.replace('_', "/");
            ("Traffic Control Dispatchers", format!("TrafficControlDispatcher:{}", structured_path))
        } else if let Some(id) = tree_name_str.strip_prefix("xdp_dispatcher_") {
            let structured_path = id.replace('_', "/");
            ("XDP Dispatchers", format!("XDPDispatcher:{}", structured_path))
        } else if tree_name_str == "__sled__default" {
            ("STORE", "IMAGES".to_string())
        } else if tree_name_str.chars().all(char::is_numeric) {
            ("Kernel Programs", format!("KernelProgram:{}", tree_name_str))
        } else {
            // what did I miss? How much do I not grok? (Lots...)
            ("Miscellaneous", format!("Misc:{}", tree_name_str))
        };

        let tree = db.open_tree(&tree_name_str)?;
        tree_groups.entry(category.to_string()).or_default().insert(subpath, tree);
    }

    println!("\nDatabase Summary:");
    for (category, trees) in &tree_groups {
        let pair_count: usize = trees.values().map(|tree| tree.iter().count()).sum();
        println!("{}: {} key-value pairs", category, pair_count);
    }

    for (category, trees) in &tree_groups {
        println!("\n{}:", category);
        for (subpath, tree) in trees {
            println!("  {}", subpath);
            print_tree_entries(tree, 4)?;
        }
    }

    Ok(())
}

/// Iterates over all key-value pairs in a tree and prints them hierarchically.
fn print_tree_entries(tree: &sled::Tree, indent: usize) -> sled::Result<()> {
    let mut key_values: Vec<(String, serde_json::Value)> = Vec::new();

    for item in tree.iter() {
        let (key, value) = item?;
        let key_str = String::from_utf8(key.to_vec()).unwrap_or_else(|_| format!("{:?}", key));
        let decoded_value = decode_value(&value);
        key_values.push((key_str, decoded_value));
    }

    key_values.sort_by(|a, b| a.0.cmp(&b.0));

    for (key, value) in key_values {
        if COMPACT_JSON {
            let compact_formatter = CompactFormatter;
            let formatted_value = format_value_as_string(&key, &value, 0, compact_formatter);
            let truncated_value = if formatted_value.len() > 100 {
                format!("{}...", &formatted_value[..100])
            } else {
                formatted_value
            };

            println!("{:indent$}{}: {}", "", key, truncated_value, indent = indent);
        } else {
            let formatted_value = format_value_as_string(&key, &value, 0, PrettyFormatter::default());
            print!("{:indent$}{}: ", "", key, indent = indent);

            let mut first_line = true;
            for line in formatted_value.lines() {
                if first_line {
                    println!("{}", line);
                    first_line = false;
                } else {
                    println!("{:indent$}{}", "", line, indent = indent + 4);
                }
            }
        }
    }

    Ok(())
}

fn format_value_as_string<F>(
    key: &str,
    value: &serde_json::Value,
    depth: usize,
    formatter: F,
) -> String
where
    F: serde_json::ser::Formatter,
{
    if depth > 5 {
        return "...".to_string();
    }

    match key {
        "kernel_program_type" => {
            if let serde_json::Value::Number(n) = value {
                let prog_type = n.as_i64().unwrap_or(-1);
                let prog_type_str = match prog_type {
                    0 => "BPF_PROG_TYPE_UNSPEC",
                    1 => "BPF_PROG_TYPE_SOCKET_FILTER",
                    2 => "BPF_PROG_TYPE_KPROBE",
                    3 => "BPF_PROG_TYPE_SCHED_CLS",
                    4 => "BPF_PROG_TYPE_SCHED_ACT",
                    5 => "BPF_PROG_TYPE_TRACEPOINT",
                    6 => "BPF_PROG_TYPE_XDP",
                    7 => "BPF_PROG_TYPE_PERF_EVENT",
                    8 => "BPF_PROG_TYPE_CGROUP_SKB",
                    9 => "BPF_PROG_TYPE_CGROUP_SOCK",
                    10 => "BPF_PROG_TYPE_LWT_IN",
                    11 => "BPF_PROG_TYPE_LWT_OUT",
                    12 => "BPF_PROG_TYPE_LWT_XMIT",
                    13 => "BPF_PROG_TYPE_SOCK_OPS",
                    14 => "BPF_PROG_TYPE_SK_SKB",
                    15 => "BPF_PROG_TYPE_CGROUP_DEVICE",
                    16 => "BPF_PROG_TYPE_SK_MSG",
                    17 => "BPF_PROG_TYPE_RAW_TRACEPOINT",
                    18 => "BPF_PROG_TYPE_CGROUP_SOCK_ADDR",
                    19 => "BPF_PROG_TYPE_LWT_SEG6LOCAL",
                    20 => "BPF_PROG_TYPE_LIRC_MODE2",
                    21 => "BPF_PROG_TYPE_SK_REUSEPORT",
                    22 => "BPF_PROG_TYPE_FLOW_DISSECTOR",
                    23 => "BPF_PROG_TYPE_CGROUP_SYSCTL",
                    24 => "BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE",
                    25 => "BPF_PROG_TYPE_CGROUP_SOCKOPT",
                    26 => "BPF_PROG_TYPE_TRACING",
                    27 => "BPF_PROG_TYPE_STRUCT_OPS",
                    28 => "BPF_PROG_TYPE_EXT",
                    29 => "BPF_PROG_TYPE_LSM",
                    30 => "BPF_PROG_TYPE_SK_LOOKUP",
                    31 => "BPF_PROG_TYPE_SYSCALL",
                    32 => "BPF_PROG_TYPE_NETFILTER",
                    _ => "Unknown",
                };
                return prog_type_str.to_string();
            }
        }
        _ => {}
    }

    match value {
        serde_json::Value::Object(obj) => {
            let mut output = Vec::new();
            let mut serializer = Serializer::with_formatter(&mut output, formatter);
            if obj.serialize(&mut serializer).is_ok() {
                String::from_utf8_lossy(&output).into()
            } else {
                format!("{}: INVALID_JSON", key)
            }
        }
        serde_json::Value::Array(arr) => {
            let truncated: Vec<String> = arr
                .iter()
                .take(10)
                .map(|v| match v {
                    serde_json::Value::Number(n) => format!("{:02X}", n.as_u64().unwrap_or(0)),
                    _ => "?".to_string(),
                })
                .collect();
            format!("[{} ...] ({} bytes)", truncated.join(" "), arr.len())
        }
        serde_json::Value::String(s) => s.to_string(),
        serde_json::Value::Bool(b) => b.to_string(),
        serde_json::Value::Number(n) => n.to_string(),
        serde_json::Value::Null => "null".to_string(),
    }
}

fn decode_value(value: &[u8]) -> serde_json::Value {
    match value.len() {
        4 => {
            // Decode as a 32-bit little-endian integer
            let int_value = i32::from_le_bytes(value.try_into().unwrap_or_default());
            serde_json::Value::Number(int_value.into())
        }
        8 => {
            // Decode as a 64-bit little-endian integer
            let int_value = i64::from_le_bytes(value.try_into().unwrap_or_default());
            serde_json::Value::Number(int_value.into())
        }
        _ => {
            // Attempt to decode as UTF-8
            if let Ok(utf8_value) = String::from_utf8(value.to_vec()) {
                if utf8_value == "\x01" {
                    return serde_json::Value::Bool(true);
                } else if utf8_value == "\x00" {
                    return serde_json::Value::Bool(false);
                }

                if let Ok(json_value) = serde_json::from_str::<serde_json::Value>(&utf8_value) {
                    return json_value;
                }

                return serde_json::Value::String(utf8_value);
            }

            serde_json::Value::Array(
                value
                    .iter()
                    .map(|&b| serde_json::Value::Number((b as u8).into()))
                    .collect(),
            )
        }
    }
}
