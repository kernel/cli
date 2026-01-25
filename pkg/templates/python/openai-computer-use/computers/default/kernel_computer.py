import base64
from typing import Literal

from kernel import Kernel

TYPING_DELAY_MS = 12
DEFAULT_WIDTH = 1024
DEFAULT_HEIGHT = 768

KEY_MAP = {
    "return": "Return",
    "enter": "Return",
    "Enter": "Return",
    "left": "Left",
    "right": "Right",
    "up": "Up",
    "down": "Down",
    "arrowdown": "ArrowDown",
    "arrowleft": "ArrowLeft",
    "arrowright": "ArrowRight",
    "arrowup": "ArrowUp",
    "ArrowLeft": "Left",
    "ArrowRight": "Right",
    "ArrowUp": "Up",
    "ArrowDown": "Down",
    "home": "Home",
    "end": "End",
    "pageup": "Page_Up",
    "page_up": "Page_Up",
    "PageUp": "Page_Up",
    "pagedown": "Page_Down",
    "page_down": "Page_Down",
    "PageDown": "Page_Down",
    "delete": "Delete",
    "backspace": "BackSpace",
    "Backspace": "BackSpace",
    "tab": "Tab",
    "insert": "Insert",
    "esc": "Escape",
    "escape": "Escape",
    "f1": "F1",
    "f2": "F2",
    "f3": "F3",
    "f4": "F4",
    "f5": "F5",
    "f6": "F6",
    "f7": "F7",
    "f8": "F8",
    "f9": "F9",
    "f10": "F10",
    "f11": "F11",
    "f12": "F12",
    "space": "space",
    "minus": "minus",
    "equal": "equal",
    "plus": "plus",
}

MODIFIER_MAP = {
    "ctrl": "ctrl",
    "control": "ctrl",
    "Control": "ctrl",
    "alt": "alt",
    "Alt": "alt",
    "shift": "shift",
    "Shift": "shift",
    "meta": "super",
    "Meta": "super",
    "cmd": "super",
    "command": "super",
    "win": "super",
    "super": "super",
    "option": "alt",
}


class KernelComputer:
    def __init__(
        self,
        kernel: Kernel,
        session_id: str,
        width: int = DEFAULT_WIDTH,
        height: int = DEFAULT_HEIGHT,
    ):
        self.kernel = kernel
        self.session_id = session_id
        self.width = width
        self.height = height
        self._current_url = "about:blank"
        self._screenshot_delay = 0.5

    def get_environment(self) -> Literal["browser"]:
        return "browser"

    def get_dimensions(self) -> tuple[int, int]:
        return (self.width, self.height)

    def get_current_url(self) -> str:
        return self._current_url

    def _convert_to_kernel_key(self, key: str) -> str:
        if key in MODIFIER_MAP:
            return MODIFIER_MAP[key]
        # Check special keys
        if key in KEY_MAP:
            return KEY_MAP[key]
        if key.lower() in KEY_MAP:
            return KEY_MAP[key.lower()]
        return key

    def _convert_key_combination(self, combo: str) -> str:
        if "+" in combo:
            parts = combo.split("+")
            mapped_parts = [self._convert_to_kernel_key(p.strip()) for p in parts]
            return "+".join(mapped_parts)
        return self._convert_to_kernel_key(combo)

    def screenshot(self) -> str:
        import time

        time.sleep(self._screenshot_delay)

        response = self.kernel.browsers.computer.capture_screenshot(id=self.session_id)
        screenshot_bytes = response.read()

        return base64.b64encode(screenshot_bytes).decode("utf-8")

    def click(self, x: int, y: int, button: str = "left") -> None:
        if button == "back":
            self.back()
            return
        if button == "forward":
            self.forward()
            return
        if button == "wheel":
            self.kernel.browsers.computer.scroll(
                id=self.session_id,
                x=x,
                y=y,
                delta_x=0,
                delta_y=120,
            )
            return

        btn = "right" if button == "right" else "left"
        self.kernel.browsers.computer.click_mouse(
            id=self.session_id,
            x=x,
            y=y,
            button=btn,
            click_type="click",
        )

    def double_click(self, x: int, y: int) -> None:
        self.kernel.browsers.computer.click_mouse(
            id=self.session_id,
            x=x,
            y=y,
            button="left",
            click_type="click",
            num_clicks=2,
        )

    def scroll(self, x: int, y: int, scroll_x: int, scroll_y: int) -> None:
        self.kernel.browsers.computer.scroll(
            id=self.session_id,
            x=x,
            y=y,
            delta_x=scroll_x,
            delta_y=scroll_y,
        )

    def type(self, text: str) -> None:
        self.kernel.browsers.computer.type_text(
            id=self.session_id,
            text=text,
            delay=TYPING_DELAY_MS,
        )

    def keypress(self, keys: list[str]) -> None:
        mapped_keys = [self._convert_to_kernel_key(k) for k in keys]
        combo = "+".join(mapped_keys)

        self.kernel.browsers.computer.press_key(
            id=self.session_id,
            keys=[combo],
        )

    def wait(self, ms: int = 1000) -> None:
        import time

        time.sleep(ms / 1000)

    def move(self, x: int, y: int) -> None:
        self.kernel.browsers.computer.move_mouse(
            id=self.session_id,
            x=x,
            y=y,
        )

    def drag(self, path: list[dict[str, int]]) -> None:
        if not path or len(path) < 2:
            return
        kernel_path = [[p["x"], p["y"]] for p in path]

        self.kernel.browsers.computer.drag_mouse(
            id=self.session_id,
            path=kernel_path,
            button="left",
        )

    def goto(self, url: str) -> None:
        import time
        self.kernel.browsers.computer.press_key(id=self.session_id, keys=["ctrl+l"])
        time.sleep(0.2)
        self.kernel.browsers.computer.press_key(id=self.session_id, keys=["ctrl+a"])
        time.sleep(0.1)
        self.kernel.browsers.computer.type_text(id=self.session_id, text=url, delay=TYPING_DELAY_MS)
        time.sleep(0.1)
        self.kernel.browsers.computer.press_key(id=self.session_id, keys=["Return"])
        time.sleep(1.0)
        self._current_url = url

    def back(self) -> None:
        import time
        self.kernel.browsers.computer.press_key(id=self.session_id, keys=["alt+Left"])
        time.sleep(0.5)

    def forward(self) -> None:
        import time
        self.kernel.browsers.computer.press_key(id=self.session_id, keys=["alt+Right"])
        time.sleep(0.5)
