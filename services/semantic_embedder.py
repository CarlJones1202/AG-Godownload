import json
import sys
import traceback

import numpy as np
import onnxruntime as ort
from PIL import Image, ImageOps

IMAGE_SIZE = 224
OUTPUT_DIM = 768


def load_session(model_path):
    options = ort.SessionOptions()
    options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
    session = ort.InferenceSession(
        model_path,
        sess_options=options,
        providers=["CPUExecutionProvider"],
    )
    input_names = [item.name for item in session.get_inputs()]
    if "pixel_values" not in input_names:
        raise RuntimeError(f"Model input pixel_values not found: {input_names}")

    outputs = session.get_outputs()
    output = next((item for item in outputs if "pooler" in item.name.lower()), None)
    if output is None:
        raise RuntimeError(f"Model has no pooled vision output: {[item.name for item in outputs]}")
    return session, output.name


def embed_image(session, output_name, path):
    with Image.open(path) as source:
        image = ImageOps.exif_transpose(source).convert("RGB")
        image = image.resize((IMAGE_SIZE, IMAGE_SIZE), Image.Resampling.BILINEAR)
        pixels = np.asarray(image, dtype=np.float32) / 255.0

    pixels = (pixels - 0.5) / 0.5
    pixels = np.ascontiguousarray(np.transpose(pixels, (2, 0, 1))[np.newaxis, ...])
    output = session.run([output_name], {"pixel_values": pixels})[0]
    vector = np.asarray(output, dtype=np.float32).reshape(-1)
    if vector.size != OUTPUT_DIM:
        raise RuntimeError(f"Expected {OUTPUT_DIM} features, model returned {vector.size}")
    norm = float(np.linalg.norm(vector))
    if not np.isfinite(norm) or norm <= 1e-12:
        raise RuntimeError("Model returned an invalid zero-magnitude embedding")
    return (vector / norm).tolist()


def main():
    if len(sys.argv) != 2:
        raise RuntimeError("usage: semantic_embedder.py MODEL_PATH")
    session, output_name = load_session(sys.argv[1])
    print(json.dumps({"ready": True, "dimension": OUTPUT_DIM}), flush=True)

    for line in sys.stdin:
        try:
            request = json.loads(line)
            vector = embed_image(session, output_name, request["path"])
            response = {"vector": vector}
        except Exception as exc:
            response = {"error": str(exc)}
            traceback.print_exc(file=sys.stderr)
        print(json.dumps(response, separators=(",", ":")), flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"error": str(exc)}), flush=True)
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
