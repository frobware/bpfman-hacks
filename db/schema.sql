PRAGMA foreign_keys = ON;

-- BPF Programs Table
CREATE TABLE IF NOT EXISTS BPFProgram (
    id INTEGER PRIMARY KEY,     -- The bpftool "prog_id"
    name TEXT NOT NULL,
    path TEXT,              -- Some programs may not have pinned paths
    type TEXT NOT NULL,
    run_time_ns INTEGER DEFAULT 0,
    run_cnt INTEGER DEFAULT 0
);

-- BPF Maps Table
CREATE TABLE IF NOT EXISTS BPFMap (
    id INTEGER PRIMARY KEY,     -- The bpftool "map_id"
    name TEXT NOT NULL,
    path TEXT,                  -- Some maps may not have pinned paths
    type TEXT NOT NULL,
    key_size INTEGER,
    value_size INTEGER,
    max_entries INTEGER CHECK (max_entries > 0),
    bytes_used INTEGER DEFAULT 0,
    bytes_limit INTEGER DEFAULT 0
);

-- Bridging Table for Program <-> Map relationships
CREATE TABLE IF NOT EXISTS BPFProgramMap (
    program_id INTEGER NOT NULL,
    map_id INTEGER NOT NULL,
    FOREIGN KEY (program_id) REFERENCES BPFProgram(id) ON DELETE CASCADE,
    FOREIGN KEY (map_id) REFERENCES BPFMap(id) ON DELETE CASCADE,
    PRIMARY KEY (program_id, map_id)
);

-- BPF Links Table
CREATE TABLE IF NOT EXISTS BPFLink (
    id INTEGER PRIMARY KEY,      -- bpftool "link_id"
    program_id INTEGER NOT NULL, -- Link belongs to one program
    path TEXT,                   -- Optional
    event TEXT,                  -- Called "target" in JSON
    attach_type TEXT,            -- Nullable
    FOREIGN KEY (program_id) REFERENCES BPFProgram(id) ON DELETE CASCADE
);
