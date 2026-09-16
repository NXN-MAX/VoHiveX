"""Render the VoX favicon from the bundled, unmodified OFL Jost font."""
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont

root = Path(__file__).resolve().parent
size = 1024
font = ImageFont.truetype(str(root / 'Jost-Italic.ttf'), 600)
font.set_variation_by_axes([800])
box = font.getbbox('VoX')
font = ImageFont.truetype(str(root / 'Jost-Italic.ttf'), round(600 * 0.78 * size / (box[2] - box[0])))
font.set_variation_by_axes([800])
box = font.getbbox('VoX')
icon = Image.new('RGBA', (size, size))
draw = ImageDraw.Draw(icon)
draw.rounded_rectangle((0, 0, size - 1, size - 1), radius=200, fill='#9FE870')
draw.text(((size - box[2] - box[0]) / 2, (size - box[3] - box[1]) / 2), 'VoX', font=font, fill='#000000')
icon.save(root / 'vohivex-logo.png')
icon.resize((512, 512), Image.Resampling.LANCZOS).save(root / 'vohivex-icon.png')
icon.save(root / 'vohivex-favicon.ico', sizes=[(n, n) for n in (16, 24, 32, 48, 64, 128, 256)])
