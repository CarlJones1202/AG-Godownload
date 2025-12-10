import argparse
import json
import sys
import os
from PIL import Image
import torch
from transformers import CLIPProcessor, CLIPModel

# Categories will be loaded from file
CATEGORIES = {}

def load_tags():
    global CATEGORIES
    try:
        with open('scripts/tags.json', 'r') as f:
            CATEGORIES = json.load(f)
    except Exception as e:
        print(json.dumps({"error": f"Failed to load tags.json: {str(e)}"}))
        sys.exit(1)

def load_model():
    """Load CLIP model and processor."""
    try:
        model_id = "openai/clip-vit-base-patch32"
        model = CLIPModel.from_pretrained(model_id)
        processor = CLIPProcessor.from_pretrained(model_id)
        
        # Move to GPU if available
        device = "cuda" if torch.cuda.is_available() else "cpu"
        model.to(device)
        
        # Log device to stderr so it doesn't break JSON stdout
        sys.stderr.write(f"Loaded model on device: {device}\n")
        
        return model, processor, device
    except Exception as e:
        print(json.dumps({"error": f"Failed to load model: {str(e)}"}))
        sys.exit(1)

def process_image(image_path, model, processor, device):
    """Process image and return tags."""
    try:
        image = Image.open(image_path)
    except Exception as e:
        print(json.dumps({"error": f"Failed to open image: {str(e)}"}))
        sys.exit(1)

    results = {}

    for category, labels in CATEGORIES.items():
        try:
            # Batch processing for large label sets?
            # CLIP handles batching, but if labels are > 77 tokens it truncates.
            # Here 'labels' is a list of strings.
            # If the list is huge (e.g. 1000), we might need to chunk it to avoid OOM or constraints?
            # CLIPProcessor handles lists of text.
            # Let's chunk if > 50 to be safe/performant, or just let it fly?
            # 1000 labels might be slow in one pass.
            
            chunk_size = 50
            all_results = []
            
            for i in range(0, len(labels), chunk_size):
                chunk = labels[i:i+chunk_size]
                inputs = processor(text=chunk, images=image, return_tensors="pt", padding=True)
                inputs = {k: v.to(device) for k, v in inputs.items()}
                
                with torch.no_grad():
                    outputs = model(**inputs)
                
                probs = outputs.logits_per_image.softmax(dim=1)
                
                for j, prob in enumerate(probs[0]):
                    p = prob.item()
                    if p > 0.05: # Lower threshold to catch more
                        all_results.append({
                            "name": chunk[j],
                            "confidence": p
                        })

            # Sort by confidence descending
            all_results.sort(key=lambda x: x["confidence"], reverse=True)
            
            # No top_k limit, just return all valid matches
            results[category] = all_results

        except Exception as e:
            results[category] = {"error": str(e)}

    return results

def main():
    parser = argparse.ArgumentParser(description="AI Image Tagger using CLIP")
    parser.add_argument("--image", required=True, help="Path to the image file")
    args = parser.parse_args()

    if not os.path.exists(args.image):
        print(json.dumps({"error": f"Image file not found: {args.image}"}))
        sys.exit(1)

    # Suppress warnings from transformers
    os.environ["TOKENIZERS_PARALLELISM"] = "false"

    load_tags()
    model, processor, device = load_model()
    tags = process_image(args.image, model, processor, device)
    
    print(json.dumps(tags))

if __name__ == "__main__":
    main()
