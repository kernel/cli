"""
Kernel Screenshot Provider.

Implements the AsyncImageProvider protocol using Kernel's Computer Controls API.
"""

import io
from typing import TYPE_CHECKING

from PIL import Image as PILImage

if TYPE_CHECKING:
    from kernel_session import KernelBrowserSession


class KernelImage:
    """
    Image wrapper that implements the oagi Image protocol.

    The Image protocol only requires a `read() -> bytes` method.
    """

    def __init__(
        self, data: bytes, width: int | None = None, height: int | None = None
    ):
        self._data = data
        self._width = width
        self._height = height

    def read(self) -> bytes:
        """Read the image data as bytes."""
        return self._data

    def resize(self, width: int, height: int) -> "KernelImage":
        """Resize the image to the specified dimensions."""
        # Load image from bytes
        img = PILImage.open(io.BytesIO(self._data))

        # Resize
        resized = img.resize((width, height), PILImage.Resampling.LANCZOS)

        # Convert back to bytes (JPEG for smaller size)
        buffer = io.BytesIO()
        # Convert RGBA to RGB if needed for JPEG
        if resized.mode == "RGBA":
            rgb_image = PILImage.new("RGB", resized.size, (255, 255, 255))
            rgb_image.paste(resized, mask=resized.split()[3])
            resized = rgb_image
        resized.save(buffer, format="JPEG", quality=85)

        return KernelImage(buffer.getvalue(), width, height)


class KernelScreenshotProvider:
    """
    Screenshot provider using Kernel's Computer Controls API.

    Implements the AsyncImageProvider protocol:
    - __call__() -> Image: Capture and return a new screenshot
    - last_image() -> Image: Return the last captured screenshot
    """

    def __init__(
        self,
        session: "KernelBrowserSession",
        resize_width: int = 1260,
        resize_height: int = 700,
    ):
        """
        Initialize the screenshot provider.

        Args:
            session: The Kernel browser session to capture from
            resize_width: Target width for screenshots (Lux default: 1260)
            resize_height: Target height for screenshots (Lux default: 700)
        """
        self.session = session
        self.resize_width = resize_width
        self.resize_height = resize_height
        self._last_image: KernelImage | None = None

    async def __call__(self) -> KernelImage:
        """
        Capture a screenshot from the Kernel browser.

        Returns:
            KernelImage: The captured screenshot
        """
        if not self.session.session_id:
            raise RuntimeError("Browser session not initialized")

        # Capture screenshot using Kernel Computer Controls API
        response = self.session.kernel.browsers.computer.capture_screenshot(
            id=self.session.session_id
        )

        # Read the raw bytes from the response
        raw_bytes = response.read()

        # Create KernelImage and resize for Lux model
        image = KernelImage(raw_bytes)

        # Resize to the expected dimensions for Lux
        if self.resize_width and self.resize_height:
            image = image.resize(self.resize_width, self.resize_height)

        # Cache the last image
        self._last_image = image

        return image

    async def last_image(self) -> KernelImage:
        """
        Return the last captured screenshot.

        If no screenshot has been taken yet, takes a new one.

        Returns:
            KernelImage: The last captured screenshot
        """
        if self._last_image is None:
            return await self()
        return self._last_image
