"""
Yutori n2 Shell and File Tools

n2's tool set always ships `bash`, `read`, `write`, and `edit` alongside
`computer_batch` — `disable_tools` is rejected by the API, so a loop has to
answer these too. They run against the Kernel browser VM via the process and
filesystem APIs.

@see https://docs.yutori.com/reference/n2
"""

from __future__ import annotations

import base64
import shlex
import time
from typing import Any, Optional

from kernel import Kernel

from .base import ToolError

DEFAULT_TIMEOUT_SEC = 120
MAX_TIMEOUT_SEC = 600
DEFAULT_READ_LIMIT = 2000
MAX_WRITE_CHARS = 256_000

# bash: "The working directory persists across calls; environment variables and
# shell functions do not." Each exec is its own process, so the command reports
# its final directory on a sentinel line that we strip before returning stdout.
CWD_SENTINEL = "__n2_cwd__"


def _decode(value: Optional[str]) -> str:
    return base64.b64decode(value).decode("utf-8", errors="replace") if value else ""


def _format_command_result(stdout: str, stderr: str, exit_code: Optional[int]) -> str:
    parts = []
    if stdout.strip():
        parts.append(stdout.rstrip())
    if stderr.strip():
        parts.append(f"stderr:\n{stderr.rstrip()}")
    if exit_code:
        parts.append(f"Exited with code {exit_code}.")

    return "\n".join(parts) if parts else "Command produced no output."


class SystemTools:
    def __init__(self, kernel: Kernel, session_id: str):
        self.kernel = kernel
        self.session_id = session_id
        self.cwd: Optional[str] = None
        self.seen_paths: set[str] = set()

    def execute(self, name: str, args: dict[str, Any]) -> str:
        handlers = {
            "bash": self._bash,
            "read": self._read,
            "write": self._write,
            "edit": self._edit,
        }
        handler = handlers.get(name)
        if not handler:
            raise ToolError(f"Unknown tool: {name}")

        return handler(args)

    def _bash(self, args: dict[str, Any]) -> str:
        command = args.get("command")
        if not command:
            raise ToolError("command is required for bash")

        if args.get("run_in_background"):
            return self._bash_background(command)

        script = "\n".join([
            command,
            "n2_status=$?",
            f"printf '\\n{CWD_SENTINEL}%s' \"$(pwd)\"",
            "exit $n2_status",
        ])

        result = self.kernel.browsers.process.exec(
            self.session_id,
            command="bash",
            args=["-lc", script],
            cwd=self.cwd,
            timeout_sec=min(args.get("timeout") or DEFAULT_TIMEOUT_SEC, MAX_TIMEOUT_SEC),
        )

        stdout = self._take_cwd(_decode(result.stdout_b64))

        return _format_command_result(stdout, _decode(result.stderr_b64), result.exit_code)

    def _read(self, args: dict[str, Any]) -> str:
        file_path = args.get("file_path")
        if not file_path:
            raise ToolError("file_path is required for read")

        response = self.kernel.browsers.fs.read_file(self.session_id, path=file_path)
        lines = response.read().decode("utf-8", errors="replace").split("\n")

        offset = max(0, args.get("offset") or 0)
        limit = max(1, args.get("limit") or DEFAULT_READ_LIMIT)
        page = lines[offset : offset + limit]

        self.seen_paths.add(file_path)

        if not page:
            return f"{file_path} has {len(lines)} line(s); offset {offset} is past the end."

        # cat -n format, so line numbers survive into the model's next edit.
        return "\n".join(f"{offset + i + 1:>6}\t{line}" for i, line in enumerate(page))

    def _write(self, args: dict[str, Any]) -> str:
        file_path = args.get("file_path")
        content = args.get("content")
        if not file_path or content is None:
            raise ToolError("file_path and content are required for write")
        if len(content) > MAX_WRITE_CHARS:
            raise ToolError(f"content exceeds the {MAX_WRITE_CHARS} character cap")

        self.kernel.browsers.fs.write_file(self.session_id, content.encode("utf-8"), path=file_path)
        self.seen_paths.add(file_path)

        return f"Wrote {len(content)} character(s) to {file_path}."

    def _edit(self, args: dict[str, Any]) -> str:
        file_path = args.get("file_path")
        old_string = args.get("old_string")
        new_string = args.get("new_string")
        if not file_path or old_string is None or new_string is None:
            raise ToolError("file_path, old_string, and new_string are required for edit")
        # n2 is expected to know the current bytes before changing them.
        if file_path not in self.seen_paths:
            raise ToolError(f"{file_path} has not been read in this session — read it before editing.")

        response = self.kernel.browsers.fs.read_file(self.session_id, path=file_path)
        original = response.read().decode("utf-8", errors="replace")

        occurrences = original.count(old_string)
        if occurrences == 0:
            raise ToolError(f"old_string not found in {file_path}")

        replace_all = bool(args.get("replace_all"))
        if occurrences > 1 and not replace_all:
            raise ToolError(
                f"old_string matches {occurrences} times in {file_path} — "
                f"pass replace_all or include more context."
            )

        updated = original.replace(old_string, new_string, -1 if replace_all else 1)
        self.kernel.browsers.fs.write_file(self.session_id, updated.encode("utf-8"), path=file_path)

        return f"Replaced {occurrences if replace_all else 1} occurrence(s) in {file_path}."

    def _bash_background(self, command: str) -> str:
        log_path = f"/tmp/n2-bg-{int(time.time() * 1000)}.log"
        script = f"nohup bash -c {shlex.quote(command)} > {log_path} 2>&1 &\necho $!"

        result = self.kernel.browsers.process.exec(
            self.session_id,
            command="bash",
            args=["-lc", script],
            cwd=self.cwd,
        )
        pid = _decode(result.stdout_b64).strip()

        return "\n".join([
            f"Started in the background with pid {pid}.",
            f"Output is being written to {log_path} — use the read tool to check on it.",
            f"Cancel it with: kill {pid}",
        ])

    def _take_cwd(self, stdout: str) -> str:
        """Strip the trailing sentinel line and remember the directory it reported."""
        marker = stdout.rfind(f"\n{CWD_SENTINEL}")
        if marker == -1:
            return stdout

        self.cwd = stdout[marker + len(CWD_SENTINEL) + 1 :].strip() or self.cwd
        return stdout[:marker]
