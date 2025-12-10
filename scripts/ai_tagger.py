import sys
import json
import torch
import warnings
import argparse
import os
from PIL import Image
from transformers import CLIPProcessor, CLIPModel
from ultralytics import YOLO

warnings.filterwarnings("ignore")

def load_tags(tag_file):
    """Load pose and mood tags from JSON"""
    with open(tag_file, 'r') as f:
        data = json.load(f)
    # Only return pose and mood, content comes from YOLO
    return {
        'pose': data.get('pose', []),
        'mood': data.get('mood', [])
    }

def load_clip_model(device):
    print(f"Loading CLIP model on {device}...", file=sys.stderr)
    model = CLIPModel.from_pretrained("openai/clip-vit-base-patch32").to(device)
    processor = CLIPProcessor.from_pretrained("openai/clip-vit-base-patch32")
    return model, processor

def load_yolo_model():
    print(f"Loading YOLO model...", file=sys.stderr)
    # Use YOLOv8n (nano) for speed, or yolov8m for better accuracy
    model = YOLO('yolov8n.pt')  # Will auto-download on first run
    return model

def process_yolo(model, image_path):
    """Run YOLO object detection and return detected objects"""
    results = model(image_path, verbose=False)
    
    detected_objects = []
    seen_classes = set()
    
    # Process detections
    for result in results:
        boxes = result.boxes
        for box in boxes:
            # Get class name and confidence
            cls_id = int(box.cls[0])
            conf = float(box.conf[0])
            class_name = result.names[cls_id]
            
            # Only include if confidence > 30% and not already seen
            if conf > 0.3 and class_name not in seen_classes:
                detected_objects.append({
                    "name": class_name,
                    "confidence": conf
                })
                seen_classes.add(class_name)
    
    # Sort by confidence
    detected_objects.sort(key=lambda x: x['confidence'], reverse=True)
    return detected_objects

def process_clip(model, processor, image, categories, device):
    """Run CLIP classification for Pose and Mood"""
    results = {}
    image_inputs = processor(images=image, return_tensors="pt").to(device)

    for category, labels in categories.items():
        text_inputs = processor(text=labels, return_tensors="pt", padding=True, truncation=True).to(device)
        
        with torch.no_grad():
            outputs = model(**text_inputs, **image_inputs)
            logits_per_image = outputs.logits_per_image
            probs = logits_per_image.softmax(dim=1)
        
        scores = probs[0].cpu().numpy()
        category_results = []
        
        for i, score in enumerate(scores):
            if score > 0.05:  # 5% threshold
                category_results.append({
                    "name": labels[i],
                    "confidence": float(score)
                })
        
        category_results.sort(key=lambda x: x['confidence'], reverse=True)
        results[category] = category_results
        
    return results

def main():
    parser = argparse.ArgumentParser(description='AI Tagger - YOLO + CLIP')
    parser.add_argument('--image', required=True, help='Path to image file')
    args = parser.parse_args()

    device = "cuda" if torch.cuda.is_available() else "cpu"
    print(f"Using device: {device}", file=sys.stderr)

    script_dir = os.path.dirname(os.path.abspath(__file__))
    tags_path = os.path.join(script_dir, "tags.json")
    
    try:
        # Load models
        clip_categories = load_tags(tags_path)
        clip_model, clip_processor = load_clip_model(device)
        yolo_model = load_yolo_model()
        
        # Load image
        image = Image.open(args.image).convert("RGB")

        # Process with YOLO for Content
        content_tags = process_yolo(yolo_model, args.image)
        
        # Process with CLIP for Pose/Mood
        clip_results = process_clip(clip_model, clip_processor, image, clip_categories, device)

        # Combine results
        final_result = {
            "content": content_tags,
            **clip_results
        }

        print(json.dumps(final_result, indent=2))

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
