# bpfman SLED Database Explorer / Dumper

[ This tool is unlikely to have any use beyond my own curiosity on or around Wed 29 Jan 16:09:13 GMT 2025. ]

I created this tool because I needed to understand what bpfman was
storing in its SLED key-value database. Rather than manually
inspecting raw bytes, I wrote a "schema inferrer" that organises the
data into logical categories and attempts to make sense of the values
based on their structure.

The tool automatically identifies different types of entries
(Programmes, Maps, Traffic Control Dispatchers, etc.), decodes binary
data intelligently, and presents everything in a hierarchical format
that's easy to explore.

## Features
- Automatic categorisation of database entries based on key prefixes
- Smart decoding of various value types:
  - 32-bit and 64-bit integers
  - UTF-8 strings
  - JSON structures
  - Boolean values
  - Binary data (displayed as hex)
- Human-readable BPF programme type conversion
- Hierarchical output format
- Database summary statistics

## Usage
```bash
cargo run -- </path/to/sled/db>
```

Sample [output](sample-output.md). Full [database dump](sample-output.txt)

## Data Categories
The tool organises data into the following categories:
- **Programmes**: Entries with prefix `program_`
- **Maps**: Entries with prefix `map_`
- **Traffic Control Dispatchers**: Entries with prefix `tc_dispatcher_`
- **XDP Dispatchers**: Entries with prefix `xdp_dispatcher_`
- **Kernel Programmes**: Entries with numeric-only names
- **Store**: Special SLED system entries
- **Miscellaneous**: Any other entries

## Value Interpretation
The tool attempts to intelligently interpret values based on their size and content:
- 4-byte values: Interpreted as 32-bit integers
- 8-byte values: Interpreted as 64-bit integers
- Other values: Attempted to be parsed as:
  1. UTF-8 strings
  2. JSON data
  3. Boolean values (0x00/0x01)
  4. Fallback to hex representation

## Dependencies
- `sled`: For database access
- `serde`: For JSON serialisation/deserialisation
- `serde_json`: For JSON processing

## Building
```bash
cargo build --release
```
