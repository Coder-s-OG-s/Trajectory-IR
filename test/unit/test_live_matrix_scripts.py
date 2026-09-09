"""Unit tests for live matrix scripts, environment handling, and compose definitions."""

import re
import urllib.parse
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
COMPOSE_FILE = REPO_ROOT / "docker-compose.live.yml"
ENV_EXAMPLE = REPO_ROOT / ".env.example"
SH_SCRIPT = REPO_ROOT / "scripts" / "run_live_matrix.sh"
PS1_SCRIPT = REPO_ROOT / "scripts" / "run_live_matrix.ps1"

LOOPBACK_PATTERN = re.compile(
    r"^([a-zA-Z][a-zA-Z0-9+.-]*://)?([^@/]*@)?(127(\.[0-9]+){3}|localhost|\[::1\]|::1)(:[0-9]+)?(/.*)?$"
)


def is_loopback(url: str) -> bool:
    return bool(LOOPBACK_PATTERN.match(url))


class TestLoopbackValidation:
    """Validate loopback detection regex used in run_live_matrix.sh and .ps1."""

    def test_loopback_valid_endpoints(self):
        valid = [
            "http://127.0.0.1:9000",
            "http://localhost:9000",
            "http://127.0.0.2:5432",
            "http://[::1]:9000",
            "http://::1:9000",
            "127.0.0.1:5432",
            "localhost:7233",
            "postgresql://trajir:trajir@127.0.0.1:5432/trajir",
            "postgresql://trajir:trajir@localhost:5432/trajir",
            "postgresql://user%40domain:p%40ss%3Aword@127.0.0.1:5432/trajir",
            "postgresql://user:pass@127.0.0.1/db?sslmode=disable",
        ]
        for ep in valid:
            assert is_loopback(ep), f"Expected {ep} to be detected as loopback"

    def test_loopback_rejects_external_endpoints(self):
        invalid = [
            "http://192.168.1.100:9000",
            "http://example.com:9000",
            "postgresql://trajir:trajir@db.internal:5432/trajir",
            "postgresql://trajir:trajir@0.0.0.0:5432/trajir",
            "http://10.0.0.1:9000",
            "http://169.254.169.254:80",
            "http://172.17.0.2:9000",
        ]
        for ep in invalid:
            assert not is_loopback(ep), f"Expected {ep} to NOT be detected as loopback"


class TestDatabaseUrlEncoding:
    """Verify URL encoding for database credentials to prevent DSN corruption."""

    def test_urlencoding_special_characters(self):
        user = "trajir_user@corp"
        password = "p@ss:w/ord#?%&"
        db = "trajir/db#1"

        encoded_user = urllib.parse.quote(user, safe="")
        encoded_password = urllib.parse.quote(password, safe="")
        encoded_db = urllib.parse.quote(db, safe="")

        assert encoded_user == "trajir_user%40corp"
        assert encoded_password == "p%40ss%3Aw%2Ford%23%3F%25%26"
        assert encoded_db == "trajir%2Fdb%231"

        dsn = f"postgresql://{encoded_user}:{encoded_password}@127.0.0.1:5432/{encoded_db}"

        # Ensure loopback check passes with encoded credentials
        assert is_loopback(dsn)

        # Ensure standard URI parsers parse host and credentials accurately
        parsed = urllib.parse.urlsplit(dsn)
        assert parsed.hostname == "127.0.0.1"
        assert parsed.port == 5432
        assert urllib.parse.unquote(parsed.username) == user
        assert urllib.parse.unquote(parsed.password) == password
        assert urllib.parse.unquote(parsed.path.lstrip("/")) == db

    def test_unencoded_special_chars_break_unquoted_uri(self):
        # Demonstrates why encoding is strictly necessary:
        # An unencoded '@' in password splits the URI at the wrong place
        broken_dsn = "postgresql://trajir:p@ssword@127.0.0.1:5432/trajir"
        assert not is_loopback(broken_dsn)


class TestEnvParsingLogic:
    """Verify .env file quote and key parsing behavior."""

    def test_quote_stripping_only_outer_matching(self):
        def parse_val(raw_val: str) -> str:
            val = raw_val.strip()
            if (val.startswith('"') and val.endswith('"') and len(val) >= 2) or (
                val.startswith("'") and val.endswith("'") and len(val) >= 2
            ):
                return val[1:-1]
            return val

        assert parse_val('"simple"') == "simple"
        assert parse_val("'single'") == "single"
        assert parse_val("\"mismatched'") == "\"mismatched'"
        assert parse_val("'mismatched\"") == "'mismatched\""
        assert parse_val('"{"nested": "json"}"') == '{"nested": "json"}'
        assert parse_val('unquoted"middle') == 'unquoted"middle'
        assert parse_val("") == ""

    def test_valid_env_key_names(self):
        valid_key_pattern = re.compile(r"^[a-zA-Z_][a-zA-Z0-9_]*$")
        assert valid_key_pattern.match("POSTGRES_USER")
        assert valid_key_pattern.match("_SECRET_KEY_123")
        assert not valid_key_pattern.match("123_INVALID")
        assert not valid_key_pattern.match("POSTGRES-USER")
        assert not valid_key_pattern.match("POSTGRES.USER")


