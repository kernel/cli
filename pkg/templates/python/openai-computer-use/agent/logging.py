import logging
import sys
import threading
import time
from datetime import datetime
from typing import Callable

MAX_LINE_WIDTH = 120


def quiet_http_transport_logs() -> None:
    # The Kernel Python SDK uses httpx underneath, and those request logs can
    # become noisy when the surrounding process configures root logging at INFO.
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)


def _timestamp() -> str:
    return datetime.now().strftime("%H:%M:%S.%f")[:-3]


def _truncate_one_line(text: str, max_len: int = 90) -> str:
    one_line = " ".join(text.split())
    if len(one_line) <= max_len:
        return one_line
    return f"{one_line[: max_len - 3]}..."


def _format_kernel_op(op: str) -> str:
    if not op:
        return op
    if "(" in op or "[" in op:
        return op
    return f"{op}()"


class _ThinkingSpinner:
    def __init__(self, enabled: bool):
        self.enabled = enabled
        self.active = False
        self.frame = 0
        self.start_at = 0.0
        self.start_ts = ""
        self.reasoning = ""
        self._thread: threading.Thread | None = None
        self._stop_event = threading.Event()
        self._lock = threading.Lock()

    def start(self) -> None:
        if not self.enabled:
            return
        with self._lock:
            if self.active:
                return
            self.active = True
            self.frame = 0
            self.reasoning = ""
            self.start_at = time.time()
            self.start_ts = _timestamp()
            self._stop_event.clear()
            self._thread = threading.Thread(target=self._run, daemon=True)
            self._thread.start()

    def add_reasoning(self, text: str) -> None:
        with self._lock:
            if not self.active:
                return
            self.reasoning += text

    def stop(self, action: str | None = None, elapsed_seconds: float | None = None) -> None:
        with self._lock:
            if not self.active:
                if action:
                    elapsed_prefix = (
                        f"[{elapsed_seconds:.3f}s] "
                        if isinstance(elapsed_seconds, (int, float))
                        else ""
                    )
                    sys.stdout.write(f"{_timestamp()}  agent> {elapsed_prefix}{action}\n")
                    sys.stdout.flush()
                return
            self.active = False
            self._stop_event.set()
            elapsed = (
                float(elapsed_seconds)
                if isinstance(elapsed_seconds, (int, float))
                else (time.time() - self.start_at)
            )
            elapsed_text = f"{elapsed:.3f}s"
            if self.reasoning.strip():
                reasoning = _truncate_one_line(self.reasoning, 70)
                suffix = f" -> {action}" if action else ""
                sys.stdout.write(
                    f"\r\033[2K{self.start_ts}  agent> [{elapsed_text}] {reasoning}{suffix}\n"
                )
            elif action:
                sys.stdout.write(
                    f"\r\033[2K{self.start_ts}  agent> [{elapsed_text}] {action}\n"
                )
            else:
                sys.stdout.write(
                    f"\r\033[2K{self.start_ts}  agent> [{elapsed_text}] thinking...\n"
                )
            sys.stdout.flush()

    def _run(self) -> None:
        while not self._stop_event.wait(0.1):
            with self._lock:
                if not self.active:
                    return
                self.frame += 1
                elapsed = time.time() - self.start_at
                elapsed_text = f"{elapsed:.3f}s"
                if self.reasoning.strip():
                    prefix = f"{self.start_ts}  agent> [{elapsed_text}] "
                    max_text = max(20, MAX_LINE_WIDTH - len(prefix))
                    reasoning = _truncate_one_line(self.reasoning, max_text)
                    sys.stdout.write(f"\r\033[2K{prefix}{reasoning}")
                else:
                    dots = "." * ((self.frame % 3) + 1)
                    dots = f"{dots:<3}"
                    sys.stdout.write(
                        f"\r\033[2K{self.start_ts}  agent> [{elapsed_text}] thinking{dots}"
                    )
                sys.stdout.flush()


