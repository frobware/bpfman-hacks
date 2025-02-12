-- Table for BPF Programs.
--
-- A BPF program is the central object loaded into the kernel. The
-- kernel assigns each program a unique 32-bit (u32) ID. In SQLite,
-- declaring the id as an INTEGER PRIMARY KEY makes it an alias for
-- the rowid. This table stores metadata about the program along with
-- the actual program binary in a BLOB.
CREATE TABLE bpf_programs (
    id INTEGER PRIMARY KEY NOT NULL,  -- Kernel's BPF program ID (u32, alias for rowid)
    name TEXT NOT NULL,
    description TEXT,
    programme_type TEXT,
    state TEXT NOT NULL,  -- Expected values: 'pre_load' or 'loaded'
    location_filename TEXT,
    location_url TEXT,
    location_image_pull_policy TEXT,
    location_username TEXT,
    location_password TEXT,
    map_owner_id INTEGER,
    map_pin_path TEXT NOT NULL,
    program_bytes BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Table for BPF Links.
--
-- A BPF link represents a specific attachment of a program to a
-- target (e.g. a network interface, cgroup, etc.). Although a program
-- may create several links (e.g. attaching to multiple interfaces),
-- each link is an independent object associated with exactly one
-- program. Therefore, we model this as a one-to-many relationship:
-- each link row includes a foreign key referencing its owning
-- program.
CREATE TABLE bpf_links (
    id INTEGER PRIMARY KEY NOT NULL,
    program_id BIGINT NOT NULL REFERENCES bpf_programs(id) ON DELETE CASCADE,
    link_type TEXT,
    target TEXT,
    state TEXT NOT NULL,  -- Expected values: 'pre_attach' or 'attached'
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Table for BPF Maps.
--
-- A BPF map stores key-value data that a BPF program may use. A
-- single map can be shared among multiple programs. For this reason,
-- we separate the identity of a map from its association with a
-- program. The bpf_maps table stores one record per unique map (with
-- the kernel's map ID, which is a 32-bit unsigned integer stored as
-- an INTEGER PRIMARY KEY).
CREATE TABLE bpf_maps (
    id INTEGER PRIMARY KEY NOT NULL,  -- Kernel's BPF map ID (u32)
    name TEXT NOT NULL,
    map_type TEXT,
    key_size INTEGER,
    value_size INTEGER,
    max_entries INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Join Table for the Many-to-Many Relationship between Programs and
-- Maps. Because a single BPF map may be shared by multiple
-- programs—and a program may use multiple maps—we need an
-- intermediary table. Each row in this join table represents an
-- association between a program (via program_id) and a map (via
-- map_id). The composite primary key (program_id, map_id) ensures
-- that the association is unique. The ON DELETE CASCADE clauses
-- ensure that if either a program or a map is deleted, the
-- corresponding association is automatically removed.
CREATE TABLE bpf_program_maps (
    program_id BIGINT NOT NULL REFERENCES bpf_programs(id) ON DELETE CASCADE,
    map_id BIGINT NOT NULL REFERENCES bpf_maps(id) ON DELETE CASCADE,
    PRIMARY KEY (program_id, map_id)
);

-- Trigger for bpf_programs.
CREATE TRIGGER update_bpf_programs_updated_at
AFTER UPDATE ON bpf_programs
FOR EACH ROW
WHEN NEW.updated_at = OLD.updated_at
BEGIN
  UPDATE bpf_programs
  SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now')
  WHERE id = NEW.id;
END;

-- Trigger for bpf_links.
CREATE TRIGGER update_bpf_links_updated_at
AFTER UPDATE ON bpf_links
FOR EACH ROW
WHEN NEW.updated_at = OLD.updated_at
BEGIN
  UPDATE bpf_links
  SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now')
  WHERE id = NEW.id;
END;

-- Trigger for bpf_maps.
CREATE TRIGGER update_bpf_maps_updated_at
AFTER UPDATE ON bpf_maps
FOR EACH ROW
WHEN NEW.updated_at = OLD.updated_at
BEGIN
  UPDATE bpf_maps
  SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now')
  WHERE id = NEW.id;
END;
