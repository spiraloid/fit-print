# Fit Print — Get Your Images Ready for CatPrint

This tool takes any PNG photo or design and turns it into a file CatPrint can
print correctly, at exactly the size you want. You don't need Photoshop,
you don't need to know what DPI or CMYK mean — just follow the steps below.

## Why this matters

CatPrint (like most print shops) needs your file to already be:

1. **The exact size you're printing** — in pixels, not just inches, so the
   printer doesn't have to guess.
2. **At least 300 DPI** — enough pixel detail per inch that the print
   doesn't look blurry or blocky.
3. **In CMYK color**, not RGB — screens (RGB) and printers (CMYK) mix color
   differently. Sending CMYK avoids colors shifting or looking dull once
   printed.

If you just upload a random photo from your phone, CatPrint's system either
rejects it or silently prints it wrong. This tool fixes all three things for
you automatically.

## The easy way (no installs, no Terminal typing)

Everything you need is a ready-to-run program in [`go-tool/dist`](go-tool/dist):

| Your computer | Use this file |
|---|---|
| Windows | `prepare-for-print-windows.exe` |
| Mac with Apple Silicon (M1/M2/M3/M4) | `prepare-for-print-mac-arm64` |
| Mac with an Intel chip | `prepare-for-print-mac-intel` |

Not sure which Mac you have? Click the Apple menu (top-left) → **About This
Mac**. It'll say "Apple M1/M2/M3/M4..." (use `mac-arm64`) or "Intel..."
(use `mac-intel`).

### Windows — step by step

1. Find your PNG image(s) in Finder/File Explorer.
2. Drag the image file(s) straight on top of `prepare-for-print-windows.exe`.
   A black window will pop up and start asking questions.
3. If Windows shows a blue "Windows protected your PC" screen, click
   **More info**, then **Run anyway**. (This just means the file isn't
   digitally signed — it's not a virus warning, it's a "we don't recognize
   this publisher" notice.)
4. Answer each question by typing an answer and pressing Enter, or just
   press Enter to accept the default shown in `[brackets]`. See "What it
   will ask you" below.
5. When it says "Done," look for a new `output` folder sitting right next
   to the .exe. Your print-ready file is in there.
6. Press Enter one more time to close the window.

Prefer not to drag files? Just double-click the .exe by itself. It will
create an `input` folder next to itself and ask you to drop your images
there instead.

### Mac — step by step

Macs don't let you drag files onto a program icon the way Windows does, so
you'll use Terminal — but you never have to type a command, just drag and
drop.

1. Open **Terminal**: press `Cmd + Space`, type `Terminal`, press Enter.
2. Drag the right file for your Mac (`prepare-for-print-mac-arm64` or
   `prepare-for-print-mac-intel`) from Finder into the Terminal window.
   Its full file path will appear as text.
3. Drag your PNG image(s) into that same Terminal line too, after the
   program's path (a space appears automatically between each item you
   drop).
4. Click into the Terminal window and press Enter.
5. **First time only:** macOS will likely block it with a message like
   *"cannot be opened because the developer cannot be verified."* Go to
   **System Settings → Privacy & Security**, scroll down, and click
   **Open Anyway** next to that message. Then repeat steps 2–4.
6. Answer each question by typing an answer and pressing Enter, or just
   press Enter to accept the default shown in `[brackets]`.
7. When it says "Done," look for a new `output` folder next to the program
   file. Your print-ready file is in there.

Prefer not to drag your images in step 3? Just run the program by itself.
It will create an `input` folder next to itself and ask you to drop your
images there instead, then press Enter to continue.

## What it will ask you

Every question has a sensible default already filled in — if you're not
sure, just press Enter.

| Question | What to answer |
|---|---|
| Target width / height | The exact size you're printing, e.g. `4` and `6` for a 4x6 print. |
| Units | `in` for inches or `mm` for millimeters. |
| Target DPI | Leave as `300` — that's CatPrint's minimum. Don't go lower. |
| Fit mode | `fit` = shrink/pad to show the whole image with no cropping (safest default). `crop` = fills the whole print, trimming the edges. `stretch` = forces the exact size, which can squish or stretch the image — only use this if you want that effect. |
| Convert to CMYK? | Leave as `Y` (yes). This is the setting CatPrint needs. |

## Reading the warning, if you see one

If your image is low-resolution for the size you asked for, you'll see a
warning like:

```
WARNING: 'photo.png' is being upscaled beyond its safe print size.
  Maximum safe print size at 300 DPI: 3.20in x 4.80in
```

That means your original photo doesn't have enough detail to print sharply
at the size you requested — printing it that big will look blurry or
pixelated. The line above tells you the largest size that *will* look sharp
at 300 DPI. The tool still finishes the job either way; it's your call
whether to shrink the print size or use a higher-resolution source photo.

## Where your finished file ends up

Look in the `output` folder that appears next to the program you ran.

- If you kept the default "Convert to CMYK? Yes," your file will end in
  `_print.tiff`. Upload that to CatPrint.
- If you chose "No" (kept RGB), your file will end in `_print.png` instead.

## For developers

There's also a plain Python version of this tool
([`prepare_for_print.py`](prepare_for_print.py)) if you'd rather run it from
the command line or import it into another project — see the comments at
the top of that file and [`requirements.txt`](requirements.txt). The Go
version's source and rebuild instructions are in
[`go-tool/README.md`](go-tool/README.md).
