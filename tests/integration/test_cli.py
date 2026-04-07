import subprocess


class TestCLI:
    def test_version(self, binary):
        r = subprocess.run(
            [str(binary), "--version"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0
        assert "SerialHub v" in r.stdout

    def test_help_flags(self, binary):
        r = subprocess.run(
            [str(binary), "--help"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0
        for flag in [
            "--serial-port",
            "--baud-rate",
            "--mcp-port",
            "--config",
            "--debug",
            "--no-tray",
            "--minimized",
        ]:
            assert flag in r.stdout, f"缺少 {flag}"

    def test_debug_with_help(self, binary):
        r = subprocess.run(
            [str(binary), "--debug", "--help"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0
        assert "--serial-port" in r.stdout

    def test_invalid_flag(self, binary):
        r = subprocess.run(
            [str(binary), "--nonexistent"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode != 0

    def test_baud_rate_flag(self, binary):
        r = subprocess.run(
            [str(binary), "--baud-rate", "9600", "--help"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=10,
        )
        assert r.returncode == 0
