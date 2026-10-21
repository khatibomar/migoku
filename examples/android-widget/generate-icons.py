#!/usr/bin/env python3
"""Generate the Android launcher icons from assets/migaku-icon.png.

The source artwork is a square, full-body character on an orange-to-red
gradient. Android's adaptive icons want the artwork inside a 72dp mask and the
recommended 66dp safe zone, so the art has to sit on a background that blends
seamlessly with the artwork's own edges.

To do that we model the artwork's background as a bilinear gradient, then:

  * ic_launcher_background - that gradient extended across the whole 108dp
    layer, so it continues past the artwork's square edge;
  * ic_launcher_foreground - the artwork scaled into the 66dp safe zone.

Because the artwork *is* that gradient, the two layers meet without a seam.

Legacy PNGs (ic_launcher.png / ic_launcher_round.png) are generated too, for
launchers and the widget picker that do not use adaptive icons.

The notification small icon is generated too: Android masks small icons to a
white silhouette and tints them itself, so the colours cannot survive. What can
survive is the shape, so ic_notification.png is the character keyed out of its
gradient background, drawn in opaque white on transparency.

Usage:  python3 generate-icons.py [--source assets/migaku-icon.png]
"""

import argparse
import os

import numpy as np
from PIL import Image, ImageDraw

# Adaptive icon geometry, in dp.
CANVAS_DP = 108
MASK_DP = 72
SAFE_DP = 66

