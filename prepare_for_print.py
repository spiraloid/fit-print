#!/usr/bin/env python3
"""
prepare_for_print.py

Print-ready PNG preparation utility, tuned to CatPrint's file requirements
(300 DPI minimum, CMYK color mode).

This module can be used two ways:

1. As a command line tool:

    python prepare_for_print.py INPUT [OPTIONS]

   INPUT may be a single PNG file or a folder of PNGs (batch mode).

2. As an importable function:

    from prepare_for_print import prepare_for_print
    prepare_for_print("photo.png", width=4, height=6, units="in", dpi=300)

Dependencies: Pillow (PIL). Install with `pip install Pillow`.
"""

from __future__ import annotations

import argparse
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

from PIL import Image

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

# CatPrint's stated minimum resolution for print-ready files.
DEFAULT_DPI = 300

# Millimeters per inch, used for unit conversion.
MM_PER_INCH = 25.4

# Fit modes supported by this tool.
FIT_MODES = ("fit", "crop", "stretch")


# ---------------------------------------------------------------------------
# Data container for a single file's result, used for the summary printout.
# ---------------------------------------------------------------------------

@dataclass
class PrintResult:
    """Holds the before/after facts about one processed image."""

    source_path: Path
    output_path: Path
    original_pixels: tuple[int, int]
    final_pixels: tuple[int, int]
    final_dpi: int
    width_units: float
    height_units: float
    units: str
    max_safe_size_units: float | None = None  # only set when a warning fires


# ---------------------------------------------------------------------------
# Core geometry helpers
# ---------------------------------------------------------------------------

def _to_inches(value: float, units: str) -> float:
    """Convert a physical dimension to inches given its unit label."""
    units = units.lower()
    if units in ("in", "inch", "inches"):
        return value
    if units in ("mm", "millimeter", "millimeters"):
        return value / MM_PER_INCH
    raise ValueError(f"Unsupported units: {units!r}. Use 'in' or 'mm'.")


def _target_pixel_size(width: float, height: float, units: str, dpi: int) -> tuple[int, int]:
    """
    Calculate the exact pixel dimensions required to print at `width` x `height`
    (in the given units) at `dpi` dots per inch.

    We do the math ourselves rather than trusting any embedded DPI metadata on
    the source image, since that metadata is frequently wrong or absent.
    """
    width_in = _to_inches(width, units)
    height_in = _to_inches(height, units)
    px_w = round(width_in * dpi)
    px_h = round(height_in * dpi)
    return px_w, px_h


def _max_safe_print_size_inches(source_px: tuple[int, int], dpi: int) -> tuple[float, float]:
    """
    Given a source image's native pixel dimensions, return the largest physical
    size (in inches) it can be printed at without dropping below `dpi`.

    This is used to build the "you are upscaling too far" warning.
    """
    src_w, src_h = source_px
    return src_w / dpi, src_h / dpi


# ---------------------------------------------------------------------------
# Resampling / fit-mode logic
# ---------------------------------------------------------------------------

def _resize_stretch(img: Image.Image, target_px: tuple[int, int]) -> Image.Image:
    """Distort the image to exactly fill the target pixel dimensions."""
    return img.resize(target_px, resample=Image.LANCZOS)


def _resize_crop(img: Image.Image, target_px: tuple[int, int]) -> Image.Image:
    """
    Scale the image up (preserving aspect ratio) until it fully covers the
    target dimensions, then center-crop the overflow so the final image
    exactly matches target_px with no distortion and no padding.
    """
    target_w, target_h = target_px
    src_w, src_h = img.size

    scale = max(target_w / src_w, target_h / src_h)
    scaled_size = (round(src_w * scale), round(src_h * scale))
    scaled = img.resize(scaled_size, resample=Image.LANCZOS)

    # Center-crop to exact target size.
    left = (scaled_size[0] - target_w) // 2
    top = (scaled_size[1] - target_h) // 2
    return scaled.crop((left, top, left + target_w, top + target_h))


def _resize_fit(img: Image.Image, target_px: tuple[int, int]) -> Image.Image:
    """
    Scale the image down/up (preserving aspect ratio) until it fits entirely
    within the target dimensions, then pad the remaining space (centered) so
    the final canvas exactly matches target_px. Padding color is white, which
    is the safe default for print backgrounds.
    """
    target_w, target_h = target_px
    src_w, src_h = img.size

    scale = min(target_w / src_w, target_h / src_h)
    scaled_size = (round(src_w * scale), round(src_h * scale))
    scaled = img.resize(scaled_size, resample=Image.LANCZOS)

    # Build a white canvas at the exact target size and paste the scaled
    # image centered within it.
    canvas_mode = "RGBA" if scaled.mode in ("RGBA", "LA") else "RGB"
    background = (255, 255, 255, 255) if canvas_mode == "RGBA" else (255, 255, 255)
    canvas = Image.new(canvas_mode, target_px, background)

    if scaled.mode != canvas_mode:
        scaled = scaled.convert(canvas_mode)

    paste_x = (target_w - scaled_size[0]) // 2
    paste_y = (target_h - scaled_size[1]) // 2

    if canvas_mode == "RGBA":
        canvas.paste(scaled, (paste_x, paste_y), scaled)
    else:
        canvas.paste(scaled, (paste_x, paste_y))

    return canvas


