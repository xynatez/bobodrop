"""Convert the supplied complete bobodrop logo into browser and Windows icons."""
from pathlib import Path
from shutil import copyfile

from PIL import Image, ImageOps

folder = Path(__file__).resolve().parent
source = folder / "bobodrop-source.png"
web = folder.parent / "web"
sizes = [(size, size) for size in (16, 24, 32, 48, 64, 128, 256)]

copyfile(source, folder / "bobodrop.png")
copyfile(source, web / "logo.png")
with Image.open(source) as supplied:
    image = ImageOps.contain(supplied.convert("RGBA"), (256, 256), Image.Resampling.LANCZOS)
    icon = Image.new("RGBA", (256, 256), (0, 0, 0, 0))
    icon.alpha_composite(image, ((256 - image.width) // 2, (256 - image.height) // 2))
    icon.save(folder / "bobodrop.ico", sizes=sizes)
copyfile(folder / "bobodrop.ico", web / "favicon.ico")
