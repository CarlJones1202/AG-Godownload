import argparse
import json
import sys
import os
from PIL import Image
import torch
from transformers import CLIPProcessor, CLIPModel

# Categories and their candidate labels
CATEGORIES = {
    "content": [
        # Clothing - Tops
        "t-shirt", "shirt", "blouse", "tank top", "crop top", "tube top", "halter top", 
        "sweater", "cardigan", "hoodie", "jacket", "coat", "blazer", "vest",
        # Clothing - Bottoms
        "jeans", "skinny jeans", "ripped jeans", "shorts", "denim shorts", "trousers", 
        "leggings", "yoga pants", "sweatpants", "skirt", "mini skirt", "midi skirt", 
        "pencil skirt", "pleated skirt",
        # Clothing - Full Body
        "dress", "sundress", "evening gown", "maxi dress", "mini dress", "cocktail dress", 
        "bodysuit", "romper", "jumpsuit", "robe", "kimono", "pajamas", "onesie",
        # Swimwear & Lingerie
        "bikini", "swimsuit", "one-piece", "lingerie", "bra", "sports bra", "panties", 
        "thong", "corset", "bustier", "stockings", "garter belt", "fishnets", "tights", "socks",
        # Accessories
        "glasses", "sunglasses", "hat", "cap", "beanie", "fedora", "jewelry", "necklace", 
        "choker", "earrings", "bracelet", "watch", "ring", "belt", "scarf", "gloves", 
        "heels", "boots", "sneakers", "sandals", "barefoot",
        # Physical Attributes
        "blonde", "brunette", "redhead", "black hair", "dyed hair", "long hair", "short hair", 
        "curly hair", "straight hair", "ponytail", "pigtails", "bun", "bangs", 
        "blue eyes", "green eyes", "brown eyes", "makeup", "lipstick", "eyeshadow", 
        "tattoo", "piercing", "navel piercing", "nose piercing", "tan lines", "pale skin", 
        "dark skin", "freckles", "glasses",
        # Setting - Indoor
        "indoor", "bedroom", "bed", "sheets", "pillow", "living room", "couch", "sofa", 
        "chair", "kitchen", "bathroom", "shower", "bathtub", "mirror", "reflection", 
        "office", "desk", "studio", "plain background", "white background", "curtains", 
        "window", "door", "floor", "carpet", "rug",
        # Setting - Outdoor
        "outdoor", "nature", "forest", "trees", "grass", "flowers", "garden", "park", 
        "beach", "ocean", "sea", "sand", "water", "pool", "swimming pool", "sky", "clouds", 
        "sun", "sunset", "sunrise", "mountains", "rocks", "city", "urban", "street", 
        "building", "wall", "brick wall", "fence", "car", "motorcycle", "bike", "boat"
    ],
    "pose": [
        # Standing
        "standing", "full body pose", "leaning", "leaning against wall", "hands on hips", 
        "arms crossed", "hands in pockets", "one leg up", "walking", "running", "jumping", 
        "dancing", "balancing",
        # Sitting
        "sitting", "sitting on chair", "sitting on floor", "sitting on bed", "knees up", 
        "legs crossed", "cross-legged", "straddling", "perched", "squatting",
        # Lying
        "lying down", "lying on back", "lying on stomach", "lying on side", "fetal position", 
        "sprawled", "reclining", "resting", "sleeping",
        # Kneeling/Bending
        "kneeling", "on all fours", "crawling", "bending over", "arching back", "stretching", 
        "shaking hips", "twisting",
        # Head & Face
        "looking at camera", "looking away", "looking down", "looking up", "looking over shoulder", 
        "head tilt", "smiling", "laughing", "grinning", "frowning", "pouting", "duck face", 
        "tongue out", "winking", "eyes closed", "biting lip", "blowing kiss", "serious face", 
        "surprise", "mouth open",
        # Hands/Arms
        "touching hair", "touching face", "hand on chin", "finger on lips", "peace sign", 
        "waving", "pointing", "holding object", "holding phone", "taking selfie", 
        "arms raised", "arms behind head", "hugging self"
    ],
    "mood": [
        # Positive/Energetic
        "happy", "joyful", "cheerful", "ecstatic", "excited", "enthusiastic", "energetic", 
        "vibrant", "lively", "playful", "fun", "bubbly", "goofy", "silly", "laughing",
        # Calm/Relaxed
        "relaxed", "calm", "peaceful", "serene", "chill", "cozy", "comfy", "aloof", 
        "bored", "tired", "sleepy", "dreamy", "content",
        # Intense/Emotional
        "serious", "intense", "focused", "thoughtful", "pensive", "contemplative", 
        "melancholic", "sad", "emotional", "moody", "angry", "fierce", "aggressive", 
        "confident", "proud", "dominant", "submissive",
        # Romantic/Alluring
        "romantic", "seductive", "flirtatious", "sensual", "alluring", "charming", 
        "cute", "sweet", "innocent", "shy", "coy", "mysterious", "intriguing"
    ],
    "vibe": [
        # Aesthetic Styles
        "vintage", "retro", "90s style", "80s style", "y2k", "classic", "modern", 
        "futuristic", "cyberpunk", "vaporwave", "grunge", "punk", "goth", "emo", "edgy", 
        "bohemian", "hippie", "minimalist", "maximalist", "urban", "streetwear", 
        "rustic", "cottagecore", "country", "fantasy", "fairy",
        # Lighting/Atmosphere
        "bright", "colorful", "pastel", "neon", "dark", "dim", "shadowy", "high contrast", 
        "soft light", "natural light", "golden hour", "warm tone", "cool tone", 
        "black and white", "monochrome", "sepia", "cinematic", "film grain", "hazy", "dreamy",
        # Quality/Context
        "candid", "selfie", "mirror selfie", "professional shot", "studio shot", 
        "amateur", "snapshot", "artistic", "editorial", "glamour", "lifestyle", 
        "luxury", "elegant", "classy", "trashy", "casual", "sporty", "athletic"
    ]
}

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
            inputs = processor(text=labels, images=image, return_tensors="pt", padding=True)
            
            # Move inputs to device
            inputs = {k: v.to(device) for k, v in inputs.items()}
            
            with torch.no_grad():
                outputs = model(**inputs)
            
            logits_per_image = outputs.logits_per_image # this is just the image-text similarity score
            probs = logits_per_image.softmax(dim=1) # we can take the softmax to get the label probabilities
            
            # Get top 3
            top_k = 3
            values, indices = probs.topk(top_k)
            
            category_results = []
            for i in range(top_k):
                idx = indices[0][i].item()
                prob = values[0][i].item()
                if prob > 0.1: # Threshold to filter weak matches
                    category_results.append({
                        "name": labels[idx],
                        "confidence": prob
                    })
            
            results[category] = category_results

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

    model, processor, device = load_model()
    tags = process_image(args.image, model, processor, device)
    
    print(json.dumps(tags))

if __name__ == "__main__":
    main()
