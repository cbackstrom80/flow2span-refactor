#!/usr/bin/env python3
"""
Synthetic NetFlow v5 generator for Flow2Span POC.

Models:
- 3 sites today, 72-site production-ready data model
- Client Communities and Server Communities
- Firewall + SD-WAN duplicate flow exporters
- Top talkers
- DNS traffic
- Site/link utilization pressure

Usage:
  ./scripts/setup_loopback_exporters.sh
  python3 scripts/generate_netflow_v5.py --target 127.0.0.1 --port 2055 --duration 120 --rate 8

Why loopback aliases?
  The NetFlow receiver often derives exporter identity from the UDP source IP.
  This script binds packets to 10.1.0.5, 10.1.0.1, etc. so the collector can
  classify exporters as firewall vs SD-WAN and deduplicate the same conversation.
"""

from __future__ import annotations

import argparse
import ipaddress
import random
import socket
import struct
import sys
import time
from dataclasses import dataclass
from typing import Dict, Iterable, List, Tuple

NETFLOW_VERSION = 5
MAX_RECORDS_PER_PACKET = 24

PROTO_TCP = 6
PROTO_UDP = 17
TCP_ACK_PUSH = 0x18
TCP_SYN_ACK = 0x12


@dataclass(frozen=True)
class Exporter:
    name: str
    ip: str
    role: str
    site: str
    priority: int


@dataclass(frozen=True)
class FlowTemplate:
    name: str
    src_ip: str
    dst_ip: str
    src_port_range: Tuple[int, int]
    dst_port: int
    proto: int
    packets_range: Tuple[int, int]
    bytes_per_packet_range: Tuple[int, int]
    tcp_flags: int
    dscp: int
    exporters: Tuple[str, ...]
    duplicate_jitter_percent: float
    weight: int


EXPORTERS: Dict[str, Exporter] = {
    "fw-site-001": Exporter("fw-site-001", "10.1.0.5", "firewall", "site-001", 100),
    "sdwan-site-001": Exporter("sdwan-site-001", "10.1.0.1", "sdwan_router", "site-001", 80),
    "fw-site-002": Exporter("fw-site-002", "10.2.0.5", "firewall", "site-002", 100),
    "sdwan-site-002": Exporter("sdwan-site-002", "10.2.0.1", "sdwan_router", "site-002", 80),
    "dc-edge-003": Exporter("dc-edge-003", "10.3.0.1", "edge_router", "site-003", 90),
}

# DNS names for the humans reading dashboards. NetFlow v5 does not carry DNS;
# the connector resolves PTRs or uses other enrichment. These are printed as scenario docs.
DNS_HINTS = {
    "10.1.10.11": "user-011.site001.example.local",
    "10.1.10.12": "user-012.site001.example.local",
    "10.1.50.44": "pos-044.site001.example.local",
    "10.1.50.45": "pos-045.site001.example.local",
    "10.2.10.21": "user-021.site002.example.local",
    "10.2.60.31": "iot-031.site002.example.local",
    "10.3.30.15": "payment-api-01.site003.example.local",
    "10.3.30.16": "auth-api-01.site003.example.local",
    "10.3.40.20": "postgres-01.site003.example.local",
    "10.3.40.21": "redis-01.site003.example.local",
    "10.3.53.10": "dns-01.site003.example.local",
}

