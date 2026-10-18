"""Read-only capacity report and explicit non-destructive service tuning generator.

This tool never deletes data or rewrites a running database. Generate emits
systemd review files for a named service; it does not apply or restart services.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys


def safe_name(value: str) -> str:
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]{0,63}", value):
        raise ValueError("invalid service identity")
    return value


def atomic_file(path: Path, text: str) -> None:
    if path.is_symlink():
        raise ValueError("refusing symlink configuration target")
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".cluster-new")
    with temporary.open("x", encoding="utf-8") as handle:
        handle.write(text)
        handle.flush()
        os.fsync(handle.fileno())
    temporary.chmod(0o644)
    temporary.replace(path)


def report() -> None:
    memory = {}
    for line in Path('/proc/meminfo').read_text().splitlines():
        key, _, value = line.partition(':')
        if key in {'MemTotal', 'MemAvailable', 'SwapTotal', 'SwapFree'}:
            memory[key] = int(value.split()[0]) * 1024
    total, used, free = shutil.disk_usage('/')
    print(json.dumps({'cpu_online': os.cpu_count(), 'memory_bytes': memory, 'root_bytes': {'total': total, 'used': used, 'available': free}, 'network_counter_source': 'interface-estimate-not-provider-bill'}))


def generate(args: argparse.Namespace) -> None:
    safe_name(args.service)
    if args.db_connections < 1 or args.db_connections > args.cluster_db_budget:
        raise ValueError('node connections exceed the cluster application budget')
    if args.max_inflight < 1 or not 0 <= args.max_queue <= args.max_inflight:
        raise ValueError('invalid bounded admission budget')
    if not args.directory.is_absolute():
        raise ValueError('output directory must be absolute')
    if args.directory.exists() and any(args.directory.iterdir()):
        raise ValueError('use a new review directory, not a live system path')
    args.directory.mkdir(parents=True, mode=0o700, exist_ok=True)
    # Application log rotation owns files; journal remains a bounded emergency channel.
    content = f'''[Service]
LimitNOFILE=524288
Restart=on-failure
RestartSec=3
TimeoutStopSec=180
KillSignal=SIGTERM
UMask=0077
NoNewPrivileges=yes
PrivateTmp=yes
Environment=DATABASE_MAX_OPEN_CONNS={args.db_connections}
Environment=DATABASE_MAX_IDLE_CONNS={min(16, args.db_connections)}
Environment=CLUSTER_MAX_IN_FLIGHT={args.max_inflight}
Environment=CLUSTER_MAX_QUEUE={args.max_queue}
Environment=CLUSTER_QUEUE_TIMEOUT_MS=1000
Environment=CLUSTER_RETENTION_APPROVED=false
Environment=DASHBOARD_AGGREGATION_RETENTION_USAGE_LOGS_DAYS=30
Environment=OPS_CLEANUP_ERROR_LOG_RETENTION_DAYS=7
Environment=OPS_CLEANUP_SYSTEM_LOG_RETENTION_DAYS=7
Environment=LOG_OUTPUT_TO_STDOUT=false
Environment=LOG_OUTPUT_TO_FILE=true
Environment=LOG_OUTPUT_FILE_PATH=/var/log/{args.service}/application.log
Environment=LOG_ROTATION_MAX_SIZE_MB=64
Environment=LOG_ROTATION_MAX_BACKUPS=4
Environment=LOG_ROTATION_MAX_AGE_DAYS=7
Environment=LOG_ROTATION_COMPRESS=true
Environment=LOG_SAMPLING_ENABLED=true
Environment=LOG_SAMPLING_INITIAL=100
Environment=LOG_SAMPLING_THEREAFTER=100
'''
    atomic_file(args.directory / 'service.conf', content)
    atomic_file(args.directory / 'limits.json', json.dumps({'service': args.service, 'db_pool': args.db_connections, 'cluster_application_budget': args.cluster_db_budget, 'max_in_flight': args.max_inflight, 'max_queue': args.max_queue, 'automatic_history_deletion': False, 'hard_oom_limit': False, 'database_durability_changed': False, 'service_restarted': False}, indent=2) + '\n')
    print(json.dumps({'review_directory': str(args.directory), 'applied': False, 'requires_capacity_acceptance': True}))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    sub.add_parser('report')
    gen = sub.add_parser('generate')
    gen.add_argument('--service', required=True)
    gen.add_argument('--directory', required=True, type=Path)
    gen.add_argument('--db-connections', type=int, required=True)
    gen.add_argument('--cluster-db-budget', type=int, default=224)
    gen.add_argument('--max-inflight', type=int, required=True)
    gen.add_argument('--max-queue', type=int, required=True)
    args = parser.parse_args()
    if args.action == 'report':
        report()
    else:
        generate(args)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