_FIT_DISPATCH = {
    "fit": _resize_fit,
    "crop": _resize_crop,
    "stretch": _resize_stretch,
}


# ---------------------------------------------------------------------------
# Color mode conversion
# ---------------------------------------------------------------------------

def _to_cmyk(img: Image.Image) -> Image.Image:
    """
    Convert an image to CMYK.

    Pillow's built-in RGB -> CMYK conversion is a naive/uncalibrated
    conversion (no ICC profile math), which will not perfectly match a print
    house's color separation but is the standard, dependency-free way to get
    a CMYK-mode PNG/TIFF out of Pillow. Flattening to RGB first ensures any
    alpha channel is composited onto a white background before conversion,
    since CMYK has no alpha channel.
    """
    if img.mode == "CMYK":
        return img

    if img.mode in ("RGBA", "LA") or (img.mode == "P" and "transparency" in img.info):
        rgba = img.convert("RGBA")
        flattened = Image.new("RGB", rgba.size, (255, 255, 255))
        flattened.paste(rgba, mask=rgba.split()[-1])
        img = flattened
    elif img.mode != "RGB":
        img = img.convert("RGB")

    return img.convert("CMYK")


# ---------------------------------------------------------------------------
# Main per-file processing function
# ---------------------------------------------------------------------------

def prepare_for_print(
    input_path: str | Path,
    output_dir: str | Path = "output",
    width: float = 4.0,
    height: float = 6.0,
    units: str = "in",
    dpi: int = DEFAULT_DPI,
    fit_mode: str = "fit",
    keep_rgb: bool = False,
) -> PrintResult:
    """
    Prepare a single PNG for print and save it into `output_dir`.

    Args:
        input_path: path to the source PNG.
        output_dir: folder to write the processed file into (created if needed).
        width: target physical width.
        height: target physical height.
        units: "in" (inches) or "mm" (millimeters).
        dpi: target resolution in dots per inch. CatPrint's stated minimum is 300.
        fit_mode: one of "fit" (pad to preserve aspect ratio), "crop" (fill and
            crop), or "stretch" (distort to exact size).
        keep_rgb: if True, skip the CMYK conversion and keep RGB output.
            CatPrint prints in CMYK, so the default (False) converts to CMYK.

    Returns:
        A PrintResult summarizing the operation, for reporting.
    """
    if fit_mode not in FIT_MODES:
        raise ValueError(f"fit_mode must be one of {FIT_MODES}, got {fit_mode!r}")

    input_path = Path(input_path)
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    img = Image.open(input_path)
    original_px = img.size

    # 1. Compute exact required pixel dimensions from physical size + DPI.
    #    We never trust the source file's embedded DPI for this calculation.
    target_px = _target_pixel_size(width, height, units, dpi)

    # 2. Warn if we would be upscaling significantly beyond what the source
    #    resolution safely supports at the target DPI.
    max_safe_w_in, max_safe_h_in = _max_safe_print_size_inches(original_px, dpi)
    requested_w_in = _to_inches(width, units)
    requested_h_in = _to_inches(height, units)

    upscale_factor_w = requested_w_in / max_safe_w_in if max_safe_w_in else float("inf")
    upscale_factor_h = requested_h_in / max_safe_h_in if max_safe_h_in else float("inf")
    max_safe_units = None

    # "Significantly" upscaling = requesting more than ~10% beyond the size
    # the source can natively support at the target DPI.
    if upscale_factor_w > 1.1 or upscale_factor_h > 1.1:
        # Report the max safe size back in the user's requested units.
        if units.lower() in ("mm", "millimeter", "millimeters"):
            max_safe_w = max_safe_w_in * MM_PER_INCH
            max_safe_h = max_safe_h_in * MM_PER_INCH
        else:
            max_safe_w = max_safe_w_in
            max_safe_h = max_safe_h_in

        max_safe_units = min(max_safe_w, max_safe_h)
        print(
            f"WARNING: '{input_path.name}' is being upscaled beyond its safe "
            f"print size.\n"
            f"  Source resolution: {original_px[0]}x{original_px[1]} px\n"
            f"  Requested size: {width}{units} x {height}{units} @ {dpi} DPI\n"
            f"  Maximum safe print size at {dpi} DPI: "
            f"{max_safe_w:.2f}{units} x {max_safe_h:.2f}{units}\n"
            f"  CatPrint recommends at least {dpi} DPI at final size to avoid "
            f"pixelation."
        )

    # 3. Resample to the exact target pixel dimensions using the selected
    #    fit mode, with high-quality Lanczos resampling.
    resized = _FIT_DISPATCH[fit_mode](img, target_px)

    # 4. Convert color mode. CMYK is the default since CatPrint prints in CMYK.
    final_img = resized if keep_rgb else _to_cmyk(resized)

    # 5. Determine output filename and format.
    #    CMYK images cannot be saved as PNG (PNG has no CMYK support), so we
    #    save those as TIFF instead, which faithfully preserves CMYK data and
    #    DPI metadata. RGB output remains PNG.
    if final_img.mode == "CMYK":
        out_name = input_path.stem + "_print.tiff"
    else:
        out_name = input_path.stem + "_print.png"
    output_path = output_dir / out_name

    # 6. Embed the correct DPI metadata so it matches the resampled pixels.
    final_img.save(output_path, dpi=(dpi, dpi))

    # 7. Compute final physical size for the summary.
    final_w_units = target_px[0] / dpi
    final_h_units = target_px[1] / dpi
    if units.lower() in ("mm", "millimeter", "millimeters"):
        final_w_units *= MM_PER_INCH
        final_h_units *= MM_PER_INCH

    return PrintResult(
        source_path=input_path,
        output_path=output_path,
        original_pixels=original_px,
        final_pixels=target_px,
        final_dpi=dpi,
        width_units=round(final_w_units, 3),
        height_units=round(final_h_units, 3),
        units=units,
        max_safe_size_units=max_safe_units,
    )


