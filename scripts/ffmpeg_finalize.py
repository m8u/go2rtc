import subprocess
import os

BASE_PATH = "./recordings"


for gate_dir in os.scandir(BASE_PATH):
    for device_dir in os.scandir(f"{BASE_PATH}/{gate_dir.name}"):
        recordings = [
            r for r in os.listdir(f"{BASE_PATH}/{gate_dir.name}/{device_dir.name}")
            if r.startswith(".")
        ]
        recordings.sort()

        path = f"{BASE_PATH}/{gate_dir.name}/{device_dir.name}/{recordings[-2]}"
        finalized_path = path.replace("_raw.mp4", "_finalized.mp4")
        p = subprocess.Popen(
            [
                "ffmpeg", "-y",
                "-i", path,
                "-c", "copy",
                "-strict", "-2",
                finalized_path,
            ],
        )
        return_code = p.wait()
        if return_code > 0:
            continue

        os.remove(path)
        os.rename(
            finalized_path,
            finalized_path.replace("/.", "/").replace("_finalized.mp4", ".mp4"),
        )

print("done")
