# Database Summary

<details>
<summary>Stats (click to expand)</summary>

```
Kernel Programs: 4311 key-value pairs
Maps: 14 key-value pairs
Programs: 397 key-value pairs
STORE: 30 key-value pairs
Traffic Control Dispatchers: 9 key-value pairs
XDP Dispatchers: 7 key-value pairs
```
</details>

<details>
<summary>Kernel Programs</summary>

```
KernelProgram:100
  id: 100
  kernel_btf_id: 0
  kernel_bytes_jited: 201
  kernel_bytes_memlock: 4096
  kernel_bytes_xlated: 312
  kernel_gpl_compatible: true
  kernel_jited: true
  kernel_loaded_at: 2025-01-28T08:59:12+0000
  kernel_map_ids_0: 87
  kernel_name: sd_fw_egress
  kernel_program_type: BPF_PROG_TYPE_CGROUP_SKB
  kernel_tag: 7dc8126e8768ea37
  kernel_verified_insns: 35
```
(etc...)
</details>

<details>
<summary>Maps</summary>

```
Map:885
  map_used_by_0: 885

Map:886
  map_used_by_0: 886
```
(etc...)
</details>

<details>
<summary>Programs</summary>

```
Program:885
  id: 885
  kernel_btf_id: 257
  kernel_bytes_jited: 124
  kernel_bytes_memlock: 4096
  kernel_bytes_xlated: 176
  kernel_gpl_compatible: true
  kernel_jited: true
  kernel_loaded_at: 2025-01-28T18:03:34+0000
  kernel_map_ids_0: 553
  kernel_name: stats
  kernel_program_type: BPF_PROG_TYPE_EXT
```
(etc...)
</details>

<details>
<summary>STORE</summary>

```
IMAGES
  quay.io_bpfman-bytecode_go-app-counter_latest24c28fb6352d8024fe8cb5c5bbe9c16b30dac3e47a317a1831a63d4cfb6d99cd: {
    "architecture": "amd64",
    "config": {
      "Env": [
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
      ],
```
(etc...)
</details>

<details>
<summary>Traffic Control Dispatchers</summary>

```
TrafficControlDispatcher:4026533525/10/ingress/2
  direction: 1
  handle: 2
  if_index: 10
  if_name: 812151909
  nsid: 4026533525
  num_extension: 2
  priority: 2
  program_name: tc_dispatcher
  revision: 2
```
</details>

<details>
<summary>XDP Dispatchers</summary>

```
XDPDispatcher:4026533525/10/3
  if_index: 10
  if_name: 812151909
  mode: 1
  nsid: 4026533525
  num_extension: 3
  program_name: xdp_dispatcher
  revision: 3
```
</details>