# Launcher icon sizes per density bucket.
LEGACY_DP = {"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}
# Adaptive layer sizes per density bucket (108dp).
CANVAS_PX = {"mdpi": 108, "hdpi": 162, "xhdpi": 216, "xxhdpi": 324, "xxxhdpi": 432}
# Notification small icon sizes per density bucket (24dp).
NOTIF_PX = {"mdpi": 24, "hdpi": 36, "xhdpi": 48, "xxhdpi": 72, "xxxhdpi": 96}

# RGB distance from the modelled background below which a pixel counts as
# background and above which it counts as character. The distance histogram is
# strongly bimodal - background sits under ~40, the character starts around
# ~150 - so everything in between is the artwork's own anti-aliased edge.
SILHOUETTE_BG = 40.0
SILHOUETTE_FG = 120.0


def fit_background(pixels):
    """Model the artwork's background as a bilinear gradient.

    The background is sampled from a border ring, skipping pixels that are not
    part of it (the character touches the bottom-left edge).
    """
    h, w, _ = pixels.shape
    ys, xs = np.mgrid[0:h, 0:w]

    ring = (xs < 14) | (xs >= w - 14) | (ys < 14) | (ys >= h - 14)
    warm = (pixels[..., 0] > 235) & (pixels[..., 1] < 130) & (pixels[..., 2] < 80)
    sel = ring & warm
    if sel.sum() < 100:
        raise SystemExit("could not find enough background pixels to model the gradient")

    design = np.stack([np.ones(sel.sum()), xs[sel], ys[sel], xs[sel] * ys[sel]], axis=1)
    coefficients = []
    for channel in range(3):
        coef, *_ = np.linalg.lstsq(design, pixels[..., channel][sel], rcond=None)
        coefficients.append(coef)
    return coefficients, w, h


def gradient_layer(coefficients, source_w, source_h, canvas_px, safe_px):
    """The background gradient extended over the whole adaptive canvas."""
    ys, xs = np.mgrid[0:canvas_px, 0:canvas_px]
    offset = (canvas_px - safe_px) // 2

    # Map every canvas pixel back into the artwork's coordinate space, so the
    # gradient lines up exactly with the artwork's own edges.
    ax = (xs - offset) / safe_px * source_w
    ay = (ys - offset) / safe_px * source_h

    channels = []
    for coef in coefficients:
        values = coef[0] + coef[1] * ax + coef[2] * ay + coef[3] * ax * ay
        channels.append(np.clip(values, 0, 255).astype(np.uint8))
    rgb = np.stack(channels, axis=-1)
    return Image.fromarray(np.dstack([rgb, np.full(rgb.shape[:2], 255, np.uint8)]), "RGBA")


def legacy_icon(art, size, round_icon=False):
    """The artwork as a standalone launcher icon."""
    icon = art.resize((size, size), Image.LANCZOS)
    mask = Image.new("L", (size, size), 0)
    draw = ImageDraw.Draw(mask)
    if round_icon:
        draw.ellipse([0, 0, size - 1, size - 1], fill=255)
    else:
        draw.rounded_rectangle([0, 0, size - 1, size - 1], radius=int(size * 0.18), fill=255)
    icon.putalpha(mask)
    return icon


def background_distance(pixels, coefficients):
    """Per-pixel RGB distance from the modelled gradient background."""
    h, w, _ = pixels.shape
    ys, xs = np.mgrid[0:h, 0:w]
    predicted = np.stack(
        [c[0] + c[1] * xs + c[2] * ys + c[3] * xs * ys for c in coefficients],
        axis=-1,
    )
    return np.linalg.norm(pixels[..., :3] - predicted, axis=-1)


def notification_icon(pixels, coefficients, size):
    """The character as a white silhouette: the app icon for a status bar.

    Android masks a notification's small icon to a silhouette and tints it
    itself, so the artwork's colours cannot survive. Keying the character out
    of its gradient background does survive, and keeps the notification
    recognisable as this app instead of a generic blob.
    """
    distance = background_distance(pixels, coefficients)
    alpha = np.clip(
        (distance - SILHOUETTE_BG) / (SILHOUETTE_FG - SILHOUETTE_BG), 0.0, 1.0
    )
    mask = Image.fromarray((alpha * 255).astype(np.uint8), "L").resize(
        (size, size), Image.LANCZOS
    )
    icon = Image.new("RGBA", (size, size), (255, 255, 255, 255))
    icon.putalpha(mask)
    return icon


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", default="assets/migaku-icon.png")
    parser.add_argument("--res", default="app/src/main/res")
    args = parser.parse_args()

    art = Image.open(args.source).convert("RGBA")
    pixels = np.asarray(art).astype(np.float64)
    coefficients, source_w, source_h = fit_background(pixels)

    for density, canvas_px in CANVAS_PX.items():
        out_dir = os.path.join(args.res, f"mipmap-{density}")
        os.makedirs(out_dir, exist_ok=True)

        safe_px = canvas_px * SAFE_DP // CANVAS_DP
        offset = (canvas_px - safe_px) // 2

        background = gradient_layer(coefficients, source_w, source_h, canvas_px, safe_px)
        background.save(os.path.join(out_dir, "ic_launcher_background.png"))

        foreground = Image.new("RGBA", (canvas_px, canvas_px), (0, 0, 0, 0))
        foreground.paste(art.resize((safe_px, safe_px), Image.LANCZOS), (offset, offset))
        foreground.save(os.path.join(out_dir, "ic_launcher_foreground.png"))

        legacy_size = LEGACY_DP[density]
        legacy_icon(art, legacy_size).save(os.path.join(out_dir, "ic_launcher.png"))
        legacy_icon(art, legacy_size, round_icon=True).save(
            os.path.join(out_dir, "ic_launcher_round.png")
        )

        notif_size = NOTIF_PX[density]
        notif_dir = os.path.join(args.res, f"drawable-{density}")
        os.makedirs(notif_dir, exist_ok=True)
        notification_icon(pixels, coefficients, notif_size).save(
            os.path.join(notif_dir, "ic_notification.png")
        )

        print(
            f"mipmap-{density}: adaptive {canvas_px}x{canvas_px}, legacy {legacy_size}x{legacy_size}, "
            f"notification {notif_size}x{notif_size}"
        )


if __name__ == "__main__":
    main()
