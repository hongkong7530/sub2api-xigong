"""Provision/revoke one enrolled node's state credentials on the data authority.

Requires local PostgreSQL owner access and Redis ACL administration. Secrets are
written only to a root-only transfer bundle, never printed or placed in argv.
The script does not grant schema ownership, activate traffic or migrate data.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys

IDENTITY = re.compile(r"^[a-z0-9][a-z0-9_-]{0,63}$")


def literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def sql(args: argparse.Namespace, statement: str) -> str:
    command = ["runuser", "-u", "postgres", "--", "psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-h", args.pg_socket, "-p", str(args.pg_port), "-d", args.database]
    result = subprocess.run(command, input="SET statement_timeout='15s';\n" + statement, text=True, capture_output=True, timeout=20)
    if result.returncode:
        raise RuntimeError("PostgreSQL operation failed; no credentials were printed. Inspect the authority locally.")
    return result.stdout.strip()


def redis(args: argparse.Namespace, *command: str, secret_input: str | None = None) -> str:
    env = os.environ.copy()
    if args.redis_password_file:
        env["REDISCLI_AUTH"] = args.redis_password_file.read_text().strip()
    argv = ["redis-cli", "--raw", "-s", args.redis_socket]
    if args.redis_admin:
        argv += ["--user", args.redis_admin]
    if secret_input is not None:
        argv += ["-x"]
    result = subprocess.run(argv + list(command), input=secret_input, env=env, text=True, capture_output=True, timeout=10)
    output = result.stdout.strip()
    if result.returncode or output.startswith(("ERR ", "NOAUTH", "WRONGPASS", "NOPERM")):
        raise RuntimeError("Redis ACL operation failed; keep the node paused until provisioning is completed")
    return output


def provision(args: argparse.Namespace) -> None:
    if os.geteuid() != 0:
        raise ValueError("run locally as root on the data authority")
    if not args.output.is_absolute() or args.output.exists() or args.output.parent.is_symlink():
        raise ValueError("a new absolute root-only transfer bundle path is required")
    node = json.loads(sql(args, "SELECT json_build_object('state',state,'connections',db_connections,'roles',roles,'certificate_sha256',certificate_sha256) FROM cluster_nodes WHERE cluster_id=" + literal(args.cluster) + " AND node_id=" + literal(args.node)))
    if node["state"] != "pending":
        raise ValueError("credentials can only be provisioned for a pending node")
    if not node["roles"] or not set(node["roles"]).issubset({"worker", "ingress", "controller", "maintenance"}):
        raise ValueError("invalid registered node roles")
    suffix = hashlib.sha256((args.cluster + ":" + args.node + ":" + node["certificate_sha256"]).encode()).hexdigest()[:20]
    role, user = "s2c_" + suffix, "s2c_" + suffix
    db_password, redis_password = secrets.token_urlsafe(36), secrets.token_urlsafe(36)
    # Journal the transfer bundle first so a partial provisioning can be safely completed or revoked.
    args.output.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(args.output.parent, 0o700)
    fd = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as handle:
        json.dump({"cluster_id": args.cluster, "node_id": args.node, "database_role": role, "database_password": db_password, "redis_user": user, "redis_password": redis_password, "db_connections": node["connections"]}, handle)
        handle.flush()
        os.fsync(handle.fileno())
    statement = """BEGIN;
SET LOCAL log_statement='none';
CREATE ROLE __ROLE__ LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT __LIMIT__ PASSWORD __PASSWORD__;
GRANT CONNECT ON DATABASE __DATABASE__ TO __ROLE__;
GRANT USAGE ON SCHEMA public TO __ROLE__;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO __ROLE__;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO __ROLE__;
DO $permissions$ DECLARE item record; BEGIN
  FOR item IN SELECT tablename FROM pg_tables WHERE schemaname='public'
    AND tablename NOT IN ('schema_migrations','atlas_schema_revisions','security_secrets','settings','cluster_control','cluster_nodes','cluster_join_tokens','cluster_operations','cluster_node_credentials','cluster_pricing_revision')
  LOOP EXECUTE format('GRANT INSERT, UPDATE, DELETE ON TABLE public.%I TO %I',item.tablename,__ROLE_LITERAL__); END LOOP;