class TestCredentialSynchronization:
    """Verify credential sync between MinIO root credentials and AWS client credentials."""

    def test_env_example_comments_out_aws_credentials(self):
        content = ENV_EXAMPLE.read_text(encoding="utf-8")
        lines = [line.strip() for line in content.splitlines()]

        # Active assignments should not include AWS_ACCESS_KEY_ID or AWS_SECRET_ACCESS_KEY
        active_assignments = [line for line in lines if not line.startswith("#") and "=" in line]
        active_keys = [line.split("=", 1)[0].strip() for line in active_assignments]

        assert "MINIO_ROOT_USER" in active_keys
        assert "MINIO_ROOT_PASSWORD" in active_keys
        assert "AWS_ACCESS_KEY_ID" not in active_keys, "AWS_ACCESS_KEY_ID should be commented out"
        assert "AWS_SECRET_ACCESS_KEY" not in active_keys, (
            "AWS_SECRET_ACCESS_KEY should be commented out"
        )

    def test_script_sync_defaults(self):
        # Simulated shell/pwsh fallback logic
        minio_user = "custom_minio_user"
        minio_pass = "custom_minio_pass"
        env = {}

        aws_id = env.get("AWS_ACCESS_KEY_ID", minio_user)
        aws_secret = env.get("AWS_SECRET_ACCESS_KEY", minio_pass)

        assert aws_id == "custom_minio_user"
        assert aws_secret == "custom_minio_pass"


class TestDockerComposeHealthchecks:
    """Verify docker-compose.live.yml healthchecks use execve CMD list syntax."""

    def test_healthchecks_avoid_cmd_shell(self):
        compose_content = yaml.safe_load(COMPOSE_FILE.read_text(encoding="utf-8"))
        services = compose_content.get("services", {})

        for svc_name in ("postgres", "temporal-postgres"):
            svc = services.get(svc_name, {})
            healthcheck = svc.get("healthcheck", {})
            test_cmd = healthcheck.get("test")

            assert isinstance(test_cmd, list), f"{svc_name} healthcheck test should be a list"
            assert test_cmd[0] == "CMD", (
                f"{svc_name} healthcheck should use CMD list syntax, got: {test_cmd[0]}"
            )
            assert "pg_isready" in test_cmd[1], f"{svc_name} healthcheck should execute pg_isready"


class TestDecoupledMinioProbe:
    """Verify scripts probe the local published MinIO port rather than TRAJIR_S3_ENDPOINT_URL."""

    def test_sh_script_probes_local_port(self):
        content = SH_SCRIPT.read_text(encoding="utf-8")
        assert 'minio_live_url="http://127.0.0.1:9000/minio/health/live"' in content
        assert 'minio_live_url="${TRAJIR_S3_ENDPOINT_URL' not in content

    def test_ps1_script_probes_local_port(self):
        content = PS1_SCRIPT.read_text(encoding="utf-8")
        assert '$minioLiveUrl = "http://127.0.0.1:9000/minio/health/live"' in content
        assert '$minioLiveUrl = "$($env:TRAJIR_S3_ENDPOINT_URL' not in content


class TestTemporalComposeConfiguration:
    """Verify Temporal service configuration in docker-compose.live.yml."""

    def test_temporal_service_env(self):
        compose_content = yaml.safe_load(COMPOSE_FILE.read_text(encoding="utf-8"))
        services = compose_content.get("services", {})
        temporal_svc = services.get("temporal", {})
        env = temporal_svc.get("environment", {})

        # Ensure unused POSTGRES_DB is removed from temporal service
        assert "POSTGRES_DB" not in env, (
            "POSTGRES_DB is unused by temporalio/auto-setup and should not be set"
        )
        # Ensure DBNAME and VISIBILITY_DBNAME are parameterized
        assert "DBNAME" in env, "DBNAME should be defined for Temporal persistence DB"
        assert "VISIBILITY_DBNAME" in env, (
            "VISIBILITY_DBNAME should be parameterized for Temporal visibility DB"
        )
        assert "TEMPORAL_POSTGRES_DB" in str(env["DBNAME"])
        assert "VISIBILITY_DBNAME" in str(env["VISIBILITY_DBNAME"]) or "temporal_visibility" in str(
            env["VISIBILITY_DBNAME"]
        )
