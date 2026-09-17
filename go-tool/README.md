# Print-Ready Image Preparation (Go tool)

A double-click-able tool that prepares PNG images for print, tuned to
CatPrint's requirements (300 DPI minimum, CMYK color). No installs, no
command line knowledge needed — just Mac or Windows.

Pre-built programs are in `dist/`:

- `prepare-for-print-mac-arm64` — Mac with Apple Silicon (M1/M2/M3/M4)
- `prepare-for-print-mac-intel` — Mac with an Intel chip
- `prepare-for-print-windows.exe` — Windows (64-bit)

Not sure which Mac chip you have? Apple menu → About This Mac. If it says
"Apple M1/M2/M3/M4," use `mac-arm64`. If it says "Intel," use `mac-intel`.

## How to run it

### Windows

1. Put your PNG files somewhere handy.
2. Drag your PNG file(s) directly onto `prepare-for-print-windows.exe`.
   A black window will pop up.
3. Answer the questions it asks (or just press Enter to accept the
   default shown in brackets).
4. When it finishes, look for a new `output` folder next to the .exe —
   your print-ready files are there.
5. Press Enter again to close the window.

If you'd rather not drag files: just double-click the .exe. It will make an
`input` folder next to itself and ask you to put your PNGs there.

### macOS

Dragging files onto a program icon doesn't work the same way on a Mac, so:

1. Open **Terminal** (Spotlight search → type "Terminal" → Enter).
2. Drag the `prepare-for-print-mac-arm64` (or `-mac-intel`) file from Finder
   into the Terminal window — this types out its full path for you.
3. Optional: also drag your PNG file(s) into the same Terminal line,
   after the program's path (a space is added automatically between
   dragged items).
4. Press Enter to run it.
5. The **first time** you run it, macOS will likely block it with a
   security warning ("cannot be opened because the developer cannot be
   verified"). Go to **System Settings → Privacy & Security**, scroll down,
   and click **"Open Anyway"** next to the message about this program.
   Run the same command again afterward.
6. Answer the questions it asks (or press Enter for the defaults).
7. Your finished files appear in an `output` folder next to the program.

If you didn't drag any PNGs in step 3, the program creates an `input`
folder next to itself — drop your PNGs there and press Enter to continue.

## What it asks you

- **Target width / height** — the final printed size (defaults: 4 x 6).
- **Units** — `in` (inches) or `mm` (millimeters).
- **Target DPI** — resolution at final print size. CatPrint's minimum is
  **300**, which is the default.
- **Fit mode**:
  - `fit` (default) — shrinks/grows the image to fit inside the target
    size, padding any leftover space with white. Nothing gets cropped or
    stretched.
  - `crop` — fills the exact target size completely, cropping any
    overflow. Nothing gets stretched.
  - `stretch` — forces the image to the exact target size, distorting it
    if the aspect ratio doesn't match.
- **Convert to CMYK?** — Yes (default) converts colors to CMYK, the color
  mode printers actually use (avoids color shifts). CMYK files are saved
  as `.tiff` since regular PNGs can't store CMYK. Choosing No keeps RGB and
  saves as `.png`.

If your source image is too low-resolution for the size/DPI you asked for,
the program prints a clear warning telling you the largest size you can
safely print at 300 DPI without visible pixelation.

## Rebuilding from source

Requires the [Go toolchain](https://go.dev/dl/). From this folder:

```bash
go build .                # builds a binary for your current machine
./build.sh                # cross-compiles mac-arm64, mac-intel, and windows.exe into dist/
```

Only third-party dependency: `github.com/disintegration/imaging` (pure Go,
no cgo), so it cross-compiles cleanly for any target from any machine.
