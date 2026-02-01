"""
Utility functions for computer use RL training.

Provides image processing, coordinate conversion, and environment setup utilities.
"""

from __future__ import annotations

import base64
import io
import os
import random
from pathlib import Path

from dotenv import load_dotenv
from PIL import Image, ImageDraw

# Default max image size for VLM processing (controls token count)
DEFAULT_MAX_IMAGE_SIZE = 480

# Qwen3-VL uses 0-999 normalized coordinate space
QWEN_COORDINATE_SPACE = 999


# =============================================================================
# Image Processing
# =============================================================================


def resize_image(image: Image.Image, max_size: int = DEFAULT_MAX_IMAGE_SIZE) -> Image.Image:
    """
    Resize an image so that its longest side is at most max_size pixels.

    Preserves aspect ratio and uses LANCZOS resampling for quality.
    Returns the original image if it's already smaller than max_size.

    Args:
        image: PIL Image to resize
        max_size: Maximum size for the longest side (default: 480)

    Returns:
        Resized PIL Image
    """
    width, height = image.size
    if max(width, height) <= max_size:
        return image

    if width > height:
        new_width = max_size
        new_height = int(height * max_size / width)
    else:
        new_height = max_size
        new_width = int(width * max_size / height)

    return image.resize((new_width, new_height), Image.Resampling.LANCZOS)


def load_image(path: str | Path) -> Image.Image:
    """
    Load an image from disk and convert to RGB.

    Args:
        path: Path to the image file

    Returns:
        PIL Image in RGB mode
    """
    image = Image.open(path)
    if image.mode in ("RGBA", "LA", "P"):
        image = image.convert("RGB")
    return image


def encode_image(image: Image.Image, format: str = "JPEG", quality: int = 85) -> str:
    """
    Convert a PIL image to base64 string.

    Args:
        image: PIL Image to encode
        format: Image format (JPEG, PNG, etc.)
        quality: JPEG quality (1-100)

    Returns:
        Base64 encoded string
    """
    if image.mode == "RGBA":
        image = image.convert("RGB")
    buffered = io.BytesIO()
    image.save(buffered, format=format, quality=quality)
    return base64.b64encode(buffered.getvalue()).decode("utf-8")


def list_screenshots(screenshots_dir: Path, limit: int | None = None) -> list[Path]:
    """
    List available screenshot files.

    Args:
        screenshots_dir: Path to screenshots directory
        limit: Maximum number of files to return

    Returns:
        List of paths to screenshot files
    """
    files = sorted(screenshots_dir.glob("*.png"))
    if limit:
        files = files[:limit]
    return files


def load_random_screenshot(screenshots_dir: Path) -> tuple[Image.Image, Path]:
    """
    Load a random screenshot from the given directory.

    Args:
        screenshots_dir: Path to screenshots directory

    Returns:
        Tuple of (PIL Image, path to the image)

    Raises:
        FileNotFoundError: If no screenshots found
    """
    screenshots = list_screenshots(screenshots_dir)
    if not screenshots:
        raise FileNotFoundError(f"No screenshots found in {screenshots_dir}")
    path = random.choice(screenshots)
    return load_image(path), path


# =============================================================================
# Coordinate Conversion
# =============================================================================


def pixel_to_normalized(
    x: int,
    y: int,
    image_width: int,
    image_height: int,
) -> tuple[int, int]:
    """
    Convert pixel coordinates to normalized 0-999 coordinate space.

    Qwen3-VL uses 0-999 normalized coordinates where:
    - (0, 0) is top-left
    - (999, 999) is bottom-right

    Args:
        x: Pixel x coordinate
        y: Pixel y coordinate
        image_width: Width of the image in pixels
        image_height: Height of the image in pixels

    Returns:
        Tuple of (normalized_x, normalized_y) in 0-999 range
    """
    x = max(0, min(x, image_width - 1))
    y = max(0, min(y, image_height - 1))

    norm_x = int((x / (image_width - 1)) * QWEN_COORDINATE_SPACE)
    norm_y = int((y / (image_height - 1)) * QWEN_COORDINATE_SPACE)

    return norm_x, norm_y


