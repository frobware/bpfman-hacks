// @generated automatically by Diesel CLI.

diesel::table! {
    bpf_links (id) {
        id -> BigInt,
        program_id -> BigInt,
        link_type -> Nullable<Text>,
        target -> Nullable<Text>,
        state -> Text,
        created_at -> Timestamp,
        updated_at -> Timestamp,
    }
}

diesel::table! {
    bpf_maps (id) {
        id -> BigInt,
        name -> Text,
        map_type -> Nullable<Text>,
        key_size -> Nullable<Integer>,
        value_size -> Nullable<Integer>,
        max_entries -> Nullable<Integer>,
        created_at -> Timestamp,
        updated_at -> Timestamp,
    }
}

diesel::table! {
    bpf_program_maps (program_id, map_id) {
        program_id -> BigInt,
        map_id -> BigInt,
    }
}

diesel::table! {
    bpf_programs (id) {
        id -> BigInt,
        name -> Text,
        description -> Nullable<Text>,
        programme_type -> Nullable<Text>,
        state -> Text,
        location_filename -> Nullable<Text>,
        location_url -> Nullable<Text>,
        location_image_pull_policy -> Nullable<Text>,
        location_username -> Nullable<Text>,
        location_password -> Nullable<Text>,
        map_owner_id -> Nullable<Integer>,
        map_pin_path -> Text,
        program_bytes -> Binary,
        created_at -> Timestamp,
        updated_at -> Timestamp,
    }
}

diesel::joinable!(bpf_links -> bpf_programs (program_id));
diesel::joinable!(bpf_program_maps -> bpf_maps (map_id));
diesel::joinable!(bpf_program_maps -> bpf_programs (program_id));

diesel::allow_tables_to_appear_in_same_query!(
    bpf_links,
    bpf_maps,
    bpf_program_maps,
    bpf_programs,
);