# The first few templates intentionally duplicate the same conversation through
# firewall and SD-WAN exporters. The collector should suppress the lower-priority copy.
FLOW_TEMPLATES: List[FlowTemplate] = [
    FlowTemplate(
        name="site001_pos_to_payment_duplicate_top_talker",
        src_ip="10.1.50.44",
        dst_ip="10.3.30.15",
        src_port_range=(50000, 52000),
        dst_port=443,
        proto=PROTO_TCP,
        packets_range=(1200, 3000),
        bytes_per_packet_range=(900, 1400),
        tcp_flags=TCP_ACK_PUSH,
        dscp=34,  # AF41 video/interactive media - intentionally top talker heavy
        exporters=("fw-site-001", "sdwan-site-001"),
        duplicate_jitter_percent=6.0,
        weight=12,
    ),
    FlowTemplate(
        name="site001_users_to_auth_duplicate",
        src_ip="10.1.10.11",
        dst_ip="10.3.30.16",
        src_port_range=(42000, 50000),
        dst_port=443,
        proto=PROTO_TCP,
        packets_range=(200, 900),
        bytes_per_packet_range=(700, 1200),
        tcp_flags=TCP_ACK_PUSH,
        dscp=46,  # EF voice
        exporters=("fw-site-001", "sdwan-site-001"),
        duplicate_jitter_percent=8.0,
        weight=8,
    ),
    FlowTemplate(
        name="site001_pos_to_postgres_duplicate",
        src_ip="10.1.50.45",
        dst_ip="10.3.40.20",
        src_port_range=(52000, 54000),
        dst_port=5432,
        proto=PROTO_TCP,
        packets_range=(300, 1200),
        bytes_per_packet_range=(800, 1500),
        tcp_flags=TCP_ACK_PUSH,
        dscp=26,  # AF31 business critical
        exporters=("fw-site-001", "sdwan-site-001"),
        duplicate_jitter_percent=10.0,
        weight=6,
    ),
    FlowTemplate(
        name="site002_users_to_payment_duplicate",
        src_ip="10.2.10.21",
        dst_ip="10.3.30.15",
        src_port_range=(43000, 51000),
        dst_port=443,
        proto=PROTO_TCP,
        packets_range=(400, 1600),
        bytes_per_packet_range=(800, 1400),
        tcp_flags=TCP_ACK_PUSH,
        dscp=34,  # AF41 video
        exporters=("fw-site-002", "sdwan-site-002"),
        duplicate_jitter_percent=6.0,
        weight=8,
    ),
    FlowTemplate(
        name="site002_iot_to_dns",
        src_ip="10.2.60.31",
        dst_ip="10.3.53.10",
        src_port_range=(30000, 45000),
        dst_port=53,
        proto=PROTO_UDP,
        packets_range=(10, 40),
        bytes_per_packet_range=(70, 140),
        tcp_flags=0,
        dscp=0,
        exporters=("fw-site-002", "sdwan-site-002"),
        duplicate_jitter_percent=5.0,
        weight=4,
    ),
    FlowTemplate(
        name="dc_edge_observes_payment_to_external_https",
        src_ip="10.3.30.15",
        dst_ip="8.8.8.8",
        src_port_range=(45000, 55000),
        dst_port=443,
        proto=PROTO_TCP,
        packets_range=(50, 300),
        bytes_per_packet_range=(500, 1200),
        tcp_flags=TCP_ACK_PUSH,
        dscp=0,
        exporters=("dc-edge-003",),
        duplicate_jitter_percent=0.0,
        weight=2,
    ),
    FlowTemplate(
        name="site001_voice_rtp_ef_duplicate",
        src_ip="10.1.10.12",
        dst_ip="10.3.30.16",
        src_port_range=(16000, 20000),
        dst_port=16384,
        proto=PROTO_UDP,
        packets_range=(800, 2200),
        bytes_per_packet_range=(160, 240),
        tcp_flags=0,
        dscp=46,  # EF voice RTP
        exporters=("fw-site-001", "sdwan-site-001"),
        duplicate_jitter_percent=4.0,
        weight=10,
    ),
    FlowTemplate(
        name="site002_video_af41_duplicate",
        src_ip="10.2.10.21",
        dst_ip="10.3.30.16",
        src_port_range=(30000, 36000),
        dst_port=5004,
        proto=PROTO_UDP,
        packets_range=(1500, 3500),
        bytes_per_packet_range=(900, 1300),
        tcp_flags=0,
        dscp=34,  # AF41 video
        exporters=("fw-site-002", "sdwan-site-002"),
        duplicate_jitter_percent=5.0,
        weight=10,
    ),

]


def ip_to_int(ip: str) -> int:
    return int(ipaddress.IPv4Address(ip))


def build_header(count: int, sequence: int, boot_time: float) -> bytes:
    now = time.time()
    sys_uptime_ms = int((now - boot_time) * 1000) & 0xFFFFFFFF
    unix_secs = int(now)
    unix_nsecs = int((now - unix_secs) * 1_000_000_000)
    engine_type = 0
    engine_id = 0
    sampling_interval = 0
    return struct.pack(
        "!HHIIIIBBH",
        NETFLOW_VERSION,
        count,
        sys_uptime_ms,
        unix_secs,
        unix_nsecs,
        sequence,
        engine_type,
        engine_id,
        sampling_interval,
    )


def build_record(
    src_ip: str,
    dst_ip: str,
    src_port: int,
    dst_port: int,
    proto: int,
    packets: int,
    octets: int,
    tcp_flags: int,
    dscp: int,
    first_ms: int,
    last_ms: int,
    input_if: int = 1,
    output_if: int = 2,
) -> bytes:
    nexthop = 0
    pad1 = 0
    tos = (dscp & 0x3F) << 2
    src_as = 0
    dst_as = 0
    src_mask = 24
    dst_mask = 24
    pad2 = 0
    return struct.pack(
        "!IIIHHIIIIHHBBBBHHBBH",
        ip_to_int(src_ip),
        ip_to_int(dst_ip),
        nexthop,
        input_if,
        output_if,
        packets,
        octets,
        first_ms,
        last_ms,
        src_port,
        dst_port,
        pad1,
        tcp_flags,
        proto,
        tos,
        src_as,
        dst_as,
        src_mask,
        dst_mask,
        pad2,
    )


def send_records(target: str, port: int, exporter: Exporter, records: List[bytes], sequence: int, boot_time: float) -> int:
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        # Bind to the loopback alias. Run setup_loopback_exporters.sh first.
        sock.bind((exporter.ip, 0))
    except OSError as exc:
        print(
            f"ERROR: could not bind exporter {exporter.name} ({exporter.ip}).\n"
            f"Run ./scripts/setup_loopback_exporters.sh first. Original error: {exc}",
            file=sys.stderr,
        )
        raise

    try:
        offset = 0
        while offset < len(records):
            chunk = records[offset : offset + MAX_RECORDS_PER_PACKET]
            packet = build_header(len(chunk), sequence, boot_time) + b"".join(chunk)
            sock.sendto(packet, (target, port))
            sequence += len(chunk)
            offset += len(chunk)
    finally:
        sock.close()
    return sequence