# ---------------------------------------------------------------------------
# Batch processing
# ---------------------------------------------------------------------------

def _iter_pngs(folder: Path) -> Iterable[Path]:
    """Yield all PNG files directly inside `folder` (non-recursive), sorted."""
    return sorted(p for p in folder.iterdir() if p.suffix.lower() == ".png" and p.is_file())


def prepare_folder_for_print(
    input_dir: str | Path,
    output_dir: str | Path = "output",
    **kwargs,
) -> list[PrintResult]:
    """
    Batch-process every PNG in `input_dir` with the same settings, saving
    results into `output_dir`. Extra keyword args are forwarded to
    `prepare_for_print` (width, height, units, dpi, fit_mode, keep_rgb).
    """
    input_dir = Path(input_dir)
    results = []
    for png_path in _iter_pngs(input_dir):
        result = prepare_for_print(png_path, output_dir=output_dir, **kwargs)
        results.append(result)
    return results


# ---------------------------------------------------------------------------
# Summary printing
# ---------------------------------------------------------------------------

def _print_summary(result: PrintResult) -> None:
    """Print a human-readable summary of one processed file."""
    orig_w, orig_h = result.original_pixels
    final_w, final_h = result.final_pixels
    print(
        f"{result.source_path.name} -> {result.output_path.name}\n"
        f"  Original resolution: {orig_w}x{orig_h} px\n"
        f"  Final resolution:    {final_w}x{final_h} px\n"
        f"  Final DPI:            {result.final_dpi}\n"
        f"  Final physical size:  {result.width_units}{result.units} x "
        f"{result.height_units}{result.units}"
    )


# ---------------------------------------------------------------------------
# Command line interface
# ---------------------------------------------------------------------------

def _build_arg_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Prepare PNG(s) for print at an exact physical size and DPI, "
            "tuned to CatPrint's requirements (300 DPI minimum, CMYK color)."
        )
    )
    parser.add_argument(
        "input",
        help="Path to a single PNG file, or a folder of PNGs for batch mode.",
    )
    parser.add_argument(
        "-o", "--output-dir", default="output",
        help="Folder to write processed file(s) into (default: ./output).",
    )
    parser.add_argument(
        "--width", type=float, default=4.0,
        help="Target physical width (default: 4.0).",
    )
    parser.add_argument(
        "--height", type=float, default=6.0,
        help="Target physical height (default: 6.0).",
    )
    parser.add_argument(
        "--units", choices=["in", "mm"], default="in",
        help="Units for width/height (default: in).",
    )
    parser.add_argument(
        "--dpi", type=int, default=DEFAULT_DPI,
        help=f"Target DPI (default: {DEFAULT_DPI}, CatPrint's stated minimum).",
    )
    parser.add_argument(
        "--fit-mode", choices=list(FIT_MODES), default="fit",
        help="How to fit the source image into the target size (default: fit).",
    )
    parser.add_argument(
        "--keep-rgb", action="store_true",
        help="Keep RGB color mode instead of converting to CMYK.",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = _build_arg_parser()
    args = parser.parse_args(argv)

    input_path = Path(args.input)
    if not input_path.exists():
        print(f"ERROR: input path does not exist: {input_path}", file=sys.stderr)
        return 1

    common_kwargs = dict(
        width=args.width,
        height=args.height,
        units=args.units,
        dpi=args.dpi,
        fit_mode=args.fit_mode,
        keep_rgb=args.keep_rgb,
    )

    if input_path.is_dir():
        results = prepare_folder_for_print(input_path, output_dir=args.output_dir, **common_kwargs)
        if not results:
            print(f"No PNG files found in {input_path}")
            return 0
        for result in results:
            _print_summary(result)
            print()
        print(f"Processed {len(results)} file(s) -> {args.output_dir}/")
    else:
        result = prepare_for_print(input_path, output_dir=args.output_dir, **common_kwargs)
        _print_summary(result)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
