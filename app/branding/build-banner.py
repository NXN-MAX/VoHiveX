"""Render the README banner with the bundled OFL Jost font (Pillow)."""
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont
root = Path(__file__).resolve().parents[2]
image = Image.new('RGBA', (2880, 720))
draw = ImageDraw.Draw(image)
draw.rounded_rectangle((0, 0, 2879, 719), radius=80, fill='#9FE870')
font = ImageFont.truetype(str(root/'app/branding/Jost-Italic.ttf'), 300)
font.set_variation_by_axes([800])
x = 172
for letter in 'VoHiveX':
    draw.text((x, 424), letter, font=font, anchor='ls', fill='#397A16' if letter=='X' else '#163300')
    x += draw.textlength(letter, font=font)
# Use the same bundled font for the secondary wordmark, avoiding system fonts.
small = ImageFont.truetype(str(root/'app/branding/Jost-Italic.ttf'), 36)
small.set_variation_by_axes([500])
x = 180
for letter in 'PERSONAL MODEM WORKSPACE':
    draw.text((x, 552), letter, font=small, anchor='ls', fill='#24540D')
    x += draw.textlength(letter, font=small)+8
ink = '#163300'
draw.line([(2400,550),(2400,166),(2560,166),(2680,286),(2680,550),(2400,550)], fill=ink, width=14, joint='curve')
draw.rounded_rectangle((2460,332,2620,464), radius=24, outline=ink, width=14)
for x in [2512,2568]:
    draw.line((x,332,x,464), fill=ink, width=8)
draw.line((2460,398,2620,398), fill=ink, width=8)
out = root/'docs/images/vohivex-banner.png'
out.parent.mkdir(parents=True, exist_ok=True)
image.resize((1440,360), Image.Resampling.LANCZOS).save(out)