def normalized_to_pixel(
    norm_x: int,
    norm_y: int,
    image_width: int,
    image_height: int,
) -> tuple[int, int]:
    """
    Convert normalized 0-999 coordinates back to pixel coordinates.

    Args:
        norm_x: Normalized x coordinate (0-999)
        norm_y: Normalized y coordinate (0-999)
        image_width: Width of the image in pixels
        image_height: Height of the image in pixels

    Returns:
        Tuple of (pixel_x, pixel_y)
    """
    pixel_x = int((norm_x / QWEN_COORDINATE_SPACE) * (image_width - 1))
    pixel_y = int((norm_y / QWEN_COORDINATE_SPACE) * (image_height - 1))

    return pixel_x, pixel_y


# =============================================================================
# Image Annotation
# =============================================================================


def add_click_overlay(
    image: Image.Image,
    x: int,
    y: int,
    box_size: int = 40,
    stroke_width: int = 3,
    color: str = "#ff0000",
) -> Image.Image:
    """
    Add a click indicator overlay (bounding box + center dot) to an image.

    The overlay shows where a click action will be performed:
    - A rectangular bounding box around the click point
    - A small filled circle at the exact click location

    Args:
        image: PIL Image to annotate
        x: X coordinate of the click (in image pixels)
        y: Y coordinate of the click (in image pixels)
        box_size: Size of the bounding box in pixels (default: 40)
        stroke_width: Width of the box outline in pixels (default: 3)
        color: Color for the overlay (default: red)

    Returns:
        New PIL Image with the click overlay drawn
    """
    annotated = image.copy()
    draw = ImageDraw.Draw(annotated)

    width, height = image.size

    # Calculate bounding box coordinates, clamped to image bounds
    left = max(0, x - box_size // 2)
    top = max(0, y - box_size // 2)
    right = min(width, x + box_size // 2)
    bottom = min(height, y + box_size // 2)

    # Draw the bounding box
    draw.rectangle(
        [(left, top), (right, bottom)],
        outline=color,
        width=stroke_width,
    )

    # Draw the center dot
    dot_radius = 4
    draw.ellipse(
        [
            (x - dot_radius, y - dot_radius),
            (x + dot_radius, y + dot_radius),
        ],
        fill=color,
    )

    return annotated


def compute_image_similarity(
    img1: Image.Image,
    img2: Image.Image,
    downsample_size: int = 100,
) -> float:
    """
    Compute similarity between two images (0.0 = completely different, 1.0 = identical).

    Uses downsampled grayscale comparison for speed and robustness to minor noise.

    Args:
        img1: First PIL Image
        img2: Second PIL Image
        downsample_size: Size to resize images to for comparison (default: 100)

    Returns:
        Float between 0.0 and 1.0 indicating similarity
    """
    size = (downsample_size, downsample_size)
    small1 = img1.resize(size, Image.Resampling.BILINEAR).convert("L")
    small2 = img2.resize(size, Image.Resampling.BILINEAR).convert("L")

    bytes1 = small1.tobytes()
    bytes2 = small2.tobytes()

    total_diff = sum(abs(p1 - p2) for p1, p2 in zip(bytes1, bytes2))
    max_diff = 255 * len(bytes1)
    normalized_diff = total_diff / max_diff

    return 1.0 - normalized_diff


# =============================================================================
# Environment Setup
# =============================================================================


def setup_environment(env_file: str | Path | None = None) -> None:
    """
    Load environment variables from .env file.

    Searches for .env file in:
    1. Provided path
    2. Current working directory
    3. Parent directories (up to 3 levels)

    Args:
        env_file: Optional explicit path to .env file

    Raises:
        EnvironmentError: If .env file not found
    """
    if env_file:
        if Path(env_file).exists():
            load_dotenv(env_file)
            return
        raise EnvironmentError(f".env file not found at {env_file}")

    # Search for .env file
    search_paths = [
        Path.cwd() / ".env",
        Path.cwd().parent / ".env",
        Path.cwd().parent.parent / ".env",
        Path.cwd().parent.parent.parent / ".env",
    ]

    for path in search_paths:
        if path.exists():
            load_dotenv(path)
            return

    raise EnvironmentError(
        ".env file not found. Create one with KERNEL_API_KEY, OPENROUTER_API_KEY, etc."
    )


def require_env(name: str) -> str:
    """
    Get a required environment variable.

    Args:
        name: Environment variable name

    Returns:
        The environment variable value

    Raises:
        EnvironmentError: If the variable is not set
    """
    value = os.getenv(name)
    if not value:
        raise EnvironmentError(f"Required environment variable {name} is not set")
    return value
