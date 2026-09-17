repo setup prompt:

Cursor Agentic System Prompt: Print-Ready PNG Preparation Script
Paste the block below into Cursor to have it write (or add to an existing codebase) a Python script that prepares PNGs for print, tuned to CatPrint's file requirements.

You are building a Python utility for print-ready image preparation, specifically targeting CatPrint's requirements. Create a script called prepare_for_print.py with the following behavior.

Core function: take an input PNG of any pixel resolution and convert it to a print-ready file at an exact physical size and resolution. Accept parameters for target width, target height, units of inches or millimeters, and target DPI, defaulting to 300 DPI since that is CatPrint's stated minimum.

Resampling: calculate the required pixel dimensions from the physical size and DPI, and actually resample the image to match, using high-quality resampling such as Lanczos. Do not rely on metadata alone. If the source image is lower resolution than required and would need to be upscaled significantly, print a clear warning to the console stating the maximum safe print size at 300 DPI for that source image, citing that CatPrint recommends at least 300 DPI at final size to avoid pixelation.

Fit mode: support three modes selectable by a flag:

fit — pads to preserve aspect ratio
crop — fills the exact dimensions by cropping
stretch — distorts the image to hit the exact dimensions
Make fit the default.

Color mode: convert the final image to CMYK before saving, since CatPrint prints in CMYK, not RGB, and this avoids color shifts. Make this the default behavior, with a flag to keep RGB if the user wants to override it.

Metadata: embed the correct DPI value into the output file's metadata so it matches the resampled pixel dimensions.

Interface: build this as both a command line tool, accepting a single file path, and as an importable function, plus support a folder input to batch process multiple PNGs at once with the same settings.

Output: save results into a new output folder rather than overwriting originals, and print a summary for each file showing original resolution, final resolution, final DPI, and final physical size.

Use Pillow as the primary dependency. Include clear docstrings and inline comments so the logic is easy to audit later.