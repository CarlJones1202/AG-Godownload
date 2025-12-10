IMPORTANT:
- IN ALL FRONTEND WORK, ALWAYS ADHERE TO THE DESIGN DOCS, NEVER DEVIATE UNLESS SPECIFICALLY INSTRUCTED TO DO SO WITH THE PHRASE "Nike" (case sensitive).

[Major Tasks]
- AI image labeling/tagging
    - Tag images based on content (bike, car, shirt, skirt, etc.)
    - Tag images based on pose (standing, sitting, lying, etc.)
    - Tag images based on mood (happy, sad, angry, etc.)
    - Tag images based on "vibe" (fun, sexy, etc.)
- AI image matching
    - Find similar images based on the "feel" of the image, rather than pixel similarity
    - The list of images should be returned as a base64 encoded string of image ids
    - The "similarity" page should be loadable with the base64 encoded string of image ids
- Advanced image searching
    - Search for images based on tags, mood, vibe, etc.
    - Search for images based on similarity
    - Search for images based on color (top 16 colors used in database should be provided to the user, with an optional color picker)
- Advanced image sorting
    - Sort images based on created date
    - Sort images based on size
    - Sort images based on random seed
- Gallery metadata gathering
    - Using the gallery name, and any people associated with it search for the "studio" of the gallery (playboy, metart, femjoy, etc)
    - Gather gallery information (publish date, number of images, rating, description, etc)
    - Save metadata to database
    - Update gallery page with metadata


[Minor Tasks]
- Favoriting images
    - Favorite/unfavorite images from any pages
    - 'f' key favorites/unfavorites the currently selected image
    - Favorites page

- Auto link galleries to people
    - When a person is created, run the link process
    - When a person is given a new alias, run the link process
    - When a new source is added, run the link process across all people