END $permissions$;
GRANT EXECUTE ON FUNCTION cluster_observe_redis_generation(TEXT,TEXT) TO __ROLE__;
INSERT INTO cluster_node_credentials(cluster_id,node_id,database_role,redis_user) VALUES(__CLUSTER__,__NODE__,__ROLE_LITERAL__,__USER_LITERAL__);
COMMIT;"""
    replacements = {"ROLE_LITERAL": literal(role), "USER_LITERAL": literal(user), "PASSWORD": literal(db_password), "DATABASE": '"' + args.database + '"', "CLUSTER": literal(args.cluster), "NODE": literal(args.node), "LIMIT": str(node["connections"]), "ROLE": '"' + role + '"'}
    for key in ("ROLE_LITERAL", "USER_LITERAL", "PASSWORD", "DATABASE", "CLUSTER", "NODE", "LIMIT", "ROLE"):
        statement = statement.replace("__" + key + "__", replacements[key])
    if "controller" in node["roles"]:
        statement = statement.replace("COMMIT;", "GRANT INSERT, UPDATE, DELETE ON cluster_control, cluster_nodes, cluster_join_tokens, cluster_operations, cluster_node_credentials TO \"" + role + "\"; COMMIT;")
    if "ingress" in node["roles"] or "maintenance" in node["roles"]:
        statement = statement.replace("COMMIT;", "GRANT INSERT, UPDATE, DELETE ON settings TO \"" + role + "\"; COMMIT;")
    sql(args, statement)
    acl_config = redis(args, "CONFIG", "GET", "aclfile").splitlines()
    if (len(acl_config) < 2 or not acl_config[1].strip()) and not args.ephemeral_acl:
        raise RuntimeError("Redis aclfile is not configured; credentials remain provisioning until restart persistence is configured")
    redis(args, "ACL", "SETUSER", user, "reset", "on", "~*", "&*", "+@all", "-@admin", "-@dangerous", "+info", "+eval", "+evalsha", "+script|load", "+script|exists", "+client|setname", secret_input=">" + redis_password)
    if not args.ephemeral_acl:
        redis(args, "ACL", "SAVE")
    sql(args, "UPDATE cluster_node_credentials SET status='active' WHERE cluster_id=" + literal(args.cluster) + " AND node_id=" + literal(args.node))
    print(json.dumps({"node_id": args.node, "status": "credentials_provisioned", "bundle": str(args.output), "traffic_enabled": False}))


def revoke(args: argparse.Namespace) -> None:
    entry = json.loads(sql(args, "SELECT json_build_object('role',c.database_role,'redis',c.redis_user,'state',n.state) FROM cluster_node_credentials c JOIN cluster_nodes n USING(cluster_id,node_id) WHERE c.cluster_id=" + literal(args.cluster) + " AND c.node_id=" + literal(args.node)))
    if entry["state"] != "retired":
        raise ValueError("retire the node and finish its work before revoking credentials")
    if not IDENTITY.fullmatch(entry["role"]) or not IDENTITY.fullmatch(entry["redis"]):
        raise ValueError("invalid stored credential identity")
    sql(args, "UPDATE cluster_node_credentials SET status='revoking' WHERE cluster_id=" + literal(args.cluster) + " AND node_id=" + literal(args.node) + "; ALTER ROLE \"" + entry["role"] + "\" NOLOGIN; SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename=" + literal(entry["role"]) + " AND pid<>pg_backend_pid();")
    redis(args, "CLIENT", "KILL", "USER", entry["redis"])
    redis(args, "ACL", "DELUSER", entry["redis"])
    if not args.ephemeral_acl:
        redis(args, "ACL", "SAVE")
    sql(args, "BEGIN; UPDATE cluster_node_credentials SET status='revoked',revoked_at=NOW() WHERE cluster_id=" + literal(args.cluster) + " AND node_id=" + literal(args.node) + "; UPDATE cluster_operations SET status='applied',applied_at=NOW(),error_code=NULL WHERE cluster_id=" + literal(args.cluster) + " AND node_id=" + literal(args.node) + " AND status='awaiting_revocation'; COMMIT;")
    print(json.dumps({"node_id": args.node, "status": "credentials_revoked", "role_dropped": False, "business_data_deleted": False}))


def parse() -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("action", choices=["provision", "revoke"])
    p.add_argument("--cluster", required=True)
    p.add_argument("--node", required=True)
    p.add_argument("--database", required=True)
    p.add_argument("--pg-socket", required=True)
    p.add_argument("--pg-port", type=int, default=5432)
    p.add_argument("--redis-socket", required=True)
    p.add_argument("--redis-admin", default="")
    p.add_argument("--redis-password-file", type=Path)
    p.add_argument("--output", type=Path)
    p.add_argument("--ephemeral-acl", action="store_true", help="Isolated tests only: do not claim restart-persistent ACL provisioning")
    args = p.parse_args()
    if not all(IDENTITY.fullmatch(v) for v in (args.cluster, args.node, args.database)):
        p.error("cluster, node and database must be safe identifiers")
    if args.action == "provision" and args.output is None:
        p.error("provision requires --output")
    return args


if __name__ == "__main__":
    try:
        args = parse()
        (provision if args.action == "provision" else revoke)(args)
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
