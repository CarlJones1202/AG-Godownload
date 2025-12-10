import json
import os

def load_imagenet():
    try:
        with open('scripts/imagenet_classes.txt', 'r') as f:
            # Clean up: remove index if present (format depends on file)
            # The linked file seems to be just names, one per line?
            # Let's inspect it. If it has IDs, we strip them.
            lines = [line.strip().replace('_', ' ') for line in f.readlines()]
            return lines
    except:
        return []

def main():
    imagenet_tags = load_imagenet()
    
    # Extensive lists
    clothing_tags = [
        "t-shirt", "shirt", "blouse", "tank top", "crop top", "tube top", "halter top", "camisole", "tunic",
        "sweater", "cardigan", "hoodie", "sweatshirt", "fleece", "turtleneck", "pullover",
        "jacket", "coat", "blazer", "vest", "parka", "trench coat", "raincoat", "bomber jacket", "leather jacket", "denim jacket",
        "jeans", "skinny jeans", "ripped jeans", "boyfriend jeans", "mom jeans", "bootcut jeans", "straight jeans",
        "shorts", "denim shorts", "running shorts", "cycling shorts", "high-waisted shorts", "booty shorts",
        "trousers", "pants", "leggings", "yoga pants", "sweatpants", "joggers", "cargo pants", "chinos", "slacks",
        "skirt", "mini skirt", "midi skirt", "maxi skirt", "pencil skirt", "pleated skirt", "skater skirt", "wrap skirt",
        "dress", "sundress", "evening gown", "maxi dress", "mini dress", "cocktail dress", "shift dress", "bodycon dress", "wrap dress", "slip dress",
        "bodysuit", "romper", "jumpsuit", "robe", "kimono", "pajamas", "onesie", "nightgown", "lingerie", "babydoll",
        "bikini", "swimsuit", "one-piece swimsuit", "monokini", "cover-up", "sarong",
        "bra", "sports bra", "push-up bra", "bralette", "panties", "thong", "g-string", "boyshorts", "briefs",
        "corset", "bustier", "garter belt", "stockings", "fishnets", "tights", "pantyhose", "knee-high socks", "ankle socks",
        "high heels", "stiletto", "pumps", "sandals", "flip-flops", "boots", "ankle boots", "knee-high boots", "thigh-high boots", "sneakers", "trainers", "flats", "loafers", "barefoot",
        "hat", "cap", "beanie", "fedora", "sun hat", "beret", "cowboy hat",
        "glasses", "sunglasses", "aviators", "cat-eye glasses", "round glasses",
        "jewelry", "necklace", "choker", "earrings", "hoop earrings", "stud earrings", "bracelet", "bangle", "watch", "ring", "nose ring", "navel ring",
        "belt", "leather belt", "scarf", "gloves", "bag", "handbag", "purse", "clutch", "backpack", "tote bag"
    ]

    physical_tags = [
        "blonde", "blonde hair", "platinum blonde", "strawberry blonde", "dirty blonde",
        "brunette", "brown hair", "light brown hair", "dark brown hair",
        "redhead", "red hair", "ginger hair", "auburn hair",
        "black hair", "jet black hair",
        "dyed hair", "pink hair", "blue hair", "green hair", "purple hair", "silver hair", "white hair", "multicolored hair",
        "long hair", "short hair", "medium hair", "shoulder length hair",
        "curly hair", "wavy hair", "straight hair", "frizzy hair", "braided hair",
        "ponytail", "high ponytail", "pigtails", "twin tails", "bun", "messy bun", "updo", "bob cut", "pixie cut", "bangs",
        "blue eyes", "green eyes", "brown eyes", "hazel eyes", "gray eyes",
        "pale skin", "fair skin", "medium skin", "olive skin", "tan skin", "dark skin", "brown skin",
        "freckles", "mole", "birthmark", "dimples",
        "tattoo", "sleeve tattoo", "back tattoo", "leg tattoo", "small tattoo",
        "piercing", "nose piercing", "lip piercing", "navel piercing", "ear piercing",
        "makeup", "natural makeup", "heavy makeup", "lipstick", "red lipstick", "pink lipstick", "nude lipstick",
        "eyeshadow", "smokey eye", "eyeliner", "cat eye", "mascara", "blush", "highlighter", "nail polish", "manicure"
    ]

    scene_tags = [
        "indoor", "bedroom", "living room", "kitchen", "bathroom", "dining room", "hallway", "staircase", "basement", "attic",
        "bed", "messy bed", "sheets", "pillow", "blanket",
        "couch", "sofa", "armchair", "chair", "desk", "office chair", "table", "coffee table",
        "window", "curtains", "blinds", "door", "doorway", "floor", "carpet", "rug", "hardwood floor", "tile floor",
        "wall", "brick wall", "painted wall", "wallpaper", "mirror", "reflection",
        "shower", "bathtub", "sink", "toilet",
        "outdoor", "nature", "forest", "woods", "jungle", "rainforest",
        "trees", "leaves", "grass", "flowers", "wildflowers", "garden", "park", "bush",
        "beach", "ocean", "sea", "sand", "waves", "coast", "shore", "cliffs", "rocks",
        "water", "lake", "river", "stream", "pool", "swimming pool", "hot tub",
        "sky", "blue sky", "cloudy sky", "clouds", "sun", "sunlight", "sunset", "sunrise", "stars", "moon",
        "mountains", "hills", "valley", "canyon", "desert", "dunes",
        "urban", "city", "street", "alley", "sidewalk", "road", "highway",
        "building", "skyscraper", "house", "apartment", "balcony", "rooftop", "terrace", "patio",
        "fence", "gate", "bridge", "tunnel",
        "car", "sports car", "convertible", "sedan", "suv", "truck", "motorcycle", "scooter", "bike", "bicycle", "boat", "yacht"
    ]
    
    # Merge content
    content = sorted(list(set(imagenet_tags + clothing_tags + physical_tags + scene_tags)))

    pose_tags = [
        "standing", "standing still", "standing tall",
        "leaning", "leaning forward", "leaning back", "leaning against wall",
        "hands on hips", "hands in pockets", "arms crossed", "arms folded", "arms raised", "arms up", "arms behind head",
        "one leg up", "leg raised", "kicking",
        "walking", "walking away", "walking towards camera", "strolling",
        "running", "jogging", "sprinting",
        "jumping", "leaping", "mid-air",
        "dancing", "twirling", "spinning",
        "balancing", "poised",
        "sitting", "sitting down", "sitting on chair", "sitting on floor", "sitting on bed", "sitting on stool", "sitting on bench",
        "knees up", "hugging knees", "legs crossed", "cross-legged", "lotus position",
        "straddling", "straddling chair", "manspreading",
        "perched", "squatting", "crouching",
        "lying down", "lying on back", "lying on stomach", "lying on side",
        "fetal position", "curled up",
        "sprawled", "stretched out",
        "reclining", "resting", "sleeping", "napping",
        "kneeling", "on knees", "on all fours", "crawling",
        "bending over", "bending forward",
        "arching back", "stretching", "yoga pose", "pilates pose",
        "shaking hips", "twerking",
        "twisting", "turning",
        "looking at camera", "eye contact", "staring",
        "looking away", "looking off camera", "looking down", "looking up", "looking back", "looking over shoulder",
        "head tilt", "tilted head",
        "smiling", "big smile", "grin", "smirk", "laughing", "giggling",
        "frowning", "scowling", "sad face", "crying",
        "pouting", "duck face", "kissing face", "blowing kiss",
        "tongue out", "sticking tongue out", "winking", "wink",
        "eyes closed", "sleeping face",
        "biting lip", "lip bite",
        "serious face", "poker face", "neutral expression",
        "surprise", "shocked", "open mouth", "gasping", "screaming",
        "touching hair", "playing with hair", "hand in hair",
        "touching face", "hand on face", "hand on chin", "hand on cheek",
        "finger on lips", "hushing",
        "peace sign", "v sign", "thumbs up", "waving", "pointing",
        "holding object", "holding phone", "holding cup", "holding glass",
        "taking selfie", "mirror selfie",
        "hugging self", "embracing"
    ]

    mood_tags = [
        "happy", "joyful", "cheerful", "ecstatic", "elated", "thrilled", "delighted", "content", "glad",
        "excited", "enthusiastic", "eager", "hyped", "pumped",
        "energetic", "vibrant", "lively", "dynamic", "active", "spirited",
        "playful", "fun", "funny", "goofy", "silly", "mischievous", "cheeky",
        "bubbly", "cheery", "sunny",
        "laughing", "humorous", "amused",
        "relaxed", "calm", "peaceful", "serene", "tranquil", "placid", "zen",
        "chill", "laid-back", "easygoing", "mellow",
        "cozy", "comfy", "snug", "warm", "inviting",
        "aloof", "detached", "indifferent", "distant", "cool",
        "bored", "uninterested", "apathetic",
        "tired", "sleepy", "drowsy", "exhausted", "fatigued", "weary",
        "dreamy", "daydreaming", "whimsical", "nostalgic", "sentimental",
        "serious", "stern", "grave", "solemn",
        "intense", "fierce", "focused", "determined", "concentrated",
        "thoughtful", "pensive", "contemplative", "reflective", "meditative",
        "melancholic", "sad", "sorrowful", "depressed", "gloomy", "downcast", "tearful",
        "emotional", "moody", "temperamental",
        "angry", "furious", "irritable", "annoyed", "frustrated", "upset",
        "aggressive", "hostile", "threatening", "confrontational",
        "confident", "bold", "assured", "self-assured", "poised",
        "proud", "haughty", "arrogant",
        "dominant", "commanding", "powerful", "strong", "assertive",
        "submissive", "meek", "passive", "yielding",
        "romantic", "loving", "affectionate", "tender", "intimate", "passionate",
        "seductive", "sexy", "sultry", "erotic", "desirable", "provocative", "tempting",
        "flirtatious", "flirty", "teasing", "coquettish",
        "sensual", "luscious", "voluptuous",
        "alluring", "charming", "captivating", "enchanting", "bewitching",
        "cute", "adorable", "sweet", "lovely", "darling",
        "innocent", "pure", "angelic", "naive",
        "shy", "bashful", "timid", "reserved", "demure",
        "coy", "arch",
        "mysterious", "enigmatic", "secretive", "dark",
        "intriguing", "fascinating", "compelling"
    ]

    vibe_tags = [
        "vintage", "retro", "antique", "old-fashioned", "nostalgic",
        "90s style", "80s style", "70s style", "y2k aesthetic",
        "classic", "timeless", "traditional",
        "modern", "contemporary", "sleek", "futuristic", "sci-fi", "high-tech",
        "cyberpunk", "neon noir", "dystopian",
        "vaporwave", "synthwave", "retrowave",
        "grunge", "distressed", "messy", "dirty", "raw",
        "punk", "rock", "edgy", "rebellious", "anarchic",
        "goth", "gothic", "dark", "moody", "eerie", "creepy",
        "emo", "melodramatic",
        "bohemian", "boho", "hippie", "free-spirited", "earthy",
        "minimalist", "clean", "simple", "uncluttered", "austere",
        "maximalist", "busy", "eclectic", "cluttered", "bold",
        "urban", "city life", "street", "gritty", "industrial",
        "streetwear", "hypebeast", "fashion-forward",
        "rustic", "country", "rural", "pastoral", "farmhouse",
        "cottagecore", "pastoral", "whimsical", "fairycore",
        "fantasy", "magical", "ethereal", "dreamlike", "surreal",
        "fairy", "princess",
        "bright", "colorful", "vibrant", "vivid", "saturated", "pop",
        "pastel", "soft", "pale", "muted", "desaturated",
        "neon", "fluorescent", "electric", "glowing",
        "dim", "dark", "shadowy", "low-key", "noir",
        "high contrast", "dramatic lighting", "chiaroscuro",
        "soft light", "diffused light", "hazy", "foggy", "misty",
        "natural light", "sun-drenched", "golden hour", "magic hour",
        "warm tone", "golden", "orange", "yellow",
        "cool tone", "blue", "cyan", "cold",
        "black and white", "monochrome", "grayscale",
        "sepia", "aged", "faded",
        "cinematic", "film look", "movie scene",
        "film grain", "grainy", "analog", "polaroid",
        "candid", "natural", "unposed", "authentic", "real",
        "selfie", "mirror selfie", "phone pic",
        "professional shot", "studio shot", "high quality", "polished",
        "amateur", "snapshot", "casual",
        "artistic", "creative", "abstract", "experimental",
        "editorial", "magazine style", "fashion shoot",
        "glamour", "high fashion", "chic", "luxury", "opulent", "expensive", "lavish",
        "elegant", "sophisticated", "classy", "refined", "graceful",
        "trashy", "kitsch", "camp",
        "casual", "laid-back", "everyday",
        "sporty", "athletic", "active", "fitness"
    ]

    data = {
        "content": content,
        "pose": sorted(list(set(pose_tags))),
        "mood": sorted(list(set(mood_tags))),
        "vibe": sorted(list(set(vibe_tags)))
    }

    with open('scripts/tags.json', 'w') as f:
        json.dump(data, f, indent=2)

if __name__ == "__main__":
    main()