def create_event_logger(verbose: bool = False) -> Callable[[dict], None]:
    spinner = _ThinkingSpinner(sys.stdout.isatty())
    in_text = False
    last_live_view_url = ""

    def render_text(event: dict) -> None:
        nonlocal in_text, last_live_view_url

        event_name = event.get("event", "")
        data = event.get("data", {})
        if not isinstance(data, dict):
            data = {}

        if event_name == "session_state":
            live_url = data.get("live_view_url")
            if (
                isinstance(live_url, str)
                and live_url
                and live_url != last_live_view_url
            ):
                sys.stdout.write(f"{_timestamp()} kernel> live view: {live_url}\n")
                sys.stdout.flush()
                last_live_view_url = live_url
            return

        if event_name == "backend":
            op = data.get("op")
            if not isinstance(op, str) or not op:
                return

            if in_text:
                sys.stdout.write("\n")
                sys.stdout.flush()
                in_text = False

            if op == "live_url":
                detail = data.get("detail")
                if (
                    isinstance(detail, str)
                    and detail
                    and detail != last_live_view_url
                ):
                    sys.stdout.write(f"{_timestamp()} kernel> live view: {detail}\n")
                    sys.stdout.flush()
                    last_live_view_url = detail
                return

            if op.endswith(".done"):
                base_op = op[: -len(".done")]
                display_op = _format_kernel_op(base_op)
                detail = data.get("detail")
                detail_text = detail if isinstance(detail, str) else ""
                elapsed_ms = data.get("elapsed_ms")
                elapsed_prefix = ""
                if isinstance(elapsed_ms, (int, float)) and not isinstance(elapsed_ms, bool):
                    elapsed_prefix = f"[{float(elapsed_ms) / 1000:.3f}s] "
                suffix = f" {detail_text}" if detail_text else ""
                sys.stdout.write(
                    f"{_timestamp()} kernel> {elapsed_prefix}{display_op}{suffix}\n"
                )
                sys.stdout.flush()
                if base_op == "browsers.new" and detail_text:
                    last_live_view_url = detail_text
                return

            if verbose:
                sys.stdout.write(f"{_timestamp()} kernel> {op}\n")
                sys.stdout.flush()
            return

        if event_name == "prompt":
            text = data.get("text")
            if isinstance(text, str) and text:
                sys.stdout.write(f"{_timestamp()}   user> {text}\n")
                sys.stdout.flush()
            return

        if event_name == "reasoning_delta":
            text = data.get("text")
            if not isinstance(text, str):
                return
            if sys.stdout.isatty():
                spinner.start()
                spinner.add_reasoning(text)
            elif verbose and text:
                sys.stdout.write(
                    f"{_timestamp()}  agent> thinking: {_truncate_one_line(text)}\n"
                )
                sys.stdout.flush()
            return

        if event_name == "text_delta":
            spinner.stop()
            text = data.get("text")
            if not isinstance(text, str) or not text:
                return
            if not in_text:
                sys.stdout.write(f"{_timestamp()}  agent> ")
                in_text = True
            sys.stdout.write(text)
            sys.stdout.flush()
            return

        if event_name == "text_done":
            if in_text:
                sys.stdout.write("\n")
                sys.stdout.flush()
                in_text = False
            return

        if event_name == "action":
            action_type = data.get("action_type")
            description = data.get("description")
            if not isinstance(description, str) or not description:
                description = action_type if isinstance(action_type, str) else "action"
            elapsed_ms = data.get("elapsed_ms")
            elapsed_seconds = (
                float(elapsed_ms) / 1000
                if isinstance(elapsed_ms, (int, float)) and not isinstance(elapsed_ms, bool)
                else None
            )
            if in_text:
                sys.stdout.write("\n")
                in_text = False
            spinner.stop(description, elapsed_seconds=elapsed_seconds)
            return

        if event_name == "screenshot":
            if verbose:
                sys.stdout.write(f"{_timestamp()} debug> screenshot captured\n")
                sys.stdout.flush()
            return

        if event_name in ("turn_done", "run_complete"):
            spinner.stop()
            if in_text:
                sys.stdout.write("\n")
                sys.stdout.flush()
                in_text = False
            return

        if event_name == "error":
            spinner.stop()
            if in_text:
                sys.stdout.write("\n")
                sys.stdout.flush()
                in_text = False
            message = data.get("message")
            if not isinstance(message, str) or not message:
                message = "unknown error"
            sys.stderr.write(f"{_timestamp()} error> {message}\n")
            sys.stderr.flush()

    return render_text


def emit_browser_new_started(on_event: Callable[[dict], None]) -> None:
    on_event({"event": "backend", "data": {"op": "browsers.new"}})


def emit_browser_new_done(
    on_event: Callable[[dict], None], started_at: datetime, live_view_url: str | None
) -> None:
    on_event(
        {
            "event": "backend",
            "data": {
                "op": "browsers.new.done",
                "detail": live_view_url or "",
                "elapsed_ms": int((datetime.now() - started_at).total_seconds() * 1000),
            },
        }
    )


def emit_session_state(
    on_event: Callable[[dict], None], session_id: str, live_view_url: str | None
) -> None:
    on_event(
        {
            "event": "session_state",
            "data": {
                "session_id": session_id,
                "live_view_url": live_view_url or "",
            },
        }
    )


def emit_browser_delete_started(on_event: Callable[[dict], None]) -> None:
    on_event({"event": "backend", "data": {"op": "browsers.delete"}})


def emit_browser_delete_done(
    on_event: Callable[[dict], None], started_at: datetime
) -> None:
    on_event(
        {
            "event": "backend",
            "data": {
                "op": "browsers.delete.done",
                "elapsed_ms": int((datetime.now() - started_at).total_seconds() * 1000),
            },
        }
    )