def choose_template() -> FlowTemplate:
    return random.choices(FLOW_TEMPLATES, weights=[t.weight for t in FLOW_TEMPLATES], k=1)[0]


def instantiate_records(template: FlowTemplate, boot_time: float) -> Dict[str, bytes]:
    src_port = random.randint(*template.src_port_range)
    packets = random.randint(*template.packets_range)
    bpp = random.randint(*template.bytes_per_packet_range)
    octets = packets * bpp
    now_ms = int((time.time() - boot_time) * 1000) & 0xFFFFFFFF
    first_ms = max(0, now_ms - random.randint(100, 5000))
    last_ms = now_ms

    out: Dict[str, bytes] = {}
    for idx, exporter_name in enumerate(template.exporters):
        jitter = 1.0
        if idx > 0 and template.duplicate_jitter_percent:
            spread = template.duplicate_jitter_percent / 100.0
            jitter = random.uniform(1.0 - spread, 1.0 + spread)
        exp_packets = max(1, int(packets * jitter))
        exp_octets = max(64, int(octets * jitter))
        out[exporter_name] = build_record(
            template.src_ip,
            template.dst_ip,
            src_port,
            template.dst_port,
            template.proto,
            exp_packets,
            exp_octets,
            template.tcp_flags,
            template.dscp,
            first_ms,
            last_ms,
        )
    return out


def print_scenario() -> None:
    print("\nFlow2Span synthetic site model")
    print("=" * 38)
    print("Sites:")
    print("  site-001: Seattle Branch    10.1.0.0/16  links: 500Mbps WAN + 50Mbps LTE")
    print("  site-002: Calgary Branch    10.2.0.0/16  links: 1Gbps WAN")
    print("  site-003: Denver DC         10.3.0.0/16  links: 10Gbps Internet/DC edge")
    print("\nDuplicate exporters modeled:")
    print("  site-001: fw-site-001 10.1.0.5 priority 100 + sdwan-site-001 10.1.0.1 priority 80")
    print("  site-002: fw-site-002 10.2.0.5 priority 100 + sdwan-site-002 10.2.0.1 priority 80")
    print("\nTop-talker-heavy flow:")
    print("  pos-044.site001.example.local -> payment-api-01.site003.example.local:443")
    print("\nDNS hints expected in dashboards/traces:")
    for ip, name in DNS_HINTS.items():
        print(f"  {ip:<15} {name}")
    print("")


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate synthetic NetFlow v5 traffic for Flow2Span POC")
    parser.add_argument("--target", default="127.0.0.1", help="Collector NetFlow receiver address")
    parser.add_argument("--port", type=int, default=2055, help="Collector NetFlow receiver port")
    parser.add_argument("--duration", type=int, default=120, help="Run duration in seconds")
    parser.add_argument("--rate", type=float, default=8.0, help="Flow conversations per second before duplicates")
    parser.add_argument("--seed", type=int, default=42, help="Random seed")
    parser.add_argument("--dry-run", action="store_true", help="Print scenario only")
    args = parser.parse_args()

    random.seed(args.seed)
    print_scenario()
    if args.dry_run:
        return 0

    boot_time = time.time() - 60
    sequences = {name: 0 for name in EXPORTERS}
    deadline = time.time() + args.duration
    interval = 1.0 / max(args.rate, 0.1)

    sent_records = 0
    sent_packets = 0
    next_report = time.time() + 10

    print(f"Sending NetFlow v5 to {args.target}:{args.port} for {args.duration}s at ~{args.rate} conversations/s")
    while time.time() < deadline:
        template = choose_template()
        records_by_exporter = instantiate_records(template, boot_time)
        for exporter_name, record in records_by_exporter.items():
            exporter = EXPORTERS[exporter_name]
            sequences[exporter_name] = send_records(
                args.target,
                args.port,
                exporter,
                [record],
                sequences[exporter_name],
                boot_time,
            )
            sent_packets += 1
            sent_records += 1

        if time.time() >= next_report:
            print(f"sent_records={sent_records} udp_packets={sent_packets}")
            next_report = time.time() + 10
        time.sleep(interval)

    print(f"Done. sent_records={sent_records} udp_packets={sent_packets}")
    print("Expected POC behavior:")
    print("  - fw-site-* records should win dedupe over sdwan-site-* for duplicate conversations")
    print("  - flow2span.dedup.duplicate_bytes_suppressed should be > 0")
    print("  - flow2span.site.bits_per_second should show site-001/site-002/site-003")
    print("  - flow2span.link.utilization_percent should show link pressure vs configured speed")
    print("  - flow2span.top_talker.* metrics should include trace.id/span.id attributes")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
